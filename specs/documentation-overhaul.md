# Spec: Documentation Restructuring & Value-Driven README

## Objective

Restructure the project documentation to elevate Zigbridge's unique value proposition in the root [`README.md`](file:///Users/julienbreux/Projects/julienbreux/zigbridge/README.md), while offloading technical deep-dives, protocol specifications, API references, and hardware setups into a clean, dedicated [`docs/`](file:///Users/julienbreux/Projects/julienbreux/zigbridge/docs) directory.

### Target Audience & Persona
- **Home Automators & General Users**: Visiting the GitHub repository to quickly understand why Zigbridge is superior for their smart home (instant direct binding, unbreakable network coordinator reliability, single-binary simplicity, friendly UI).
- **Engineers & Integrators**: Looking for in-depth technical guides (REST/WebSocket APIs, Z-Stack/Ember architecture, TCP/RFC2217 details, Home Assistant discovery formats, AI hook configuration).

### Core Goals
1. **Compelling Root README**:
   - Deliver a clear, punchy value proposition focused on *benefits over mechanics* (e.g., "Why Direct Binding matters: your switches work even if the server is off").
   - Include a comparison matrix highlighting Zigbridge vs traditional bridges (latency, resilience, memory footprint, deployment model).
   - Provide a 3-step Quick Start (download/run binary, open web dashboard).
   - Link cleanly to modular guides in [`docs/`](file:///Users/julienbreux/Projects/julienbreux/zigbridge/docs).
2. **Comprehensive `docs/` Knowledge Base**:
   - Modular, topic-specific markdown guides avoiding a monolithic wall of text.
   - Maintain 100% working links, code snippets, and configuration samples.

---

## Tech Stack & Commands

- **Format**: GitHub Flavored Markdown (GFM)
- **Tooling**: Static verification of markdown links, Go build test suite

### Executable Commands
```bash
# Verify all existing Go tests and embedded static assets still pass
go test -v -race ./...

# Verify Go static binary build
make build

# Lint Markdown files (if markdownlint is installed)
# markdownlint README.md docs/*.md
```

---

## Project Structure

```
zigbridge/
├── README.md                      # High-impact value proposition, key benefits & quick start
├── docs/                          # Detailed technical documentation & guides
│   ├── index.md                   # Documentation index & navigation map
│   ├── architecture.md            # System internals, event bus, zero-allocation pooling
│   ├── coordinators.md            # Hardware guide: SMLIGHT SLZB-06 (TCP), USB serial dongles
│   ├── direct-binding.md          # Deep dive: Zero-latency mesh binding & optimistic binds
│   ├── web-dashboard.md           # Dual-Mode Web UI (Simple vs Advanced, Diagnostics Drawer)
│   ├── api.md                     # REST API endpoints & WebSocket real-time event stream
│   ├── configuration.md           # Complete config.yaml reference and environment variables
│   └── ai-recommendations.md      # Telemetry ring-buffer, heuristics & LLM integration
├── specs/                         # Project architectural specifications
│   ├── zigbridge.md               # Core architecture & initial spec
│   ├── ui-simplification.md       # Dual-mode UI specification
│   └── documentation-overhaul.md  # This specification
├── tasks/
│   ├── plan.md
│   └── todo.md
├── config.yaml
├── Makefile
└── cmd/ & internal/
```

---

## Documentation Style Guide

1. **Voice & Tone**: Confident, user-centric, concise, and technically precise.
2. **Value First, Mechanics Second**:
   - Start with the *problem solved* and the *user outcome* before presenting configuration parameters or code blocks.
3. **GFM Standard Alerts**:
   - Use `> [!TIP]`, `> [!NOTE]`, `> [!IMPORTANT]`, and `> [!WARNING]` appropriately.
4. **Mermaid Diagrams**:
   - Use flowcharts (`flowchart TD` / `flowchart LR`) to illustrate concepts like direct binding flow vs routed flow.
5. **Link Integrity**:
   - All internal links between `README.md` and `docs/` must use relative links (e.g. `[Coordinator Setup](docs/coordinators.md)`).

---

## Boundaries

- **Always**:
  - Keep `README.md` focused on user benefits, value proposition, simple installation, and clean links to `docs/`.
  - Ensure all commands and configuration examples in `docs/` match the real implementation in `config.yaml` and `internal/`.
  - Preserve all existing technical information by migrating it into the appropriate `docs/` guides.
- **Ask First**:
  - Adding third-party documentation generators (e.g., MkDocs, Docusaurus, VitePress).
- **Never**:
  - Delete technical documentation without migrating it to a guide in `docs/`.
  - Introduce broken relative links or obsolete Go version references.

---

## Success Criteria

1. **`README.md` Transformation**:
   - Technical deep dives (ZCL buffer pools, raw REST tables, coordinator driver internals) are removed from the root README and replaced with a strong, benefits-driven value proposition.
   - Includes a comparison table: *Zigbridge vs. Traditional Zigbee Bridges* (Zero-latency direct binding, network coordinator resilience, single binary, dual-mode UI).
   - Features a 60-second Quick Start guide.
   - Navigation links direct readers to relevant guides in `docs/`.
2. **`docs/` Structure Established**:
   - `docs/index.md`: Overview and table of contents.
   - `docs/coordinators.md`: Step-by-step setup for SMLIGHT SLZB-06 (TCP/RFC2217) and USB dongles.
   - `docs/direct-binding.md`: Hardware-level direct binding benefits, optimistic bindings, and examples.
   - `docs/api.md`: REST and WebSocket API contracts with request/response JSON payloads.
   - `docs/configuration.md`: Comprehensive `config.yaml` reference with all default values.
   - `docs/architecture.md`: Engineering design, low-memory zero-allocation ZCL buffer pool, and non-blocking event bus.
   - `docs/web-dashboard.md`: Guide to Simple Mode vs Advanced Mode, Activity stream, and Diagnostics Drawer.
   - `docs/ai-recommendations.md`: Telemetry collector, heuristics, and external LLM hook integration.
3. **Verification**:
   - Zero broken relative markdown links between `README.md` and `docs/`.
   - Go build (`make build`) and tests (`make test`) continue to execute with zero issues.

---

## Open Questions

1. **Documentation Site**: Do you prefer keeping documentation purely in GitHub Flavored Markdown (directly rendered in GitHub/IDE), or would you eventually like a static doc site generator (such as VitePress or MkDocs)?
   *(Assumption: Pure GFM files in `docs/` for seamless offline reading and native GitHub rendering).*
2. **Diagrams in docs**: Should we include Mermaid sequence/flowchart diagrams in `docs/architecture.md` and `docs/direct-binding.md` to visually demonstrate the latency difference between direct binding and traditional hub hops?
   *(Assumption: Yes, Mermaid diagrams render natively on GitHub and provide high visual clarity).*
