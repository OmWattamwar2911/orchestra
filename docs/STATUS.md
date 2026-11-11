# Orchestra System Status & Requirements Checklist

This living status document tracks the verification state, test evidence, and known limitations for every requirement in Orchestra.

Last Updated: Phase 6 Documentation & Measured Results Complete (2026-11-11)

---

## 1. Core Engine Requirements

| Requirement | Status | Evidence / Location | Known Limitations / Notes |
| :--- | :---: | :--- | :--- |
| **DAG Validation** (Cycles with path, unknown deps, duplicate IDs, self-dep, empty workflow) | **DONE** | `internal/dag/dag.go`<br>`internal/dag/dag_test.go` (`TestCycleDetection`, `TestSelfDependency`) | Comprehensive acyclicity & dependency validation using Kahn's algorithm. |
| **Topological Scheduling & Parallel Branches** | **DONE** | `internal/dag/dag.go`<br>`internal/scheduler/engine.go` | Concurrent branch traversal with topological ordering. |
| **Failure Policies: FAIL_FAST & CONTINUE** | **DONE** | `internal/scheduler/engine.go`<br>`internal/scheduler/engine_test.go` (`TestScheduler_FailurePolicies`) | `FAIL_FAST` cancels in-flight/pending tasks; `CONTINUE` marks downstream `SKIPPED` while independent branches continue. |
| **Workflow Cancellation** | **DONE** | `internal/scheduler/engine.go` (`CancelWorkflow`)<br>`internal/scheduler/engine_test.go` (`TestScheduler_CancelWorkflow`) | Recursively cancels pending tasks and dispatches cancellation signals. |
| **Input Limits & Bounds Validation** | **DONE** | `internal/dag/dag.go`<br>`internal/dag/types.go`<br>`internal/dag/dag_test.go` (`TestValidate_InputLimits`) | Validates max tasks (10,000), payload size limit (10MB), regex identifier `^[a-zA-Z0-9_-]{1,128}$`, timeout bounds (0..604800s), and max retries (0..50). |
| **Task State Machine (9 States)** | **DONE** | `internal/store/state_machine.go`<br>`internal/store/store_test.go` (`TestStateMachine_LegalTransitions`, `TestStateMachine_IllegalTransitions`) | Strictly validates `PENDING`, `READY`, `ASSIGNED`, `RUNNING`, `COMPLETED`, `FAILED`, `SKIPPED`, `CANCELLED`, `DEAD_LETTER`. Illegal transitions rejected. |
| **Workflow State Machine (4 States)** | **DONE** | `internal/store/state_machine.go` | Validates `RUNNING`, `SUCCEEDED`, `FAILED`, `CANCELLED`. |
| **Retries with Exponential Backoff & Jitter** | **DONE** | `internal/scheduler/engine.go` (`ReportResult`)<br>`internal/scheduler/engine_test.go` (`TestScheduler_TaskRetriesAndExponentialBackoff`) | Jittered exponential delay before re-enqueuing task in `READY` state. |
| **Per-Task Timeouts** | **DONE** | `internal/scheduler/engine.go` (`startTaskTimer`)<br>`internal/worker/worker.go` | Automatic attempt cancellation when execution deadline is exceeded. |
| **Dead-Letter Queue** | **DONE** | `internal/scheduler/engine.go`<br>`internal/store/state_machine.go` | Tasks with exhausted retries transition directly to `DEAD_LETTER`. |
| **Idempotent Workflow Submission** | **DONE** | `internal/store/store.go` (`SubmitWorkflow`)<br>`internal/store/store_test.go` (`TestStore_IdempotentSubmit`) | Duplicate submissions return existing workflow without re-execution. |
| **Single Active Leader Election** | **DONE** | `internal/election/leader.go`<br>`internal/election/election_test.go` (`TestElection_SingleLeaderAndFailover`) | Contested etcd election lease (TTL 5s) with auto-recampaign. |
| **Zombie Leader Fencing (CAS)** | **DONE** | `internal/election/leader.go` (`FencedContext`)<br>`internal/election/election_test.go` (`TestElection_ZombieLeaderWriteRejected`) | Stale terms atomically rejected by etcd ModRevision compare barrier. |
| **Leader Term & Election Counter** | **DONE** | `internal/election/leader.go` (`ElectionCount()`)<br>`internal/server/http_gateway.go` | Exposes monotonic etcd revision term and election count counter in `/api/cluster`. |
| **Persistence & Single Commit** | **DONE** | `internal/store/store.go` (`CommitTaskResult`)<br>`internal/store/store_test.go` (`TestStore_CommitTaskResult_DuplicateRejected`) | Only 1 commit allowed per attempt; duplicate/stale commits rejected and metric incremented. |
| **Append-Only Event Stream** | **DONE** | `internal/store/events.go` | Globally ordered audit trail under `/orchestra/events/{ts}` with leader term. |
| **Leader Crash Recovery** | **DONE** | `internal/store/store.go` (`RecoverState`)<br>`internal/store/store_test.go` (`TestStore_LeaderRecovery_RequeuesDeadWorkerTasks`) | Scans active DAGs, compares with live worker leases, re-queues orphaned tasks. |
| **Worker Lease & Bounded Pool** | **DONE** | `internal/worker/worker.go`<br>`internal/worker/worker_test.go` | 5s ephemeral etcd lease keepalive, semaphore-bounded execution slots. |
| **Worker Evidence Log** | **DONE** | `internal/evidence/evidence.go` | Independent local JSON lines evidence file per worker node. |
| **Real Task Types (HTTP Task with SSRF Guard)** | **DONE** | `internal/worker/http_task.go`<br>`internal/worker/http_task_test.go`<br>`examples/http_pipeline.yaml` | Production `http` task executor supporting method, headers, payload, expected status, response limits, and SSRF blocking of private/loopback/link-local IPs. |
| **Scheduler Backpressure & Dispatch** | **DONE** | `internal/scheduler/engine.go`<br>`internal/scheduler/engine_test.go` | Added `hasAvailableCapacity()` to avoid popping queued items when busy; added deterministic worker tie-breaking; verified 30x pass under `-race` (`results/repeat30_scheduler.txt`). |

---

## 2. Observability & Security

| Requirement | Status | Evidence / Location | Known Limitations / Notes |
| :--- | :---: | :--- | :--- |
| **Prometheus Metrics (/metrics)** | **DONE** | `internal/metrics/metrics.go`<br>`internal/server/http_gateway.go` | Exposes task counts, durations, failovers, and duplicate rejections. |
| **Health & Readiness (/healthz, /readyz)** | **DONE** | `internal/server/http_gateway.go`<br>`internal/server/http_gateway_test.go`<br>`internal/worker/worker.go` | `/healthz` returns process status; `/readyz` queries etcd connectivity, returning 200 when ready and 503 when unready. |
| **Structured slog Logging** | **DONE** | Throughout `cmd/` and `internal/` | Logs contain `workflow_id`, `task_id`, `attempt_id`, `worker_id`, `term`. |
| **Security & Dev Mode Boundaries** | **DONE** | `internal/server/http_gateway.go`<br>`internal/server/http_gateway_test.go` | Destructive chaos endpoints return 403 Forbidden unless `ORCHESTRA_DEV_MODE=true` is set. |

---

## 3. End-to-End Verification & Chaos Harness

| Requirement | Status | Evidence / Location | Known Limitations / Notes |
| :--- | :---: | :--- | :--- |
| **End-to-End Automated Script (PowerShell & Bash)** | **DONE** | `scripts/e2e.ps1`<br>`scripts/e2e.sh`<br>`results/e2e/e2e_run.txt` | Auto-detects cluster, verifies Diamond DAG, worker failover mid-task, leader failover mid-workflow, zombie leader fencing CAS barrier, and runs offline checker. |
| **Docker Multi-Stage Image Builds** | **DONE** | `deploy/Dockerfile`<br>`deploy/Dockerfile.web`<br>`docker-compose.yml` | Builds clean stripped alpine binaries for `orchestrator`, `worker`, `orchctl`, and multi-stage Nginx container for `web`. |
| **Zombie Leader Fencing CAS Barrier** | **DONE** | `scripts/e2e.ps1` (Step 5)<br>`internal/election/leader.go`<br>`results/e2e/checker_report.json` | Paused leader lease expires after 5s TTL; unpaused zombie leader's mutating writes are strictly rejected by etcd ModRevision CAS check. |
| **Chaos Fault Generator** | **DONE** | `cmd/chaos/` | Worker SIGKILLs, leader SIGSTOP zombie pauses, network drops. |
| **Independent Invariant Checker** | **DONE** | `cmd/checker/`, `internal/checker/`<br>`cmd/checker/collector.go` | Verifies Dependency Order, Single Commit, No Lost Tasks, Workflow Termination, and Fencing Barrier. Live etcd artifact collector integrated. |
| **Checker Mutation Tests** | **DONE** | `cmd/checker/mutation_test.go` | Proves checker fails when synthetic violations are injected. |
| **Process-Based Chaos Execution** | **DONE** | `cmd/chaos/`, `scripts/e2e.ps1` | Supports both containerized Docker Compose cluster and local process-based execution. |
| **10 Seeded Runs & summary.json** | **DONE** | `results/chaos-suite/`<br>`results/summary.json`<br>`results/summary.md` | 10 of 10 seeded chaos runs (seeds 5001-5010) passed 100% with 0 invariant violations across 500 workflows, 11,419 tasks, 266,184 audit events, 178,344 worker records, and 123 leader failovers. |
| **Sharded Priority Queue Scale Benchmark** | **DONE** | `internal/queue/sharded.go`<br>`results/sharded_benchmark.txt` | Benchmarked 32-shard priority queue under chaos contention (482.3 ns/op vs Coarse 1427 ns/op) and 32 goroutines (295.9 ns/op vs Coarse 970.2 ns/op, >3.2x speedup). |

---

## 4. UI Dashboard & Web App

| Requirement | Status | Evidence / Location | Known Limitations / Notes |
| :--- | :---: | :--- | :--- |
| **Poster Modernist Aesthetics** | **DONE** | `web/src/` | Cream `#E3E2DE`, Jet `#141414`, Cobalt `#1351AA`, 0px radius, General Sans. Verified 100% by `npm run verify`. |
| **Demo Simulator Mode** | **DONE** | `web/src/data/simulatorAdapter.ts` | Complete fallback simulator with 3 orchestrators, 5 workers, live workflows. |
| **External URL Proxying (No Docker hostnames)** | **DONE** | `internal/server/http_gateway.go`<br>`web/src/` | HTTP Gateway derives `http_addr` from client `Host` header to avoid unresolvable internal hostnames. |
| **Friendly Worker Naming (worker-1..N)** | **DONE** | `web/src/components/ClusterView.tsx` | Displays friendly worker names with tooltip for full ID; removed `mock` task types from sample data. |
| **Build Stamp in Footer** | **DONE** | `web/src/components/layout/AppShell.tsx`<br>`web/src/pages/LandingPage.tsx` | Git short hash and ISO build timestamp injected via Vite define and displayed in UI footers. |

---

## 5. Codebase Hygiene & Redundancies

| Item | Status | Action Plan / Evidence |
| :--- | :---: | :--- |
| **Competing Store Packages** (`internal/etcdstore` vs `internal/store`) | **DONE** | Consolidated all keys, state machines, and interfaces into `internal/store`; deleted redundant `internal/etcdstore`. |
| **Deprecated gRPC Methods** (`DialContext`, `WithBlock`) | **DONE** | Modernized all clients in `cmd/chaos`, `cmd/orchctl`, `internal/server`, `internal/worker` to `grpc.NewClient`. Zero `SA1019` warnings remaining. |
| **Gofmt differences (7 files)** | **DONE** | Ran `gofmt -w .`. Verified `gofmt -l .` reports 0 files. |
| **Staticcheck unused variables (store_test.go)** | **DONE** | Strengthened test assertions in `internal/store/store_test.go` and verified `staticcheck ./...` returns 0 warnings. |
