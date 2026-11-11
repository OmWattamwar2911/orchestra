# Contributing to Orchestra

Thank you for your interest in contributing to Orchestra!

Orchestra is a distributed workflow scheduler built to demonstrate crash resilience, strict DAG execution guarantees, and independent verification through formal evidence checking.

---

## Code of Conduct

Please be respectful, collaborative, and constructive when opening issues, submitting pull requests, or participating in discussions.

---

## Development Setup

### Prerequisites
- **Go**: 1.22 or higher
- **Node.js**: v18 or higher (v20 LTS recommended)
- **etcd**: v3.5 or Docker
- **Docker & Docker Compose**: For multi-node cluster and chaos testing

### Getting the Code
```bash
git clone https://github.com/OmWattamwar2911/distributed-scheduler.git
cd distributed-scheduler
```

### Running the Backend
```bash
# Build all Go binaries (orchestrator, worker, chaos, checker, orchctl)
make build

# Run unit and race tests
go test -race ./internal/...
```

### Running the Web Dashboard
```bash
cd web
npm install
npm run dev        # Launches Vite development server on :3000
npm run build      # Production TypeScript & Vite bundle
npm run verify     # Runs Poster Modernist design token & 0px radius verification
```

---

## Submitting Pull Requests

1. **Branch Naming**: Use descriptive branch names like `feat/add-worker-metrics` or `fix/dag-cycle-detection`.
2. **Conventional Commits**: Commit messages should follow the [Conventional Commits](https://www.conventionalcommits.org/) convention:
   - `feat(scope): ...`
   - `fix(scope): ...`
   - `docs: ...`
   - `test: ...`
   - `chore: ...`
3. **Verification**: Always ensure `go test -race ./...` and `npm run verify` pass before submitting your PR.
4. **CI Pipeline**: All pull requests trigger automated GitHub Actions that compile Go binaries, run race detector suites, build the frontend, and verify design tokens.

---

## Reporting Issues

If you find a bug or potential race condition, please open an issue with:
- Steps to reproduce
- System environment (OS, Go version, Docker version)
- Relevant log output or evidence JSON snippets
