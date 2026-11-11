#!/bin/bash
set -euo pipefail

# Orchestra Chaos Experiment Script
# Submits concurrent workflows, injects failures, and runs independent verification.

ADDR="${ORCHESTRA_ADDR:-localhost:50051}"
ETCD="${ETCD_ENDPOINTS:-localhost:2379}"
WORKFLOWS="${WORKFLOWS:-10}"
RESULTS_DIR="./results"

mkdir -p "$RESULTS_DIR"

echo "=================================================================="
echo "Running Orchestra Chaos Experiment"
echo "Orchestrator: $ADDR | etcd: $ETCD | Workflows: $WORKFLOWS"
echo "=================================================================="

# Run chaos experiment binary
./bin/chaos -addr "$ADDR" -etcd-endpoints "$ETCD" -workflows "$WORKFLOWS" -inject-faults=true -results "$RESULTS_DIR/chaos_experiment.json"

# Run invariant checker to verify independent evidence log
./bin/checker -etcd-endpoints "$ETCD" -evidence-log "$RESULTS_DIR/evidence.jsonl" -output "$RESULTS_DIR/verification.json"

echo "Chaos test completed successfully. Check $RESULTS_DIR/ for full reports."
