#!/bin/bash
set -euo pipefail

# Orchestra Throughput Benchmark Script
# Submits batch workflows and measures end-to-end task throughput.

ADDR="${ORCHESTRA_ADDR:-localhost:50051}"
ETCD="${ETCD_ENDPOINTS:-localhost:2379}"
WORKFLOWS="${WORKFLOWS:-25}"
RESULTS_DIR="./results"

mkdir -p "$RESULTS_DIR"

echo "=================================================================="
echo "Running Orchestra Throughput Benchmark"
echo "Orchestrator: $ADDR | etcd: $ETCD | Workflows: $WORKFLOWS"
echo "=================================================================="

START_TS=$(date +%s%N)

./bin/chaos -addr "$ADDR" -etcd-endpoints "$ETCD" -workflows "$WORKFLOWS" -inject-faults=false -results "$RESULTS_DIR/benchmark_experiment.json"

END_TS=$(date +%s%N)
TOTAL_NANOS=$((END_TS - START_TS))
TOTAL_MS=$((TOTAL_NANOS / 1000000))

echo "Benchmark duration: ${TOTAL_MS} ms"
echo "Raw output saved to $RESULTS_DIR/benchmark_experiment.json"
