# Configuration Reference

ZigBridge is configured via a single YAML file (searched in order: `data/config.yaml`, `config.yaml`, `config.yaml.dist`). A starter template is provided in [`config.yaml.dist`](../config.yaml.dist).

```bash
# Initialize your local config from the template
make config   # copies config.yaml.dist to data/config.yaml

# Run ZigBridge
./bin/zigbridge
```

> [!TIP]
> Both `data/` and `config.yaml` are listed in `.gitignore` by default so your MQTT passwords, network keys, and private credentials are never accidentally committed to version control, and all runtime persistence remains cleanly contained in `data/` for easy backups.

---

## Complete `config.yaml` Schema

```yaml
# Logging verbosity: debug, info, warn, error
log_level: info

# ==============================================================================
# Coordinator Transport Connection
# ==============================================================================
transport:
  # Type of transport connection:
  # - "tcp": Network-attached coordinators (SMLIGHT SLZB-06, TubeZB)
  # - "serial": Local USB dongles (Sonoff-P, Sonoff-E, SkyConnect)
  # - "mock": In-memory virtual simulator for development/tests
  type: tcp

  # TCP Network Socket Endpoint (used when type: tcp)
  url: "tcp://192.168.1.50:6638"

  # Serial Device Node (used when type: serial)
  port: "/dev/ttyUSB0"

  # Serial Baud Rate (115200 for TI CC2652; 115200 or 230400 for EZSP)
  baudrate: 115200

  # Network Reconnection & Keepalive Settings (optimized for SLZB-06)
  reconnect_interval: 2s
  max_reconnect_interval: 30s
  tcp_keepalive: 10s
  rfc2217: true                     # Filter Telnet RFC2217 negotiation sequences

  read_timeout: 10s
  write_timeout: 5s

# ==============================================================================
# Radio Coprocessor Adapter & Zigbee Network Parameters
# ==============================================================================
adapter:
  # Radio adapter driver type:
  # - "zstack": Texas Instruments Z-Stack 3.x (CC2652P, SLZB-06, Sonoff-P)
  # - "ember": Silicon Labs EmberZNet / EZSP v8+ (EFR32MG21, SLZB-06M, Sonoff-E)
  # - "mock": Virtual simulation adapter
  type: zstack

  # 16-bit Zigbee PAN ID (0x0001 to 0xFFFE)
  pan_id: 0x1A62

  # 64-bit Extended PAN ID (hexadecimal)
  ext_pan_id: "0xDDDDDDDDDDDDDDDD"

  # Zigbee 2.4 GHz Channel (11 - 26, recommended 15, 20, or 25)
  channel: 20

  # 16-byte Transport Key / Network Encryption Key (hexadecimal)
  network_key: "01030507090B0D0F00020406080A0C0D"

# ==============================================================================
# Network Pairing & Join Behavior
# ==============================================================================
network:
  # Default duration in seconds when permit-join is triggered (max 254)
  permit_join_duration: 254

  # Whether to automatically open network join window upon startup
  permit_join_on_start: false

# ==============================================================================
# MQTT Broker & Home Assistant Auto-Discovery
# ==============================================================================
mqtt:
  enabled: true
  broker: "tcp://localhost:1883"
  client_id: "zigbridge"
  username: ""
  password: ""

  # Root MQTT topic for state publishing and bridge telemetry
  base_topic: "zigbridge"

  # Home Assistant MQTT discovery (automatically creates entities in HA)
  ha_discovery: true
  ha_discovery_prefix: "homeassistant"

  retain: true
  qos: 0
  connection_timeout: 10s

# ==============================================================================
# Embedded Web Dashboard & REST / WebSocket API
# ==============================================================================
web:
  # Listening address and port for the embedded web interface
  listen_addr: "0.0.0.0:8080"

  # Cross-Origin Resource Sharing (CORS) support
  enable_cors: true

# ==============================================================================
# AI Telemetry Logger & On-Demand Recommendation Engine
# ==============================================================================
ai:
  # Enable telemetry event buffer and smart binding recommendations (disabled by default)
  enabled: false

  # Engine type:
  # - "rule_based": Fast, offline heuristics based on traffic and cluster types
  # - "external_llm": Inspects topology via local/remote LLM endpoint
  engine: "rule_based"

  # Minimum confidence threshold (0.0 to 1.0) for proposals
  min_confidence: 0.75

  # Bounded ring-buffer size for device event telemetry
  max_event_history: 2000

  # External LLM endpoint settings (optional, used if engine: external_llm)
  llm_endpoint: ""                  # e.g. "http://localhost:11434/v1/chat/completions"
  llm_api_key: ""

# ==============================================================================
# Persistent Storage & Data Directory
# ==============================================================================
storage:
  # Path to the persistent devices storage file (stores friendly names & bindings)
  devices_path: "data/devices.yaml"

  # Debounce duration to batch rapid updates and protect flash storage
  debounce_interval: 2s
```

---

## Example Setups

### Example A: SMLIGHT SLZB-06 over Ethernet / Wi-Fi
```yaml
transport:
  type: tcp
  url: "tcp://192.168.1.50:6638"
  reconnect_interval: 2s
  max_reconnect_interval: 30s
  tcp_keepalive: 10s
  rfc2217: true

adapter:
  type: zstack
  pan_id: 0x1A62
  channel: 25
```

### Example B: Sonoff USB Dongle-P (Linux USB)
```yaml
transport:
  type: serial
  port: "/dev/serial/by-id/usb-Silicon_Labs_Sonoff_Zigbee_3.0_USB_Dongle_Plus-if00-port0"
  baudrate: 115200

adapter:
  type: zstack
  pan_id: 0x1A62
  channel: 20
```

### Example C: Offline Developer / CI Simulation
```yaml
transport:
  type: mock

adapter:
  type: mock

mqtt:
  enabled: false

web:
  listen_addr: "127.0.0.1:8080"
```
