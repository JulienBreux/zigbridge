# Contributing to Zigbridge

Thank you for your interest in contributing to **Zigbridge**! 🎉

Zigbridge is a next-generation Zigbee-to-MQTT bridge and Direct Binding Orchestrator designed for speed (<15ms latency), low memory (<25 MB RSS), and hardware resilience. We welcome contributions of all kinds: bug fixes, hardware adapter support, device definitions, documentation improvements, and new features.

---

## Code of Conduct

By participating in this project, you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md). Please treat all contributors with kindness, empathy, and respect.

---

## How Can I Contribute?

### 1. Reporting Bugs
- Search existing [GitHub Issues](https://github.com/julienbreux/zigbridge/issues) to verify the bug hasn't already been reported.
- Open a new issue with a clear title and description. Include:
  - Coordinator hardware model (e.g., SMLIGHT SLZB-06, Sonoff ZBDongle-P, TubeZB).
  - Coordinator transport mode (`tcp://...` or serial `/dev/ttyUSB0`) and firmware version.
  - Zigbridge version / commit hash (`./bin/zigbridge -version`).
  - Exact steps to reproduce the issue.
  - Relevant logs (redact any sensitive Wi-Fi/MQTT credentials or network keys).

### 2. Adding Device Fixtures & Definitions
Zigbridge supports simulated device models and Home Assistant expose schemas via YAML fixtures in [`fixtures/devices.yaml`](fixtures/devices.yaml). If you have tested a new Zigbee device, you can contribute its definition to expand our out-of-the-box catalog!

### 3. Proposing Features & Specifications
For significant features or changes to core architectural boundaries, please follow our **spec-driven development** process:
1. Open a GitHub Issue or Discussion to outline the idea.
2. Submit a formal specification document in [`specs/`](specs/) (e.g. `specs/my-feature.md`) before writing extensive code. Never prefix files in `specs/` with `SPEC-`.
3. Once the specification is agreed upon, proceed with implementation.

---

## Development Environment Setup

### Prerequisites
- **Go**: Version 1.27 or newer ([Download Go](https://go.dev/dl/)).
- **Make**: Standard build automation tool.
- **golangci-lint**: Static analysis tool ([Installation Guide](https://golangci-lint.run/usage/install/)).
- *(Optional)* Zigbee hardware coordinator (SMLIGHT SLZB-06 or USB dongle).
  - **No hardware? No problem!** Zigbridge includes a fully functional **Mock Adapter & Virtual Device Simulation Lab** allowing complete development and testing without physical radio hardware.

### Quick Setup Steps

1. **Clone the Repository**:
   ```bash
   git clone https://github.com/julienbreux/zigbridge.git
   cd zigbridge
   ```

2. **Initialize Configuration**:
   ```bash
   make config   # Copies config.yaml.dist to data/config.yaml
   ```
   *(The `data/` directory is git-ignored to prevent leaking local credentials).*

3. **Run in Mock Simulation Mode** (Default for development without hardware):
   In `data/config.yaml`, ensure:
   ```yaml
   adapter:
     type: mock
   ```

4. **Build the Binary**:
   ```bash
   make build
   ```

5. **Run the Application**:
   ```bash
   ./bin/zigbridge
   ```
   Access the dashboard at **[http://localhost:8080](http://localhost:8080)**.

---

## Engineering & Architectural Principles

All code submitted to Zigbridge must adhere to these foundational principles:

### 1. Low-Memory & Zero-Allocation
- Target resident set size (RSS): `< 25 MB`.
- Use `sync.Pool` byte buffers ([`internal/zcl/buffer_pool.go`](internal/zcl/buffer_pool.go)) for encoding and decoding frames to prevent heap churn.
- Avoid unnecessary string concatenations or heap escapes in high-frequency radio paths.

### 2. Concurrency Safety & Non-Blocking Event Bus
- Protect shared in-memory registries using `sync.RWMutex`.
- Distribute events asynchronously across WebSockets and MQTT using non-blocking buffered Go channels so slow consumers never stall radio frame processing.

### 3. Offline-First & Zero-CDN Web UI
- The embedded web dashboard (`webui/dist/` via `embed.FS`) must remain **100% functional offline**.
- **No external CDN dependencies** (no CDN CSS frameworks, no external fonts, no third-party JS scripts).

### 4. Modern Go Standards (Go 1.21 – 1.27+)
- Use `errors.Is(err, ...)` and `errors.As(err, &target)`. Never use direct `err == ...` comparisons on wrapped errors.
- Use `any` instead of legacy `interface{}`.
- Use `for i := range n` instead of C-style index loops.
- Use builtin `min` / `max` and `cmp.Or` for fallback defaults.
- Use standard library `slices` and `maps` packages (`slices.Contains`, `slices.Clone`, `slices.DeleteFunc`, `maps.Clone`).
- Modern testing: always use `t.Context()` instead of `context.Background()` in test cases, and register teardowns with `t.Cleanup(func() { ... })`.

---

## Quality Gates & Testing

Before submitting a pull request, ensure all quality gates pass:

```bash
# Run all unit tests with Go race detector
make test

# Run static analysis and linter
make lint

# Run modern Go analysis checks
make modernize

# Scan dependencies for known vulnerabilities
make vulncheck
```

---

## Git Workflow & Commit Guidelines

We use **Conventional Commits** to keep git history clean, readable, and machine-parsable for automated changelogs:

### Commit Types
- `feat:` A new feature or user-facing functionality.
- `fix:` A bug fix.
- `docs:` Documentation-only changes (README, guides in `docs/`, specs).
- `refactor:` Code restructuring without changing external behavior.
- `perf:` A code change that improves performance or reduces memory allocations.
- `test:` Adding or correcting tests.
- `chore:` Build process, dependency updates, or tool configurations.

### Examples
- `feat(adapter): support EZSP protocol v13 frame format`
- `fix(controller): return 503 when coordinator is disconnected`
- `docs(specs): add capability blueprint for direct binding groups`

### Rules
- **Commit Early & Often**: Break large changes into atomic, logical commits.
- **Imperative Mood**: Use imperative tone in commit summaries ("add feature" instead of "added feature").
- **Never Commit Broken Code**: All commits must build cleanly and pass `make test` and `make lint`.

---

## Submitting a Pull Request (PR)

1. Fork the repository and create your branch from `main`:
   ```bash
   git checkout -b feat/my-new-feature
   ```
2. Implement your changes, adhering to the coding principles and testing requirements above.
3. Verify that `make test` and `make lint` pass with zero warnings or errors.
4. Push your branch to GitHub and open a Pull Request against `julienbreux/zigbridge:main`.
5. Fill out the PR description template detailing:
   - What problem this PR solves.
   - Any relevant issue numbers (`Fixes #123`).
   - How the change was verified (unit test names, simulated lab tests, or physical hardware tests).
6. Maintainers will review your PR, provide constructive feedback, and merge once approved!
