# scripts/bench_dag.ps1
# Runs DAG benchmarks for 10,000-task workflows and saves raw output to results/dag_benchmark.txt

$ErrorActionPreference = "Stop"

$OutputDir = Join-Path $PSScriptRoot "..\results"
if (-not (Test-Path $OutputDir)) {
    New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
}

$OutputFile = Join-Path $OutputDir "dag_benchmark.txt"

Write-Host "Running DAG benchmarks on 10,000-task workflows..."
$startTime = Get-Date

# Execute benchmark with memory allocation metrics
go test -bench=Benchmark -benchmem -benchtime=5s -count=1 ./internal/dag/... | Tee-Object -FilePath $OutputFile

$elapsed = (Get-Date) - $startTime
Write-Host "Benchmark completed in $($elapsed.TotalSeconds)s. Results saved to $OutputFile"
