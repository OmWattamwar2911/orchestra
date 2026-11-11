#!/usr/bin/env bash
# scripts/bench_dag.sh
# Runs DAG benchmarks for 10,000-task workflows and saves raw output to results/dag_benchmark.txt
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
RESULTS_DIR="$ROOT_DIR/results"
mkdir -p "$RESULTS_DIR"

OUTPUT_FILE="$RESULTS_DIR/dag_benchmark.txt"
echo "Running DAG benchmarks on 10,000-task workflows..."
go test -bench=Benchmark -benchmem -benchtime=5s -count=1 ./internal/dag/... | tee "$OUTPUT_FILE"
echo "Benchmark finished. Results saved to $OUTPUT_FILE"
