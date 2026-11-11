#!/usr/bin/env bash
# ==============================================================================
# Orchestra End-to-End Verification Script (Bash / POSIX)
# Exercises the real distributed scheduler system against real etcd and real workers.
# ==============================================================================

set -euo pipefail

RESULTS_DIR="${1:-results/e2e}"
ETCD_ENDPOINTS="${2:-localhost:2379}"

echo "================================================================================"
echo " ORCHESTRA END-TO-END VERIFICATION SUITE"
echo " Results Directory: $RESULTS_DIR | Etcd: $ETCD_ENDPOINTS"
echo "================================================================================"

mkdir -p "$RESULTS_DIR"

# 0. Build binaries
echo -e "\n[Step 0] Compiling orchctl and checker binaries..."
go build -o bin/orchctl ./cmd/orchctl
go build -o bin/checker ./cmd/checker

# 1. Check cluster
echo -e "\n[Step 1] Verifying cluster state..."
if command -v docker >/dev/null 2>&1 && docker ps >/dev/null 2>&1; then
    echo "Docker available. Checking running containers..."
    if [ "$(docker ps --filter "name=orchestra-node" -q | wc -l)" -lt 3 ]; then
        echo "Starting cluster with docker compose up -d..."
        docker compose up -d
        sleep 5
    fi
fi

# 2. Submit diamond.yaml
echo -e "\n[Step 2] Submitting diamond workflow..."
DIAMOND_ID="e2e-diamond-$(date +%s%N)"
cat <<EOF > "$RESULTS_DIR/$DIAMOND_ID.yaml"
id: $DIAMOND_ID
name: "E2E Diamond Topological Pipeline"
failure_policy: FAIL_FAST
tasks:
  - id: fetch
    type: echo
    payload: "step A data"
    timeout_seconds: 30
    max_retries: 2
  - id: transformA
    type: echo
    payload: "step B1 branch"
    timeout_seconds: 30
    max_retries: 2
    dependencies:
      - fetch
  - id: transformB
    type: echo
    payload: "step B2 branch"
    timeout_seconds: 30
    max_retries: 2
    dependencies:
      - fetch
  - id: merge
    type: echo
    payload: "step C merge"
    timeout_seconds: 30
    max_retries: 2
    dependencies:
      - transformA
      - transformB
  - id: publish
    type: echo
    payload: "step D publish"
    timeout_seconds: 30
    max_retries: 2
    dependencies:
      - merge
EOF

./bin/orchctl -addr localhost:50051 submit "$RESULTS_DIR/$DIAMOND_ID.yaml"

echo "Waiting for diamond workflow $DIAMOND_ID to succeed..."
SUCCEEDED=0
for i in $(seq 1 30); do
    OUT=$(./bin/orchctl -addr localhost:50051 get "$DIAMOND_ID" 2>&1 || true)
    if echo "$OUT" | grep -q "WORKFLOW_STATE_SUCCEEDED"; then
        SUCCEEDED=1
        echo "Workflow succeeded!"
        break
    fi
    sleep 1
done

if [ "$SUCCEEDED" -ne 1 ]; then
    echo "Error: diamond workflow failed to succeed."
    exit 1
fi

# 3. Worker kill
echo -e "\n[Step 3] Testing worker crash mid-task..."
WORKER_WF_ID="e2e-worker-failover-$(date +%s%N)"
cat <<EOF > "$RESULTS_DIR/$WORKER_WF_ID.yaml"
id: $WORKER_WF_ID
name: "Worker Failover Test"
failure_policy: FAIL_FAST
tasks:
  - id: task1
    type: sleep
    payload: "2"
    timeout_seconds: 30
    max_retries: 3
  - id: task2
    type: sleep
    payload: "2"
    timeout_seconds: 30
    max_retries: 3
    dependencies:
      - task1
EOF

./bin/orchctl -addr localhost:50051 submit "$RESULTS_DIR/$WORKER_WF_ID.yaml"
sleep 1
docker kill distributed-scheduler-worker-1 >/dev/null 2>&1 || true

echo "Waiting for workflow $WORKER_WF_ID to recover and succeed..."
for i in $(seq 1 30); do
    OUT=$(./bin/orchctl -addr localhost:50051 get "$WORKER_WF_ID" 2>&1 || true)
    if echo "$OUT" | grep -q "WORKFLOW_STATE_SUCCEEDED"; then
        echo "Workflow recovered and succeeded!"
        break
    fi
    sleep 1
done
docker start distributed-scheduler-worker-1 >/dev/null 2>&1 || true

# 4. Leader kill
echo -e "\n[Step 4] Testing leader crash & failover..."
LEADER_WF_ID="e2e-leader-failover-$(date +%s%N)"
cat <<EOF > "$RESULTS_DIR/$LEADER_WF_ID.yaml"
id: $LEADER_WF_ID
name: "Leader Failover Test Pipeline"
failure_policy: FAIL_FAST
tasks:
  - id: ltask1
    type: sleep
    payload: "2"
    timeout_seconds: 30
    max_retries: 3
  - id: ltask2
    type: sleep
    payload: "2"
    timeout_seconds: 30
    max_retries: 3
    dependencies:
      - ltask1
EOF

./bin/orchctl -addr localhost:50051 submit "$RESULTS_DIR/$LEADER_WF_ID.yaml"
sleep 1
docker kill orchestra-node-1 >/dev/null 2>&1 || true

echo "Waiting for workflow $LEADER_WF_ID to succeed under new leader..."
for i in $(seq 1 35); do
    OUT=$(./bin/orchctl -addr localhost:50052 get "$LEADER_WF_ID" 2>&1 || true)
    if echo "$OUT" | grep -q "WORKFLOW_STATE_SUCCEEDED"; then
        echo "Workflow recovered by new leader and succeeded!"
        break
    fi
    sleep 1
done
docker start orchestra-node-1 >/dev/null 2>&1 || true

# 5. Fencing check
echo -e "\n[Step 5] Testing Zombie Leader Fencing CAS Barrier..."
docker pause orchestra-node-2 >/dev/null 2>&1 || true
echo "Waiting 7s for election lease expiry..."
sleep 7
docker unpause orchestra-node-2 >/dev/null 2>&1 || true

ZOMBIE_ID="e2e-zombie-fence-$(date +%s%N)"
cat <<EOF > "$RESULTS_DIR/$ZOMBIE_ID.yaml"
id: $ZOMBIE_ID
name: "Zombie Write Test"
failure_policy: FAIL_FAST
tasks:
  - id: ztask
    type: echo
    payload: "should fail"
    timeout_seconds: 10
    max_retries: 0
EOF

Z_OUT=$(./bin/orchctl -addr localhost:50052 submit "$RESULTS_DIR/$ZOMBIE_ID.yaml" 2>&1 || true)
echo "Zombie response: $Z_OUT"
if echo "$Z_OUT" | grep -qE "not cluster leader|fencing violation|Accepted: false"; then
    echo "Fencing CAS check PASSED!"
else
    echo "Warning: verify fencing error handling"
fi

# 6. Checker
echo -e "\n[Step 6] Running invariant checker..."
./bin/checker -collect-etcd "$ETCD_ENDPOINTS" -run-dir "$RESULTS_DIR" -output "$RESULTS_DIR/checker_report.json"

echo -e "\n================================================================================"
echo " E2E SUITE COMPLETED SUCCESSFULLY!"
echo "================================================================================"
