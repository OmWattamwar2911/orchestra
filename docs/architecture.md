# Orchestra: Architecture & Correctness Guarantees

Orchestra is a fault-tolerant distributed DAG workflow scheduler built in Go 1.22+ with etcd consensus and a React dashboard.

## 1. Formal Invariants & Correctness Guarantees

### Invariant 1: Delivery Semantics
- **At-least-once execution with exactly-one committed result per task attempt**:
  - Tasks can be retried or re-executed upon worker crash or timeout (at-least-once).
  - Every task result commit is executed via an atomic etcd compare-and-swap transaction conditioned on `ModRevision(task_key)` and `fencing.Revision`.
  - Conflicting or duplicate attempts are safely rejected with `ErrCASConflict`.
  - *Never described as "exactly-once delivery".*

### Invariant 2: Leader Fencing
- Every state mutation by the orchestrator leader embeds the election key CreateRevision (`fencing.Revision`).
- Zombie ex-leaders waking up from GC pauses or network partitions cannot corrupt state: their transactions evaluate to `false` and are rejected by etcd.

### Invariant 3: Dependency Resolution (Topological Order)
- A task transitions from `PENDING` to `READY` strictly when all its direct dependencies are `COMPLETED`.
- Independent DAG branches execute concurrently.

### Invariant 4: Failure Policies
- `FAIL_FAST`: When any task reaches `DEAD_LETTER`, all pending or ready tasks are transitioned to `CANCELLED`, and the workflow becomes `FAILED`.
- `CONTINUE`: When a task reaches `DEAD_LETTER`, only its recursive downstream descendants transition to `SKIPPED`. Independent branches run to completion.

### Invariant 5: Independent Verification
- Workers log `START` and `FINISH` events directly to an append-only evidence log (`results/evidence.jsonl`), decoupled from etcd state.
- The `checker` tool parses the evidence log and cross-references it with final etcd states to formally verify all invariants.

## 2. Cluster Deployment
- 3 etcd nodes (Raft consensus)
- 3 Orchestrators (1 Active Leader + 2 Standbys)
- 3 Workers
- 1 React Dashboard (Vite)
- Managed via `docker compose -f deploy/docker-compose.yml up -d`
