# ==============================================================================
# Orchestra Chaos Experiment Runner Script (PowerShell)
# Executes N runs (default 10) with different seeds, randomized DAG shapes,
# concurrent fault injection, cluster draining, and independent invariant verification.
# ==============================================================================

param(
    [int]$Runs = 10,
    [int]$Workflows = 200,
    [int]$Rate = 15,
    [int64]$Seed = 1001,
    [string]$ResultsDir = "results",
    [string]$EtcdEndpoints = "localhost:2379",
    [string]$OrchAddrs = "localhost:50051,localhost:50052,localhost:50053",
    [string]$DrainTimeout = "180s"
)

New-Item -ItemType Directory -Force -Path $ResultsDir | Out-Null

Write-Host "================================================================================"
Write-Host " Starting Orchestra Chaos Experiment Suite"
Write-Host " Runs: $Runs | Workflows/Run: $Workflows | Rate: $Rate wf/s | Base Seed: $Seed"
Write-Host " Output Directory: $ResultsDir"
Write-Host "================================================================================"

# Build chaos experiment binary
Write-Host "Building chaos binary..."
go build -o bin/chaos.exe ./cmd/chaos
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to build chaos binary"
    exit 1
}

# Execute multi-run chaos experiment
& .\bin\chaos.exe `
  -runs $Runs `
  -workflows $Workflows `
  -rate $Rate `
  -seed $Seed `
  -results-dir $ResultsDir `
  -etcd-endpoints $EtcdEndpoints `
  -orchestrator-addrs $OrchAddrs `
  -drain-timeout $DrainTimeout `
  -inject-faults=true

if ($LASTEXITCODE -ne 0) {
    Write-Error "Chaos suite encountered errors or invariant violations"
    exit $LASTEXITCODE
}
Write-Host "Chaos test suite completed successfully. Results saved in $ResultsDir/"
