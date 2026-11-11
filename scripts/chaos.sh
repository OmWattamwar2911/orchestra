#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# Orchestra Chaos Experiment Runner Script
# Executes N runs (default 10) with different seeds, randomized DAG shapes,
# concurrent fault injection, cluster draining, and independent invariant verification.
# ==============================================================================

RUNS="${RUNS:-10}"
WORKFLOWS="${WORKFLOWS:-200}"
RATE="${RATE:-15}"
SEED="${SEED:-1001}"
RESULTS_DIR="${RESULTS_DIR:-results}"
ETCD_ENDPOINTS="${ETCD_ENDPOINTS:-localhost:2379}"
ORCH_ADDRS="${ORCH_ADDRS:-localhost:50051,localhost:50052,localhost:50053}"
DRAIN_TIMEOUT="${DRAIN_TIMEOUT:-180s}"

mkdir -p "$RESULTS_DIR"

echo "================================================================================"
echo " Starting Orchestra Chaos Experiment Suite"
echo " Runs: $RUNS | Workflows/Run: $WORKFLOWS | Rate: $RATE wf/s | Base Seed: $SEED"
echo " Output Directory: $RESULTS_DIR"
echo "================================================================================"

# Build chaos experiment binary
echo "Building chaos binary..."
go build -o bin/chaos ./cmd/chaos

# Execute multi-run chaos experiment
./bin/chaos \
  -runs "$RUNS" \
  -workflows "$WORKFLOWS" \
  -rate "$RATE" \
  -seed "$SEED" \
  -results-dir "$RESULTS_DIR" \
  -etcd-endpoints "$ETCD_ENDPOINTS" \
  -orchestrator-addrs "$ORCH_ADDRS" \
  -drain-timeout "$DRAIN_TIMEOUT" \
  -inject-faults=true

echo "Chaos test suite completed successfully. Results saved in $RESULTS_DIR/"
