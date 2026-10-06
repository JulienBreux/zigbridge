# Capability Map: Zigbridge

| Module ID | Responsibility | Depends On |
|---|---|---|
| `config` | YAML configuration schema, default values, parsing, and validation | — |
| `transport` | Connection stream abstraction supporting local USB serial (`go.bug.st/serial`) and TCP/RFC2217 with keepalive and auto-reconnect (SMLIGHT SLZB-06 optimized) | — |
| `zcl` | Zigbee Cluster Library definitions, low-memory zero-allocation frame encoding/decoding, attribute reporting, and buffer pooling (`sync.Pool`) | — |
| `adapter` | Decoupled radio coprocessor interfaces and driver implementations for Texas Instruments Z-Stack (CC2652/CC1352 MT protocol), Silicon Labs EmberZNet (EZSP), and virtual mock | `transport`, `zcl` |
| `binding` | Direct Zigbee binding engine managing direct autonomous links between source/target device endpoints and clusters | `adapter`, `zcl` |
| `ai` | Telemetry event ring buffer, interaction history logger, heuristic rule-based correlation analyzer, and LLM hook interface for smart binding & scene proposals | `zcl` |
| `mqtt` | MQTT client, state publisher/subscriber, and Home Assistant MQTT Auto-Discovery entity configuration generator | `config` |
| `fixture` | Declarative device definitions (Zigbee2MQTT format), embedded catalog, and virtual device simulator for testing | `zcl`, `adapter` |
| `converter` | Zero-allocation inbound (`fromZigbee`) and outbound (`toZigbee`) declarative translation pipeline decoupling cluster math and vendor quirks from orchestrator | `zcl`, `fixture` |
| `controller` | Central orchestrator coordinating adapter, device registry, binding engine, AI recommender, MQTT dispatcher, and event broadcasting | `config`, `transport`, `adapter`, `binding`, `ai`, `mqtt`, `zcl`, `converter` |
| `web` | Embedded single-page dashboard (`embed.FS`) and HTTP REST / WebSocket event streaming API | `controller`, `binding`, `ai`, `adapter`, `zcl` |
| `cli` | Command-line entrypoint (`cmd/zigbridge`), flags, graceful OS signal shutdown (`SIGINT`, `SIGTERM`), and logging setup | `config`, `controller`, `transport`, `adapter`, `web`, `mqtt` |

## Build Order

`config`, `transport`, `zcl` → `adapter`, `mqtt`, `ai`, `fixture` → `converter` → `binding` → `controller` → `web` → `cli`
