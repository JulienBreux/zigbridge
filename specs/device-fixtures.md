# Spec: Device Definitions & Test Fixtures (Zigbee2MQTT Format)

## Objective

Provide the capability to define, import, and simulate realistic Zigbee devices in test suites, mock environments, and local development using declarative definitions modeled directly after the **Zigbee2MQTT device format** (`zigbee-herdsman-converters`).

### The Reference Device: SONOFF SNZB-01P
The initial target device is the **[SONOFF SNZB-01P Wireless Button](https://www.zigbee2mqtt.io/devices/SNZB-01P.html)**:
- **Vendor / Manufacturer**: `SONOFF`
- **Model Identifier**: `SNZB-01P`
- **Description**: `Wireless button`
- **Exposes**:
  - `action`: Button press events with enum values `["single", "double", "long"]`
  - `battery`: Remaining battery level percentage (`0`–`100%`)
  - `voltage`: Battery terminal voltage in millivolts (`2500`–`3200 mV`)
  - `linkquality`: Radio link signal quality (`0`–`255 LQI`)
- **Clusters & Endpoints**:
  - **Endpoint**: `1`
  - **Input (Server) Clusters**: `0x0000` (Basic), `0x0001` (PowerConfiguration), `0x0003` (Identify), `0x0020` (PollControl)
  - **Output (Client) Clusters**: `0x0006` (OnOff)
- **Direct Binding Note**: Hardware sends momentary command frames or attribute reports; direct binding to light clusters requires specific receiver compatibility.

### Problem Statement
Currently, `internal/adapter/mock` and test suites instantiate synthetic hardcoded devices with static endpoints and generic on/off clusters. There is no mechanism to:
1. Define rich device profiles (buttons with multi-actions, multi-sensor clusters, battery voltage curves).
2. Add arbitrary devices from Zigbee2MQTT device documentation without modifying Go source code.
3. Test end-to-end event flows for battery, button actions, and Home Assistant MQTT Auto-Discovery using authentic real-world device signatures.

### The Solution
1. **Declarative Device Definition Schema**: Create a standardized YAML/JSON schema reflecting Zigbee2MQTT device definitions (`model`, `vendor`, `description`, `endpoints`, `input_clusters`, `output_clusters`, `exposes`, and `simulated_actions`).
2. **Device Fixture Loader & Embedded Library**: Implement a fixture loader (`internal/fixture`) with embedded standard device templates (`SNZB-01P`, IKEA TRADFRI, Sonoff sensors) and support for user-supplied fixtures from a configurable directory (`fixtures/devices/*.yaml`).
3. **Simulated Device Test Driver**: Enable tests and the mock adapter to instantiate virtual devices from these definitions, with helpers to trigger authentic actions (e.g., `dev.TriggerAction("single")`, `dev.ReportBattery(95, 3000)`).
4. **Mock API Test Endpoints (Development & UI Testing)**: Expose test APIs under `/api/test/devices` in mock mode so developers can spawn devices and inject clicks or sensor events directly from the web dashboard or CLI.

---

## Tech Stack

- **Language**: Go 1.27+ (`go.mod`)
- **Schema & Serialization**: `gopkg.in/yaml.v3` and `encoding/json`
- **Embedded Templates**: Go standard library `embed.FS`
- **Testing**: `go test -v -race ./...`

---

## Commands

```bash
# Run unit and race tests
make test

# Run static analysis and linting
make lint

# Compile static binary
make build

# Run Zigbridge with mock adapter to interact with test devices
./bin/zigbridge -config data/config.yaml
```

---

## Declarative Device Definition Format

Every device fixture file represents one device definition matching the Zigbee2MQTT model format.

### Schema Definition: `fixtures/devices/sonoff_snzb_01p.yaml`

```yaml
schema_version: "1.0"
device:
  model: "SNZB-01P"
  vendor: "SONOFF"
  description: "Wireless button"
  zigbee_models:
    - "SNZB-01P"
  
  endpoints:
    - endpoint: 1
      profile_id: 0x0104 # Zigbee Home Automation (ZHA)
      device_id: 0x0401  # Non-Color Controller
      input_clusters:
        - 0x0000 # genBasic
        - 0x0001 # genPowerCfg
        - 0x0003 # genIdentify
        - 0x0020 # genPollCtrl
      output_clusters:
        - 0x0006 # genOnOff

  exposes:
    - type: "enum"
      name: "action"
      property: "action"
      description: "Triggered button press action"
      values:
        - "single"
        - "double"
        - "long"
      access: 1 # Read/Report

    - type: "numeric"
      name: "battery"
      property: "battery"
      unit: "%"
      min: 0
      max: 100
      description: "Remaining battery percentage"
      access: 1

    - type: "numeric"
      name: "voltage"
      property: "voltage"
      unit: "mV"
      min: 2500
      max: 3200
      description: "Battery voltage in millivolts"
      access: 1

    - type: "numeric"
      name: "linkquality"
      property: "linkquality"
      unit: "lqi"
      min: 0
      max: 255
      description: "Radio link quality indicator"
      access: 1

  # Test simulation behaviors: maps high-level test actions to raw ZCL frames
  simulations:
    actions:
      single:
        cluster: 0x0006
        command: 0x02 # Toggle / Command or custom attribute report
        payload: []
        mqtt_payload:
          action: "single"
      double:
        cluster: 0x0006
        command: 0x02
        payload: []
        mqtt_payload:
          action: "double"
      long:
        cluster: 0x0006
        command: 0x00 # Off / Custom long press indication
        payload: []
        mqtt_payload:
          action: "long"

    telemetry:
      battery:
        cluster: 0x0001 # genPowerCfg
        attribute: 0x0021 # BatteryPercentageRemaining (half percent, 0-200)
        voltage_attribute: 0x0020 # BatteryVoltage (in 100mV units)
```

---

## Project Structure

```
zigbridge/
├── fixtures/
│   └── devices/
│       ├── sonoff_snzb_01p.yaml     # SONOFF SNZB-01P reference definition
│       ├── ikea_tradfri_bulb.yaml   # IKEA TRADFRI E27 CWS bulb reference
│       └── sonoff_snzb_02p.yaml     # SONOFF temperature & humidity sensor reference
├── internal/
│   ├── fixture/
│   │   ├── definition.go            # DeviceDefinition, Expose, Simulation models
│   │   ├── loader.go                # YAML/JSON loader, embed.FS integration, validation
│   │   ├── virtual_device.go        # Simulated device instance & state machine
│   │   ├── embedded/                # Embedded default fixtures (embed.FS)
│   │   └── fixture_test.go          # Unit tests verifying definition parsing and simulation
│   ├── adapter/
│   │   └── mock/
│   │       ├── mock.go              # Updated to support spawning devices from definitions
│   │       └── mock_test.go
│   ├── controller/
│   │   ├── controller.go            # Handles device join and frame emission from fixtures
│   │   └── controller_test.go       # End-to-end tests using SNZB-01P fixture
│   └── web/
│       ├── server.go                # Optional test routes (enabled in mock/test mode)
│       └── static/                  # Test simulation controls in UI (when mock adapter active)
└── specs/
    ├── CAPABILITY-MAP.md
    ├── device-fixtures.md           # This specification
    ├── device-persistence.md
    └── zigbridge.md
```

---

## Architectural Design

### 1. The Definition Model (`internal/fixture/definition.go`)

```go
package fixture

// DeviceDefinition models a device catalog specification from Zigbee2MQTT.
type DeviceDefinition struct {
    SchemaVersion string              `yaml:"schema_version" json:"schema_version"`
    Device        DeviceMeta          `yaml:"device" json:"device"`
}

type DeviceMeta struct {
    Model         string              `yaml:"model" json:"model"`
    Vendor        string              `yaml:"vendor" json:"vendor"`
    Description   string              `yaml:"description" json:"description"`
    ZigbeeModels  []string            `yaml:"zigbee_models" json:"zigbee_models"`
    Endpoints     []EndpointDef       `yaml:"endpoints" json:"endpoints"`
    Exposes       []ExposeDef         `yaml:"exposes" json:"exposes"`
    Simulations   SimulationDef       `yaml:"simulations" json:"simulations"`
}

type EndpointDef struct {
    Endpoint       uint8              `yaml:"endpoint" json:"endpoint"`
    ProfileID      uint16             `yaml:"profile_id" json:"profile_id"`
    DeviceID       uint16             `yaml:"device_id" json:"device_id"`
    InputClusters  []uint16           `yaml:"input_clusters" json:"input_clusters"`
    OutputClusters []uint16           `yaml:"output_clusters" json:"output_clusters"`
}

type ExposeDef struct {
    Type        string                `yaml:"type" json:"type"` // enum, numeric, binary, text
    Name        string                `yaml:"name" json:"name"`
    Property    string                `yaml:"property" json:"property"`
    Description string                `yaml:"description" json:"description"`
    Unit        string                `yaml:"unit,omitempty" json:"unit,omitempty"`
    Values      []string              `yaml:"values,omitempty" json:"values,omitempty"`
    Min         *float64              `yaml:"min,omitempty" json:"min,omitempty"`
    Max         *float64              `yaml:"max,omitempty" json:"max,omitempty"`
    Access      uint8                 `yaml:"access" json:"access"`
}

type SimulationDef struct {
    Actions   map[string]ActionSim    `yaml:"actions" json:"actions"`
    Telemetry map[string]TelemetrySim `yaml:"telemetry" json:"telemetry"`
}

type ActionSim struct {
    Cluster     uint16                 `yaml:"cluster" json:"cluster"`
    Command     uint8                  `yaml:"command" json:"command"`
    MQTTPayload map[string]interface{} `yaml:"mqtt_payload" json:"mqtt_payload"`
}

type TelemetrySim struct {
    Cluster          uint16            `yaml:"cluster" json:"cluster"`
    Attribute        uint16            `yaml:"attribute" json:"attribute"`
    VoltageAttribute uint16            `yaml:"voltage_attribute,omitempty" json:"voltage_attribute,omitempty"`
}
```

### 2. Device Loader (`internal/fixture/loader.go`)

```go
type Registry struct {
    definitions map[string]*DeviceDefinition
}

// LoadFromDir loads all .yaml and .json files in the specified directory.
func (r *Registry) LoadFromDir(dir string) error

// LoadEmbedded loads the standard built-in definitions bundled into the binary.
func (r *Registry) LoadEmbedded() error

// Get finds a definition by model name or zigbee_model identifier.
func (r *Registry) Get(model string) (*DeviceDefinition, bool)
```

### 3. Virtual Device Simulator (`internal/fixture/virtual_device.go`)

```go
type VirtualDevice struct {
    Def      *DeviceDefinition
    IEEE     string
    NWK      uint16
    adapter  adapter.Adapter
    state    map[string]interface{}
}

// Spawn creates a new virtual device on the mock network and emits a DeviceJoin event.
func Spawn(def *DeviceDefinition, ieee string, nwk uint16, adp adapter.Adapter) *VirtualDevice

// TriggerAction simulates a button click (e.g. "single", "double", "long").
func (v *VirtualDevice) TriggerAction(actionName string) error

// ReportBattery simulates a battery telemetry report.
func (v *VirtualDevice) ReportBattery(percentage uint8, voltageMV uint16) error
```

---

## Testing Strategy

1. **Unit Testing (`internal/fixture/fixture_test.go`)**:
   - Verify parsing of `sonoff_snzb_01p.yaml` into Go structures.
   - Verify validation rules: required model, vendor, valid endpoint numbers, and exposes.
   - Verify lookup by model name (`SNZB-01P`) and alias.

2. **Integration Testing with Mock Adapter (`internal/controller/controller_test.go`)**:
   - Instantiate a `Controller` backed by `MockAdapter`.
   - Spawn a virtual `SNZB-01P` device.
   - Verify device registration in `DeviceRegistry` with exact vendor (`SONOFF`), model (`SNZB-01P`), endpoints (`[1]`), and clusters.
   - Call `dev.TriggerAction("single")`:
     - Assert MQTT publishes payload `{"action": "single"}` to `zigbridge/SNZB-01P`.
     - Assert AI Event Collector records the interaction.
     - Assert WebSocket broadcast receives the event.
   - Call `dev.ReportBattery(90, 3000)`:
     - Assert device battery updates to `90` in registry and persists to `data/devices.yaml`.

3. **Home Assistant Discovery Testing (`internal/mqtt/mqtt_test.go`)**:
   - Verify HA discovery payload for `SNZB-01P`:
     - Generates device trigger entities for `single`, `double`, and `long` button presses.
     - Generates sensor entity for battery (`%`, device_class `battery`).
     - Generates sensor entity for voltage (`mV`, device_class `voltage`).

---

## Boundaries

- **Always Do**:
  - Keep device definitions strictly declarative (YAML/JSON) with zero external network calls at runtime.
  - Embed reference definitions (`embed.FS`) so tests and mock adapters function out of the box with zero setup.
  - Verify all tests pass with `make test` (race detector enabled) and zero linter warnings (`make lint`).
- **Ask First**:
  - Adding network-based scraping or automated web downloading of Zigbee2MQTT device pages at runtime.
  - Modifying the core `Device` schema in `internal/controller/device.go`.
- **Never Do**:
  - Never require Node.js or JavaScript runtime dependencies to interpret `zigbee-herdsman-converters`.
  - Never hardcode vendor-specific button logic inside the core controller; all behavior must flow through the cluster/attribute reporting pipeline.

---

## Success Criteria

1. **Declarative SNZB-01P Fixture**: A clean YAML definition for `SNZB-01P` adhering to the Zigbee2MQTT format exists under `fixtures/devices/sonoff_snzb_01p.yaml`.
2. **Multi-Device Extensibility**: Any new device from `https://www.zigbee2mqtt.io/devices/` can be added by simply dropping a new YAML definition file into `fixtures/devices/` without compiling or modifying Go source code.
3. **Programmatic Test Helpers**: Go test suites can instantiate simulated devices (`fixture.Spawn`) and invoke actions (`TriggerAction("single")`, `ReportBattery(...)`) in unit and integration tests.
4. **End-to-End Event Validation**: In automated tests, triggering an action on the virtual `SNZB-01P` successfully routes through ZCL parsing, Device Registry updates, MQTT publishing, and persistent store debouncing.
5. **Quality Gates**: `make test` and `make lint` pass with 100% green status.

---

## Open Questions

1. **CLI Import Helper**: Would you like a utility command (e.g. `zigbridge fixture import --url <zigbee2mqtt-url>` or `--json <file>`) to automatically generate the YAML fixture file, or do you prefer curating fixture YAMLs by hand?
2. **Web UI Test Controls**: When running Zigbridge in mock mode, should the web dashboard include a "Simulate Device" panel allowing you to click "Single Press", "Double Press", or "Long Press" on the virtual SNZB-01P directly in the browser?
