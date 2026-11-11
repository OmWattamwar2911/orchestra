# Orchestra

Orchestra is a fault-tolerant, high-performance distributed workflow scheduler in Go and React, designed with at-least-once task execution and exactly-one committed result per task semantics.

## Architecture Highlights
- **Distributed Leader Election & Fencing**: Active leader elected through etcd concurrency sessions. All state transitions conditioned on monotonic create revisions (fencing tokens) to prevent zombie/split-brain writes.
- **DAG Workflow Engine**: Kahn's algorithm topological sorting, dynamic ready-task dependency resolution, diamond branches, fan-out, and failure policies (`FAIL_FAST` and `CONTINUE`).
- **Bounded Worker Pool & Dynamic Discovery**: Workers self-register with etcd liveness leases (TTL 5s), automatically discover active leader, execute tasks in bounded goroutine pools, and record append-only independent evidence logs.
- **Full Observability**: Prometheus metrics, healthz/readyz probes, and structured slog state transition auditing.

---

## Develop the UI (Poster Modernist Redesign)

Orchestra features a Swiss **Poster Modernist** web application (Cream `#E3E2DE` paper background, Cobalt `#1351AA` accent, Jet `#141414` text, General Sans typography, and universal `0px` border-radius).

### 1. Local Vite Development Server (Recommended)
To run the live development server with Hot Module Replacement (HMR):
```bash
cd web
npm install
npm run dev
```
The Vite development server always mounts the latest local source at **[http://localhost:3000](http://localhost:3000)** (or `http://127.0.0.1:3000`).

### 2. Standalone Demo Mode (Zero Backend Required)
You can run and explore the entire UI without Docker or Go running:
- Open: **[http://localhost:3000/?demo=true](http://localhost:3000/?demo=true)**
- Or set environment variable: `VITE_DEMO=true` in `web/.env.local`.
The built-in deterministic simulator replicates 3 orchestrators, 5 workers, DAG task scheduling, live worker deaths, and leader failovers with zero lost tasks.

### 3. Rebuilding the Docker Container (Production Image)
If running inside Docker Compose, always rebuild with `--no-cache` to ensure the newest compiled UI bundle is copied into Nginx:
```bash
docker compose build --no-cache web && docker compose up -d --force-recreate web
```

### 4. Running UI Verification Tests
To assert that page background is `#E3E2DE`, primary button is `#1351AA`, General Sans font is loaded, and all elements adhere to `0px` border-radius:
```bash
cd web
npm run verify
```

---

## Run it

### Prerequisites
- Docker & Docker Compose
- Go 1.22+ (for running CLI or local development)

### 1. Build and Start the Cluster
To start the 3-node etcd cluster, 3 orchestrator nodes (1 active leader + 2 warm standbys), 5 worker nodes, and the React web dashboard:

```bash
docker compose up --build -d
```

Check running services:
```bash
docker compose ps
```

Scale workers dynamically at any time:
```bash
docker compose up --scale worker=8 -d
```

### 2. Verify Leader Election
Verify that exactly one orchestrator node won leadership and transitioned to the active leader:

```bash
docker compose logs orchestrator1 orchestrator2 orchestrator3 | grep "elected leader"
```

### 3. Submit the Sample Diamond Workflow
Submit the sample diamond DAG workflow (`fetch -> [transformA, transformB] -> merge -> publish`):

```bash
# Build the CLI (if not already built)
go build -o bin/orchctl ./cmd/orchctl

# Submit the diamond workflow definition
./bin/orchctl submit examples/diamond.yaml

# Or submit and immediately stream execution events to completion:
./bin/orchctl submit -watch examples/diamond.yaml
```

### 4. Query Workflow & Task States
Inspect workflow progression and task results:

```bash
# Query workflow details and task assignment breakdown:
./bin/orchctl get diamond-workflow

# List all active or completed workflows:
./bin/orchctl list
```

### 5. Access Dashboards & Health Probes
- **Web Dashboard**: Open [http://localhost:3000](http://localhost:3000) in your browser.
  - **Cluster Page**: Shows real-time leader, fencing term, orchestrator nodes, active workers (load bar, capacity, lease status), queue depth, and live event feed.
  - **Workflows & DAG Page**: Interactive ReactFlow DAG with Dagre auto-layout, node color coding per state, live SSE updates, task inspector drawer, and manual workflow cancellation.
  - **Submit Page**: YAML/JSON editor with sample templates (Diamond, Wide Fan-Out, Deep Chain, Fail-Fast, Continue-on-Failure) and client-side DAG cycle validation.
  - **Chaos Panel** (Dev-only, behind flag): Open via `http://localhost:3000/?chaos=true` or set `VITE_ENABLE_CHAOS=true` to toggle interactive fault injection ("Kill the leader", "Kill a worker").
- **Orchestrator Leader HTTP** (mapped to port 8084 on host, 8080 inside container):
  - Metrics: [http://localhost:8084/metrics](http://localhost:8084/metrics)
  - Liveness: [http://localhost:8084/healthz](http://localhost:8084/healthz)
  - Readiness: [http://localhost:8084/readyz](http://localhost:8084/readyz)

---

## Recording the Failover Demo (Step-by-Step)

Demonstrates zero downtime failover: submitting a DAG, killing the active leader mid-execution, automatic leadership failover in etcd, SSE auto-reconnect, and workflow execution completion without duplicated work.

1. **Start the Cluster**:
   ```bash
   docker compose up --build -d
   ```
2. **Open the Dashboard**:
   Navigate to [http://localhost:3000/?chaos=true](http://localhost:3000/?chaos=true) in your browser. Note the header shows `Leader: orchestrator-1`, `Term 1`, and `SSE Live` with the red **Chaos Panel** button available.
3. **Submit a Long-Running Workflow**:
   - Go to the **Submit** tab.
   - Choose **Wide Fan-Out** or **Diamond DAG** from the template dropdown (or paste a workflow with ~1-2s tasks).
   - Click **Validate & Submit Workflow**. The UI automatically switches to the **Workflows & DAG** tab.
4. **Kill the Leader Mid-Run**:
   - Open the **Chaos Panel** drawer (top-right button) and click **Kill the leader** (or run `docker kill orchestra-node-1` in terminal).
5. **Watch the Failover and Completion Live**:
   - **Reconnecting Indicator**: Notice the header SSE status changes to yellow `Reconnecting...` momentarily while Nginx routes to an alive standby node (`orchestrator-2` or `orchestrator-3`).
   - **Term & Leader Update**: Leadership smoothly switches to `orchestrator-2` (or 3) with an incremented fencing term (`Term 2`).
   - **DAG Progress**: Active tasks continue running on workers without interruption, downstream tasks get scheduled by the new leader, and all nodes turn green (`COMPLETED`).
6. **Capture the Recording**:
   - On Windows: Press `Win + Alt + R` (Xbox Game Bar) or use OBS Studio.
   - On macOS: Press `Cmd + Shift + 5` to record the browser window.
   - CLI / Terminal: Use `asciinema rec demo.cast` with `./scripts/chaos_scenario.sh`.

### 6. Tear Down the Cluster
Stop all services and remove shared volumes:

```bash
docker compose down -v
```

---

## Verification & Testing

Run full test suite with Go data race detection:
```bash
go test -race -p 1 ./...
```
