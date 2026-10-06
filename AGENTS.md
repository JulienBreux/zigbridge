# Agent Guidelines & Project Instructions: Zigbridge

## Project Overview
- **Project Name**: `zigbridge`
- **Language**: Go 1.27+ (`go.mod`)
- **Distribution Model**: Standalone static binary (`CGO_ENABLED=0`) with embedded web UI (`embed.FS`)
- **Primary Goal**: High-performance, lightweight Zigbee-to-MQTT bridge and Direct Binding Orchestrator optimized for physical mesh bindings (<15ms latency) and network coordinators (SMLIGHT SLZB-06).

---

## Directory & File Conventions

1. **Specifications & Capability Maps (`specs/`)**:
   - **MANDATORY**: Always store and organize all specification documents and capability blueprints (`CAPABILITY-MAP.md`) in the [`specs/`](specs/) directory. Never place loose specification files in the repository root. Do not prefix files in `specs/` with `SPEC-`.
2. **Technical Guides (`docs/`)**:
   - Detailed user and developer documentation belongs in [`docs/`](docs/) (`architecture.md`, `coordinators.md`, `direct-binding.md`, `web-dashboard.md`, `api.md`, `configuration.md`, `ai-recommendations.md`).
3. **Agent Skills (`.agents/skills/`)**:
   - Reusable agent skills are maintained in `.agents/skills/`.
4. **Configuration, Data Persistence & Credentials Safety**:
   - `data/` directory: All runtime persistence (including `data/config.yaml` and `data/devices.yaml`) resides in the git-ignored `data/` directory for unified backup and Docker volume mounting (`-v ./data:/app/data`).
   - [`config.yaml.dist`](config.yaml.dist) is the committed, sanitized template.
   - Run `make config` to copy `config.yaml.dist` to `data/config.yaml`.
   - `config.yaml` and `data/` are git-ignored to prevent leaking local MQTT passwords, network keys, or tokens. Never commit credentials to version control.
5. **Source Code (`internal/` & `cmd/`)**:
   - `cmd/zigbridge/`: Application entrypoint, CLI flags, OS signal handling.
   - `internal/`: Strictly decoupled packages (`adapter`, `transport`, `zcl`, `controller`, `binding`, `ai`, `mqtt`, `config`, `web`).
6. **Task Scratchpads (`tasks/`)**:
   - `tasks/` (`plan.md`, `todo.md`) is git-ignored and excluded from version control and repository tracking.

---

## Engineering & Architectural Principles

1. **Low-Memory & Zero-Allocation**:
   - Target memory usage: `<25 MB` resident set size (RSS).
   - Use `sync.Pool` byte buffers (`internal/zcl/buffer_pool.go`) for frame encoding and decoding to avoid heap churn.
2. **Concurrency Safety & Non-Blocking Event Bus**:
   - Protect shared in-memory data (e.g. Device Registry) using `sync.RWMutex`.
   - Distribute events asynchronously across WebSockets and MQTT using non-blocking buffered Go channels so slow consumers never stall radio frame processing.
3. **Decoupled Interfaces**:
   - Abstract radio coprocessors behind the `adapter.Adapter` interface (TI Z-Stack MT, Silicon Labs EZSP, Mock).
   - Abstract physical transport behind `transport.Transport` (`io.ReadWriteCloser`).
4. **Offline-First & Zero-CDN Web UI**:
   - The embedded web dashboard must remain 100% functional offline without external CDN dependencies or third-party web fonts.
5. **Quality Gates & Testing**:
   - **MANDATORY**: Always use `make test` for running tests (runs `go test -v -race ./...`).
   - All code must pass `make test` and `make lint` (zero `go vet` and `golangci-lint` issues).
6. **Git Workflow & Commit Discipline**:
   - **MANDATORY**: After each modification, use Git idioms (Conventional Commits: `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`) to commit changes atomically.
   - Commit early and often: each logical change gets its own commit with an imperative summary explaining the *why*.
   - Never commit broken code; verify with `make test` and `make lint` prior to committing.
