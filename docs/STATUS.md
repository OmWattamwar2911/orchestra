# Orchestra System Status & Requirements Checklist

This living status document tracks the verification state, test evidence, and known limitations for every requirement in Orchestra.

Last Updated: Phase 1 Code Hygiene & Static Checks Complete (2026-11-11)

---

## 1. Core Engine Requirements

| Requirement | Status | Evidence / Location | Known Limitations / Notes |
| :--- | :---: | :--- | :--- |
| **DAG Validation** (Cycles with path, unknown deps, duplicate IDs, self-dep, empty workflow) | **DONE** | `internal/dag/dag.go`<br>`internal/dag/dag_test.go` (`TestCycleDetection`, `TestSelfDependency`) | Comprehensive acyclicity & dependency validation using Kahn's algorithm. |
| **Topological Scheduling & Parallel Branches** | **DONE** | `internal/dag/dag.go`<br>`internal/scheduler/engine.go` | Concurrent branch traversal with topological ordering. |
| **Failure Policies: FAIL_FAST & CONTINUE** | **DONE** | `internal/scheduler/engine.go`<br>`internal/scheduler/engine_test.go` (`TestScheduler_FailurePolicies`) | `FAIL_FAST` cancels in-flight/pending tasks; `CONTINUE` marks downstream `SKIPPED` while independent branches continue. |
| **Workflow Cancellation** | **DONE** | `internal/scheduler/engine.go` (`CancelWorkflow`)<br>`internal/scheduler/engine_test.go` (`TestScheduler_CancelWorkflow`) | Recursively cancels pending tasks and dispatches cancellation signals. |
| **Input Limits & Bounds Validation** | **PARTIAL** | `internal/dag/dag.go` | Needs maximum workflow size checks (task count limit, payload bytes), ID format regex, and retry bounds. |
| **Task State Machine (9 States)** | **DONE** | `internal/store/state_machine.go`<br>`internal/store/store_test.go` (`TestStateMachine_LegalTransitions`, `TestStateMachine_IllegalTransitions`) | Strictly validates `PENDING`, `READY`, `ASSIGNED`, `RUNNING`, `COMPLETED`, `FAILED`, `SKIPPED`, `CANCELLED`, `DEAD_LETTER`. Illegal transitions rejected. |
| **Workflow State Machine (4 States)** | **DONE** | `internal/store/state_machine.go` | Validates `RUNNING`, `SUCCEEDED`, `FAILED`, `CANCELLED`. |
| **Retries with Exponential Backoff & Jitter** | **DONE** | `internal/scheduler/engine.go` (`ReportResult`)<br>`internal/scheduler/engine_test.go` (`TestScheduler_TaskRetriesAndExponentialBackoff`) | Jittered exponential delay before re-enqueuing task in `READY` state. |
| **Per-Task Timeouts** | **DONE** | `internal/scheduler/engine.go` (`startTaskTimer`)<br>`internal/worker/worker.go` | Automatic attempt cancellation when execution deadline is exceeded. |
| **Dead-Letter Queue** | **DONE** | `internal/scheduler/engine.go`<br>`internal/store/state_machine.go` | Tasks with exhausted retries transition directly to `DEAD_LETTER`. |
| **Idempotent Workflow Submission** | **DONE** | `internal/store/store.go` (`SubmitWorkflow`)<br>`internal/store/store_test.go` (`TestStore_IdempotentSubmit`) | Duplicate submissions return existing workflow without re-execution. |
| **Single Active Leader Election** | **DONE** | `internal/election/leader.go`<br>`internal/election/election_test.go` (`TestElection_SingleLeaderAndFailover`) | Contested etcd election lease (TTL 5s) with auto-recampaign. |
| **Zombie Leader Fencing (CAS)** | **DONE** | `internal/election/leader.go` (`FencedContext`)<br>`internal/election/election_test.go` (`TestElection_ZombieLeaderWriteRejected`) | Stale terms atomically rejected by etcd ModRevision compare barrier. |
| **Leader Term & Election Counter** | **PARTIAL** | `internal/election/leader.go` | Uses etcd ModRevision as term; needs human-friendly election counter exposed alongside it. |
| **Persistence & Single Commit** | **DONE** | `internal/store/store.go` (`CommitTaskResult`)<br>`internal/store/store_test.go` (`TestStore_CommitTaskResult_DuplicateRejected`) | Only 1 commit allowed per attempt; duplicate/stale commits rejected and metric incremented. |
| **Append-Only Event Stream** | **DONE** | `internal/store/events.go` | Globally ordered audit trail under `/orchestra/events/{ts}` with leader term. |
| **Leader Crash Recovery** | **DONE** | `internal/store/store.go` (`RecoverState`)<br>`internal/store/store_test.go` (`TestStore_LeaderRecovery_RequeuesDeadWorkerTasks`) | Scans active DAGs, compares with live worker leases, re-queues orphaned tasks. |
| **Worker Lease & Bounded Pool** | **DONE** | `internal/worker/worker.go`<br>`internal/worker/worker_test.go` | 5s ephemeral etcd lease keepalive, semaphore-bounded execution slots. |
| **Worker Evidence Log** | **DONE** | `internal/evidence/evidence.go` | Independent local JSON lines evidence file per worker node. |
| **Real Task Types (HTTP Task with SSRF Guard)** | **MISSING** | `internal/worker/worker.go` | Needs production `http` task executor (URL, method, headers, timeout, response size limits) with SSRF blocking of private/loopback IPs. |
| **Scheduler Backpressure & Dispatch** | **PARTIAL** | `internal/scheduler/engine.go` | Least-loaded dispatch implemented; `TestScheduler_BackpressureAndQueueDepth` exhibited timing race in baseline test under race detector. |

---

## 2. Observability & Security

| Requirement | Status | Evidence / Location | Known Limitations / Notes |
| :--- | :---: | :--- | :--- |
| **Prometheus Metrics (/metrics)** | **DONE** | `internal/metrics/metrics.go`<br>`internal/server/http_gateway.go` | Exposes task counts, durations, failovers, and duplicate rejections. |
| **Health & Readiness (/healthz, /readyz)** | **PARTIAL** | `internal/server/http_gateway.go` | `/healthz` exists; `/readyz` needs to verify real etcd connectivity and worker registration. |
| **Structured slog Logging** | **DONE** | Throughout `cmd/` and `internal/` | Logs contain `workflow_id`, `task_id`, `attempt_id`, `worker_id`, `term`. |
| **Security & Dev Mode Boundaries** | **PARTIAL** | `internal/server/http_gateway.go` | Dev-mode gating required for destructive chaos endpoints; CORS and graceful shutdown. |

---

## 3. Chaos Harness & Independent Checker

| Requirement | Status | Evidence / Location | Known Limitations / Notes |
| :--- | :---: | :--- | :--- |
| **Chaos Fault Generator** | **DONE** | `cmd/chaos/` | Worker SIGKILLs, leader SIGSTOP zombie pauses, network drops. |
| **Independent Invariant Checker** | **DONE** | `cmd/checker/`, `internal/checker/` | Verifies Dependency Order, Single Commit, No Lost Tasks, and Fencing Barrier. |
| **Checker Mutation Tests** | **DONE** | `cmd/checker/mutation_test.go` | Proves checker fails when synthetic violations are injected. |
| **Process-Based Chaos Execution** | **PARTIAL** | `cmd/chaos/` | Needs pure process-based mode when Docker daemon is not used. |
| **10 Seeded Runs & summary.json** | **PARTIAL** | `results/` | Initial runs exist; needs verified refreshed suite and summary export. |

---

## 4. UI Dashboard & Web App

| Requirement | Status | Evidence / Location | Known Limitations / Notes |
| :--- | :---: | :--- | :--- |
| **Poster Modernist Aesthetics** | **DONE** | `web/src/` | Cream `#E3E2DE`, Jet `#141414`, Cobalt `#1351AA`, 0px radius, General Sans. Verified 100% by `npm run verify`. |
| **Demo Simulator Mode** | **DONE** | `web/src/data/simulatorAdapter.ts` | Complete fallback simulator with 3 orchestrators, 5 workers, live workflows. |
| **External URL Proxying (No Docker hostnames)** | **PARTIAL** | `web/src/` | Needs sanitization of `orchestrator1:8080` raw URLs to proxied browser endpoints. |
| **Friendly Worker Naming (worker-1..N)** | **PARTIAL** | `web/src/`, `internal/worker/` | Needs friendly names with full UUID in tooltip; hide 'mock' task type. |
| **Build Stamp in Footer** | **MISSING** | `web/src/` | Needs Git commit hash and build timestamp rendered in UI footer. |

---

## 5. Codebase Hygiene & Redundancies

| Item | Status | Action Plan / Evidence |
| :--- | :---: | :--- |
| **Competing Store Packages** (`internal/etcdstore` vs `internal/store`) | **DONE** | Consolidated all keys, state machines, and interfaces into `internal/store`; deleted redundant `internal/etcdstore`. |
| **Deprecated gRPC Methods** (`DialContext`, `WithBlock`) | **DONE** | Modernized all clients in `cmd/chaos`, `cmd/orchctl`, `internal/server`, `internal/worker` to `grpc.NewClient`. Zero `SA1019` warnings remaining. |
| **Gofmt differences (7 files)** | **DONE** | Ran `gofmt -w .`. Verified `gofmt -l .` reports 0 files. |
| **Staticcheck unused variables (store_test.go)** | **DONE** | Strengthened test assertions in `internal/store/store_test.go` and verified `staticcheck ./...` returns 0 warnings. |
