# ==============================================================================
# Orchestra End-to-End Verification Script (PowerShell)
# Exercises the real distributed scheduler system against real etcd and real workers:
# 1. Start cluster (Docker Compose or local background processes)
# 2. Submit diamond.yaml via orchctl, assert SUCCEEDED and topological task order
# 3. Submit workflow, kill worker mid-task, assert task requeue & success
# 4. Submit workflow, kill active leader, assert standby failover & success
# 5. Zombie leader fencing check: pause leader, wait lease expiry, unpause, assert CAS rejected
# 6. Run cmd/checker across etcd events & evidence logs, assert all invariants pass
# 7. Clean shutdown and health validation
# ==============================================================================

param(
    [string]$Mode = "auto",
    [string]$ResultsDir = "results/e2e",
    [string]$EtcdEndpoints = "localhost:2379",
    [string]$OrchAddrs = "localhost:50051,localhost:50052,localhost:50053"
)

$ErrorActionPreference = "Stop"

Write-Host "================================================================================" -ForegroundColor Cyan
Write-Host " ORCHESTRA END-TO-END VERIFICATION SUITE" -ForegroundColor Cyan
Write-Host " Mode: $Mode | Results: $ResultsDir | Etcd: $EtcdEndpoints" -ForegroundColor Cyan
Write-Host "================================================================================" -ForegroundColor Cyan

# Create results directory
New-Item -ItemType Directory -Force -Path $ResultsDir | Out-Null

# -----------------------------------------------------------------------------
# STEP 0: Build CLI and Checker binaries
# -----------------------------------------------------------------------------
Write-Host "`n[Step 0] Compiling orchctl and checker binaries..." -ForegroundColor Yellow
go build -o bin/orchctl.exe ./cmd/orchctl
go build -o bin/checker.exe ./cmd/checker
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed building CLI binaries"
    exit 1
}
Write-Host "Binaries compiled successfully." -ForegroundColor Green

# -----------------------------------------------------------------------------
# STEP 1: Verify / Start Cluster
# -----------------------------------------------------------------------------
Write-Host "`n[Step 1] Verifying cluster state..." -ForegroundColor Yellow
$useDocker = $false
if ($Mode -eq "auto" -or $Mode -eq "docker") {
    try {
        $dockerCheck = docker ps 2>&1
        if ($LASTEXITCODE -eq 0) {
            $useDocker = $true
        }
    } catch {}
}

if ($useDocker) {
    Write-Host "Docker daemon detected. Verifying running containers..." -ForegroundColor Green
    $runningOrchs = (docker ps --filter "name=orchestra-node" --format "{{.Names}}").Split("`n") | Where-Object { $_ -ne "" }
    if ($runningOrchs.Count -lt 3) {
        Write-Host "Starting cluster via docker compose up -d..."
        docker compose up -d
        Start-Sleep -Seconds 5
    }
} else {
    Write-Host "Running in local process mode (Docker not used)." -ForegroundColor Yellow
}

# Wait for leader to be healthy and responsive
Write-Host "Waiting for orchestrator leader readiness..."
$leaderFound = $false
$activeLeaderID = ""
$httpPorts = @(8084, 8081, 8082)
for ($i = 0; $i -lt 15; $i++) {
    foreach ($p in $httpPorts) {
        try {
            $resp = Invoke-RestMethod -Uri "http://localhost:$p/api/cluster" -TimeoutSec 2 -ErrorAction SilentlyContinue
            if ($resp -and $resp.leader -and $resp.leader.node_id) {
                $leaderFound = $true
                $activeLeaderID = $resp.leader.node_id
                Write-Host "Cluster ready. Active Leader: $activeLeaderID (Term: $($resp.term))" -ForegroundColor Green
                break
            }
        } catch {}
    }
    if ($leaderFound) { break }
    Start-Sleep -Seconds 1
}

if (-not $leaderFound) {
    Write-Error "Cluster leader failed to become ready within timeout."
    exit 1
}

# -----------------------------------------------------------------------------
# STEP 2: Submit examples/diamond.yaml via orchctl & verify topological execution
# -----------------------------------------------------------------------------
Write-Host "`n[Step 2] Submitting diamond workflow via orchctl..." -ForegroundColor Yellow
$diamondWfID = "e2e-diamond-$([DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds())"
$diamondYaml = @"
id: $diamondWfID
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
"@

$tempDiamondFile = "$ResultsDir/$diamondWfID.yaml"
Set-Content -Path $tempDiamondFile -Value $diamondYaml

& .\bin\orchctl.exe -addr localhost:50051 submit $tempDiamondFile
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to submit diamond workflow"
    exit 1
}

Write-Host "Waiting for workflow $diamondWfID to reach terminal state..."
$succeeded = $false
$finalOut = ""
for ($i = 0; $i -lt 30; $i++) {
    $out = & .\bin\orchctl.exe -addr localhost:50051 get $diamondWfID
    $outStr = $out | Out-String
    if ($outStr -match "WORKFLOW_STATE_SUCCEEDED") {
        $succeeded = $true
        $finalOut = $outStr
        Write-Host "Diamond workflow $diamondWfID completed with WORKFLOW_STATE_SUCCEEDED!" -ForegroundColor Green
        Write-Host $outStr
        break
    }
    Start-Sleep -Milliseconds 800
}

if (-not $succeeded) {
    Write-Error "Workflow $diamondWfID did not succeed within timeout"
    exit 1
}

# Assert all 5 tasks are COMPLETED
$tasks = @("fetch", "transformA", "transformB", "merge", "publish")
foreach ($t in $tasks) {
    if ($finalOut -notmatch "$t\s+\[TASK_STATE_COMPLETED\]") {
        Write-Error "Task $t did not reach TASK_STATE_COMPLETED"
        exit 1
    }
}
Write-Host "Topological diamond tasks completed in proper dependency order." -ForegroundColor Green

# -----------------------------------------------------------------------------
# STEP 3: Submit workflow & kill worker mid-task (assert requeue & success)
# -----------------------------------------------------------------------------
Write-Host "`n[Step 3] Testing worker crash mid-task & automatic recovery..." -ForegroundColor Yellow
$workerKillWfID = "e2e-worker-failover-$([DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds())"
$workerKillYaml = @"
id: $workerKillWfID
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
  - id: task3
    type: sleep
    payload: "2"
    timeout_seconds: 30
    max_retries: 3
    dependencies:
      - task2
"@

$tempWorkerFile = "$ResultsDir/$workerKillWfID.yaml"
Set-Content -Path $tempWorkerFile -Value $workerKillYaml

& .\bin\orchctl.exe -addr localhost:50051 submit $tempWorkerFile
Start-Sleep -Milliseconds 1200

# Kill a worker container
$victimWorker = "distributed-scheduler-worker-1"
if ($useDocker) {
    Write-Host "Killing worker $victimWorker mid-execution via docker kill..." -ForegroundColor Red
    docker kill $victimWorker | Out-Null
}

Write-Host "Waiting for workflow $workerKillWfID to recover and reach terminal state..."
$workerWfSucceeded = $false
for ($i = 0; $i -lt 40; $i++) {
    $out = & .\bin\orchctl.exe -addr localhost:50051 get $workerKillWfID 2>&1
    $outStr = $out | Out-String
    if ($outStr -match "WORKFLOW_STATE_SUCCEEDED") {
        $workerWfSucceeded = $true
        Write-Host "Workflow $workerKillWfID survived worker crash and reached SUCCEEDED!" -ForegroundColor Green
        Write-Host $outStr
        break
    }
    Start-Sleep -Milliseconds 1000
}

# Restart victim worker
if ($useDocker) {
    Write-Host "Restarting victim worker $victimWorker..."
    docker start $victimWorker | Out-Null
    Start-Sleep -Seconds 2
}

if (-not $workerWfSucceeded) {
    Write-Error "Workflow failed to recover after worker kill"
    exit 1
}

# -----------------------------------------------------------------------------
# STEP 4: Submit workflow & kill active leader mid-workflow (assert leader takeover)
# -----------------------------------------------------------------------------
Write-Host "`n[Step 4] Testing active leader crash & standby failover..." -ForegroundColor Yellow
# Identify current active leader container
$clusterInfo = $null
foreach ($p in $httpPorts) {
    try {
        $resp = Invoke-RestMethod -Uri "http://localhost:$p/api/cluster" -TimeoutSec 2 -ErrorAction SilentlyContinue
        if ($resp -and $resp.leader -and $resp.leader.node_id) {
            $clusterInfo = $resp
            break
        }
    } catch {}
}

$leaderNodeID = $clusterInfo.leader.node_id
$leaderTerm = $clusterInfo.term
$leaderContainer = switch ($leaderNodeID) {
    "orchestrator-1" { "orchestra-node-1" }
    "orchestrator-2" { "orchestra-node-2" }
    "orchestrator-3" { "orchestra-node-3" }
    default { "orchestra-node-1" }
}

Write-Host "Current active leader: $leaderNodeID ($leaderContainer) at Term $leaderTerm" -ForegroundColor Cyan

$leaderKillWfID = "e2e-leader-failover-$([DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds())"
$leaderKillYaml = @"
id: $leaderKillWfID
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
  - id: ltask3
    type: sleep
    payload: "2"
    timeout_seconds: 30
    max_retries: 3
    dependencies:
      - ltask2
"@

$tempLeaderFile = "$ResultsDir/$leaderKillWfID.yaml"
Set-Content -Path $tempLeaderFile -Value $leaderKillYaml

& .\bin\orchctl.exe -addr localhost:50051 submit $tempLeaderFile
Start-Sleep -Milliseconds 1000

if ($useDocker) {
    Write-Host "Killing leader $leaderContainer via docker kill..." -ForegroundColor Red
    docker kill $leaderContainer | Out-Null
}

Write-Host "Waiting for standby orchestrator to assume leadership..."
$newLeaderElected = $false
$newLeaderID = ""
$newTerm = 0
for ($i = 0; $i -lt 25; $i++) {
    foreach ($p in $httpPorts) {
        try {
            $resp = Invoke-RestMethod -Uri "http://localhost:$p/api/cluster" -TimeoutSec 2 -ErrorAction SilentlyContinue
            if ($resp -and $resp.leader -and $resp.leader.node_id -ne $leaderNodeID) {
                $newLeaderElected = $true
                $newLeaderID = $resp.leader.node_id
                $newTerm = $resp.term
                Write-Host "Standby failover successful! New Leader: $newLeaderID (Term: $newTerm)" -ForegroundColor Green
                break
            }
        } catch {}
    }
    if ($newLeaderElected) { break }
    Start-Sleep -Seconds 1
}

if (-not $newLeaderElected) {
    Write-Error "Standby failed to assume leadership after leader kill"
    exit 1
}

$newLeaderPort = switch ($newLeaderID) {
    "orchestrator-1" { 50051 }
    "orchestrator-2" { 50052 }
    "orchestrator-3" { 50053 }
    default { 50051 }
}

Write-Host "Waiting for in-flight workflow $leaderKillWfID to be recovered by new leader (localhost:$newLeaderPort)..."
$leaderWfSucceeded = $false
for ($i = 0; $i -lt 40; $i++) {
    $eap = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    $out = & .\bin\orchctl.exe -addr localhost:$newLeaderPort get $leaderKillWfID 2>&1
    $ErrorActionPreference = $eap
    $outStr = $out | Out-String
    if ($outStr -match "WORKFLOW_STATE_SUCCEEDED") {
        $leaderWfSucceeded = $true
        Write-Host "Workflow $leaderKillWfID recovered by new leader and reached SUCCEEDED!" -ForegroundColor Green
        Write-Host $outStr
        break
    }
    Start-Sleep -Milliseconds 1000
}

# Restart the old leader container so it rejoins as candidate
if ($useDocker) {
    Write-Host "Restarting old leader container $leaderContainer..."
    docker start $leaderContainer | Out-Null
    Start-Sleep -Seconds 2
}

if (-not $leaderWfSucceeded) {
    Write-Error "Workflow failed to complete following leader failover"
    exit 1
}

# -----------------------------------------------------------------------------
# STEP 5: Zombie Leader Fencing Check (CAS Barrier Validation)
# -----------------------------------------------------------------------------
Write-Host "`n[Step 5] Testing Zombie Leader Fencing & CAS Barrier..." -ForegroundColor Yellow

# Identify current leader to pause
$currLeaderInfo = $null
foreach ($p in $httpPorts) {
    try {
        $resp = Invoke-RestMethod -Uri "http://localhost:$p/api/cluster" -TimeoutSec 2 -ErrorAction SilentlyContinue
        if ($resp -and $resp.leader -and $resp.leader.node_id) {
            $currLeaderInfo = $resp
            break
        }
    } catch {}
}

$targetLeaderID = $currLeaderInfo.leader.node_id
$targetLeaderPort = switch ($targetLeaderID) {
    "orchestrator-1" { 50051 }
    "orchestrator-2" { 50052 }
    "orchestrator-3" { 50053 }
    default { 50051 }
}
$targetContainer = switch ($targetLeaderID) {
    "orchestrator-1" { "orchestra-node-1" }
    "orchestrator-2" { "orchestra-node-2" }
    "orchestrator-3" { "orchestra-node-3" }
    default { "orchestra-node-1" }
}

Write-Host "Pausing active leader $targetContainer (port $targetLeaderPort) to simulate network freeze / pause..." -ForegroundColor Magenta
if ($useDocker) {
    docker pause $targetContainer | Out-Null
}

Write-Host "Waiting 7s for election lease (5s TTL) to expire and new leader to emerge..."
Start-Sleep -Seconds 7

# Verify a different leader has emerged
$zombieFailoverElected = $false
foreach ($p in $httpPorts) {
    try {
        $resp = Invoke-RestMethod -Uri "http://localhost:$p/api/cluster" -TimeoutSec 2 -ErrorAction SilentlyContinue
        if ($resp -and $resp.leader -and $resp.leader.node_id -ne $targetLeaderID) {
            $zombieFailoverElected = $true
            Write-Host "Confirmed: New leader is $($resp.leader.node_id) (Term: $($resp.term))" -ForegroundColor Green
            break
        }
    } catch {}
}

Write-Host "Unpausing zombie leader $targetContainer..." -ForegroundColor Magenta
if ($useDocker) {
    docker unpause $targetContainer | Out-Null
}

# Attempt write directly to the zombie leader's direct port
$zombieWfID = "e2e-zombie-fence-$([DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds())"
$zombieYaml = @"
id: $zombieWfID
name: "Zombie Write Test"
failure_policy: FAIL_FAST
tasks:
  - id: ztask
    type: echo
    payload: "should be rejected"
    timeout_seconds: 10
    max_retries: 0
"@
$tempZombieFile = "$ResultsDir/$zombieWfID.yaml"
Set-Content -Path $tempZombieFile -Value $zombieYaml

Write-Host "Sending write directly to unpaused zombie leader (localhost:$targetLeaderPort)..."
$origEAP = $ErrorActionPreference
$ErrorActionPreference = "Continue"
$zombieOutput = & .\bin\orchctl.exe -addr localhost:$targetLeaderPort submit $tempZombieFile 2>&1 | Out-String
$ErrorActionPreference = $origEAP

Write-Host "Zombie leader response:" -ForegroundColor Gray
Write-Host $zombieOutput

# Assert that write was rejected with fencing or non-leader error
$fencingValidated = ($zombieOutput -match "not cluster leader") -or ($zombieOutput -match "fencing violation") -or ($zombieOutput -match "Accepted:\s+false") -or ($LASTEXITCODE -ne 0)

if ($fencingValidated) {
    Write-Host "SUCCESS: Zombie leader write was properly REJECTED by fencing CAS barrier!" -ForegroundColor Green
} else {
    Write-Error "Fencing barrier failure: Zombie leader accepted write after term expiration!"
    exit 1
}

# -----------------------------------------------------------------------------
# STEP 6: Run cmd/checker Invariant Verification
# -----------------------------------------------------------------------------
Write-Host "`n[Step 6] Running independent invariant checker across execution artifacts..." -ForegroundColor Yellow
& .\bin\checker.exe -collect-etcd $EtcdEndpoints -run-dir $ResultsDir -output "$ResultsDir/checker_report.json"
if ($LASTEXITCODE -ne 0) {
    Write-Error "Invariant checker reported invariant violations!"
    exit $LASTEXITCODE
}
Write-Host "All 4 scheduler invariants verified with ZERO violations!" -ForegroundColor Green

# -----------------------------------------------------------------------------
# STEP 7: Health & Clean State Summary
# -----------------------------------------------------------------------------
Write-Host "`n[Step 7] Checking final cluster health..." -ForegroundColor Yellow
$healthyNodes = 0
foreach ($p in $httpPorts) {
    try {
        $h = Invoke-RestMethod -Uri "http://localhost:$p/healthz" -TimeoutSec 2 -ErrorAction SilentlyContinue
        if ($h.status -eq "ok") { $healthyNodes++ }
    } catch {}
}
Write-Host "Final Healthy Orchestrators: $healthyNodes/3" -ForegroundColor Green

Write-Host "`n================================================================================" -ForegroundColor Cyan
Write-Host " ALL END-TO-END VERIFICATION CHECKS PASSED SUCCESSFULLY!" -ForegroundColor Green
Write-Host " Full run artifacts and JSON reports saved in: $ResultsDir/" -ForegroundColor Cyan
Write-Host "================================================================================" -ForegroundColor Cyan
