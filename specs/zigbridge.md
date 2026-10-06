# Spec: Zigbridge

## Objective

Zigbridge is a modern, ultra-lightweight, high-performance Zigbee-to-MQTT bridge and network controller written in idiomatic Go. It provides:
1. Native support for networked Zigbee coordinators (such as SMLIGHT SLZB-06/SLZB-06M) via TCP/RFC2217 with aggressive keepalive and automated exponential-backoff reconnects, as well as local USB serial sticks (`/dev/ttyUSB0`, Sonoff ZBDongle-P/E, SkyConnect).
2. Decoupled radio coprocessor drivers for Texas Instruments Z-Stack (CC2652/CC1352/CC2538) and Silicon Labs EmberZNet (EZSP v8+).
3. A zero-allocation / low-memory packet processing pipeline using `sync.Pool` byte buffers for high-rate Zigbee Cluster Library (ZCL) frame handling.
4. An embedded, responsive web dashboard (`embed.FS`) running directly from the single binary with no external CDN or node dependencies, exposing REST and WebSocket APIs to control permit-join, inspect devices, manage direct bindings, and view telemetry.
5. An extensible direct Zigbee binding engine allowing battery switches and sensors to bind directly to actuator lights and plugs for sub-15ms local response and offline resilience.
6. An extensible AI recommendation hook interface logging telemetry events into a bounded ring buffer, powering heuristic pattern recognition and local/cloud LLM hooks to propose direct bindings and automated scenes.
7. Concurrency-safe event routing with full Home Assistant MQTT auto-discovery compatibility.

---

## Assumptions

1. **Target Runtime**: Pure Go single binary with `CGO_ENABLED=0` capable of running on Linux (amd64, arm64, armv7 for Raspberry Pi) and macOS.
2. **Network Coordinators**: SMLIGHT SLZB-06 / SLZB-06M commonly uses port 6638 over TCP with optional RFC2217 Telnet negotiation. The transport layer must safely negotiate or filter Telnet IAC sequences to prevent stream corruption.
3. **Web Dashboard**: Self-contained single-page application embedded in the binary via standard library `embed.FS` with vanilla JavaScript, modern CSS, and HTML5 (no Node.js build step required, no external CDN dependencies to allow offline / air-gapped IoT network operation).
4. **MQTT**: Compatible with Home Assistant MQTT auto-discovery standards (`homeassistant/<component>/.../config`) and standard Zigbee2MQTT-style topic conventions (`<base_topic>/<device_id>`).
5. **AI Analyzer**: Decoupled interface supporting both a built-in zero-dependency heuristic rule engine (temporal correlation analysis) and an extensible LLM hook for external/local models (e.g. Ollama, Gemini, OpenAI).

---

## Tech Stack

- **Language**: Go 1.24+ (using standard toolchain)
- **YAML Configuration**: `gopkg.in/yaml.v3`
- **MQTT Client**: `github.com/eclipse/paho.mqtt.golang`
- **Serial Communication**: `go.bug.st/serial` (pure Go on Linux/macOS)
- **WebSocket Streaming**: `github.com/gorilla/websocket`
- **Embedded Web UI**: Go standard `embed.FS`, HTML5, CSS3, Vanilla ES6 JavaScript

---

## Commands

```bash
# Build static binary (CGO disabled, stripped debug symbols)
CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/zigbridge cmd/zigbridge/main.go

# Run unit tests with race detection and coverage
go test -v -race -coverprofile=coverage.out ./...

# View coverage summary
go tool cover -func=coverage.out

# Static analysis and linting
go vet ./...

# Run Zigbridge with custom configuration
./bin/zigbridge -config config.yaml
```

---

## Project Structure

```
zigbridge/
├── cmd/
│   └── zigbridge/
│       └── main.go                  # Application entrypoint, CLI flags, graceful shutdown
├── internal/
│   ├── config/                      # YAML configuration parser, validation, defaults
│   │   ├── config.go
│   │   └── config_test.go
│   ├── transport/                   # Byte stream layer (TCP/RFC2217 & Serial)
│   │   ├── transport.go             # Transport interface and status callbacks
│   │   ├── tcp.go                   # SMLIGHT SLZB-06 TCP socket with keepalive & reconnect
│   │   ├── serial.go                # USB serial port driver
│   │   ├── mock.go                  # In-memory mock transport
│   │   └── transport_test.go
│   ├── zcl/                         # Zigbee Cluster Library definitions & framing
│   │   ├── clusters.go              # Standard cluster IDs and command codes
│   │   ├── frame.go                 # ZCL frame parser/encoder (zero-allocation slicing)
│   │   ├── attributes.go            # Attribute report parser (temperature, On/Off, battery)
│   │   ├── buffer_pool.go           # sync.Pool reusable byte buffers
│   │   └── zcl_test.go
│   ├── adapter/                     # Decoupled radio coprocessor drivers
│   │   ├── adapter.go               # Adapter interface and BindRequest definitions
│   │   ├── zstack/                  # Texas Instruments Z-Stack (CC2652/CC1352 MT protocol)
│   │   │   └── zstack.go
│   │   ├── ember/                   # Silicon Labs EmberZNet (EZSP protocol)
│   │   │   └── ember.go
│   │   ├── mock/                    # Virtual mock adapter for offline testing
│   │   │   └── mock.go
│   │   └── adapter_test.go
│   ├── binding/                     # Direct Zigbee binding engine
│   │   ├── binding.go               # Binding registry, execution, and lifecycle
│   │   └── binding_test.go
│   ├── ai/                          # Telemetry logger & smart recommendation engine
│   │   ├── events.go                # Bounded ring buffer EventCollector & event types
│   │   ├── recommendation.go        # Recommendation, SceneSuggestion, and Analyzer interface
│   │   ├── rule_based.go            # Heuristic correlation analyzer for direct bindings
│   │   ├── llm_hook.go              # LLM prompt serialization and hook interface
│   │   └── ai_test.go
│   ├── mqtt/                        # MQTT dispatcher & Home Assistant auto-discovery
│   │   ├── client.go                # MQTT Client interface
│   │   ├── ha_discovery.go          # Home Assistant MQTT discovery entity builders
│   │   ├── paho_client.go           # Eclipse Paho MQTT client implementation
│   │   ├── mock_client.go           # In-memory mock MQTT client
│   │   └── mqtt_test.go
│   ├── controller/                  # Central orchestrator
│   │   ├── controller.go            # Bridge runtime coordinator
│   │   ├── device.go                # Device registry and state management
│   │   ├── event_bus.go             # Non-blocking concurrent event broadcaster
│   │   └── controller_test.go
│   └── web/                         # Embedded web server and dashboard
│       ├── server.go                # HTTP router, REST endpoints, WebSocket handler
│       ├── static/                  # Embedded frontend assets (embed.FS)
│       │   ├── index.html           # Modern single-page dashboard
│       │   ├── app.js               # Reactive vanilla JS state & WebSocket handler
│       │   └── style.css            # Responsive dark/light theme CSS
├── specs/                           # Specifications and capability blueprints
│   ├── zigbridge.md                 # Architecture & protocol specification
│   ├── ui-simplification.md         # Simplified UI specification
│   ├── documentation-overhaul.md    # Documentation specification
│   ├── device-persistence.md        # Persistent device registry & bindings
│   └── CAPABILITY-MAP.md            # Module decomposition and dependency graph
├── docs/                            # Modular user and developer guides
├── config.yaml.dist                 # Production-ready YAML configuration template
├── Makefile                         # Build, test, lint, clean, cross-compilation targets
├── AGENTS.md                        # Project instructions and engineering rules for AI agents
├── go.mod
└── go.sum
```

---

## Code Style

- **Standard Library First**: Utilize standard library primitives (`sync.RWMutex`, `sync.Pool`, `context.Context`, `net/http`, `embed.FS`) wherever possible.
- **Explicit Error Handling**: Always return structured or wrapped errors (`fmt.Errorf("...: %w", err)`). Never discard errors silently.
- **Zero-Allocation Packet Slicing**: Avoid allocating new heap memory when decoding network and serial payloads. Use sub-slicing and `sync.Pool`.
- **Non-blocking Event Dispatching**: Channel sends in high-throughput fan-out paths must be non-blocking with default drops or buffered queues to prevent downstream consumers from stalling the radio pipeline.

### Canonical Example:

```go
package controller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/julienbreux/zigbridge/internal/adapter"
	"github.com/julienbreux/zigbridge/internal/zcl"
)

// HandleIncomingFrame processes a radio frame with zero heap allocation where possible.
func (c *Controller) HandleIncomingFrame(frame *zcl.Frame) {
	if frame == nil {
		return
	}

	c.devices.UpdateState(frame.SourceAddress, map[string]interface{}{
		"last_seen": time.Now().UTC(),
		"lqi":       frame.LQI,
	}, frame.LQI)

	// Non-blocking broadcast to WebSocket and MQTT consumers
	c.eventBus.Publish("frame", frame)
}
```

---

## Testing Strategy

1. **Unit Testing**:
   - Each package (`config`, `transport`, `zcl`, `adapter`, `binding`, `ai`, `mqtt`, `controller`, `web`) contains focused unit tests.
   - Mock implementations (`MockTransport`, `MockAdapter`, `MockClient`) allow 100% offline verification of protocols and edge cases without requiring physical radio hardware.
2. **Race Detection**:
   - All tests must pass with `go test -race ./...`.
3. **Buffer Reuse Verification**:
   - Ensure `sync.Pool` returns clean zero-length pre-allocated buffers without memory leakage or race conditions.
4. **TCP Disconnect & Reconnect Testing**:
   - Verify that unexpected TCP server terminations cause immediate transition to `StatusReconnecting` and resume automatically once the socket listener reopens.

---

## Boundaries

### Always Do:
- Run `go test -v -race ./...` and `go vet ./...` before considering any task complete.
- Use `context.Context` for cancellation and timeouts across all I/O and network operations.
- Ensure the embedded frontend works fully offline with zero CDN dependencies.
- Keep CGO disabled (`CGO_ENABLED=0`) to guarantee static binary portability across IoT gateways and Raspberry Pi platforms.

### Ask First:
- Adding any new external Go third-party module to `go.mod`.
- Modifying the core `Adapter` or `Transport` public interface methods.
- Changing Home Assistant MQTT auto-discovery topic patterns.

### Never Do:
- Hardcode serial device paths or TCP IP addresses without YAML config overrides.
- Perform blocking I/O inside event dispatch loops.
- Commit private keys, passwords, or credentials into repository files.

---

## Success Criteria

1. **Compilation & Static Binary**:
   - Running `make build` produces a single standalone binary `bin/zigbridge` with `CGO_ENABLED=0` and no external runtime dependencies.
2. **Transport Resilience**:
   - The TCP transport connects to remote coordinators (e.g. SMLIGHT SLZB-06), filters RFC2217 sequences, activates TCP keepalive, and automatically reconnects upon link failure with exponential backoff.
3. **Decoupled Radio Support**:
   - Both TI Z-Stack and Silicon Labs Ember/EZSP adapters compile and satisfy the unified `Adapter` interface.
4. **Embedded Dashboard**:
   - Running `bin/zigbridge` serves the embedded dashboard on `http://localhost:8080` showing coordinator status, real-time permit-join timer, device listing, direct binding manager, AI recommendation cards, and live WebSocket frame log.
5. **Direct Zigbee Binding**:
   - Creating or removing a binding from the Web UI or REST API invokes the adapter's binding mechanism, updates the in-memory registry, and broadcasts change events to WebSockets and MQTT.
6. **AI Hook & Heuristics**:
   - Event collector logs telemetry; the rule-based analyzer successfully identifies complementary clusters and temporal correlations to generate high-confidence direct binding recommendations; the LLM hook provides structured prompt generation.
7. **Home Assistant Discovery**:
   - Discovered devices generate valid HA MQTT auto-discovery payloads and publish state updates according to configuration.
8. **Test Coverage & Cleanliness**:
   - All packages compile cleanly, `go vet` reports zero warnings, and unit tests pass with race detection enabled.

---

## Decisions on Prior Questions

1. **Firmware NVRAM Initialization**: Preserve existing coordinator NVRAM by default to prevent accidental network reconfiguration.
2. **AI Recommender**: Provide data structures and the pluggable hook interface for future AI analyzers on-demand, without running active background analysis cron jobs.
3. **Direct Binding Validation**: Support optimistic direct binding with a warning when target device endpoints or clusters are not yet fully discovered (enabling battery-operated end device pairing).
