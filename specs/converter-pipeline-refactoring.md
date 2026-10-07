# Spec & Blueprint: Declarative Converter Pipeline Refactoring

## 1. Executive Summary & Objective

As ZigBridge expands its catalog of supported Zigbee devices (such as Tuya energy meters, LiXee TIC teleinformation sensors, IKEA multi-button remotes, Develco alarm sirens, and Aqara sensors), routing raw ZCL cluster frames directly inside `internal/controller/controller.go` creates a monolithic maintenance bottleneck.

This specification details the refactoring strategy to transition `internal/controller` from hardcoded cluster switch-cases to a **modular, declarative, zero-allocation Converter Pipeline** (`internal/converter`) inspired by modern Zigbee architectures while adhering to Go idioms and ZigBridge's strict performance targets (`<25 MB` RSS, zero heap churn via buffer pooling).

---

## 2. Current Architecture & Bottlenecks

### 2.1 The Current `controller.go` Monolith
- **Overloaded Responsibility**: `HandleIncomingFrame` in `internal/controller/controller.go` currently performs:
  1. ZCL frame header decoding and transaction tracking.
  2. Cluster-specific decoding (`Basic`, `OnOff`, `ElectricalMeasurement`, `Metering`, `Temperature`, `RelativeHumidity`, `Occupancy`, `IASZone`).
  3. Multi-endpoint routing hacks (e.g. hardcoded `PJ-1203A` endpoint 1 -> phase A, endpoint 2 -> phase B).
  4. Device definition lookup and simulated action prefix matching.
  5. Fallback command parsing (`IASACE`, `LevelControl`, `Scenes`).
  6. In-memory state mutation and storage debouncing.
  7. Telemetry recording for AI ring buffers.
  8. MQTT state publication and EventBus broadcasting.
- **Maintenance Cost**: Adding a new device with proprietary attributes (e.g. LiXee TIC Linky meters or Tuya `0xEF00` DP clusters) requires modifying `controller.go` directly, risking regressions in core routing logic.
- **Testing Inefficiency**: Testing cluster decoding requires spinning up the complete `Controller` with `MockAdapter`, `MockTransport`, and `MockClient`.

---

## 3. Target Architecture: `internal/converter` Pipeline

```
                                    +---------------------------+
                                    |     Radio Transport       |
                                    +-------------+-------------+
                                                  |
                                                  v
                                    +---------------------------+
                                    |      adapter.Adapter      |
                                    +-------------+-------------+
                                                  |
                                                  v
                                    +---------------------------+
                                    |        *zcl.Frame         |
                                    +-------------+-------------+
                                                  |
                                                  v
                                    +---------------------------+
                                    |   controller.Controller   |
                                    +-------------+-------------+
                                                  |
                                                  v
                             +-----------------------------------------+
                             |       converter.Pipeline                |
                             +--------------------+--------------------+
                                                  |
                 +--------------------------------+--------------------------------+
                 |                                                                 |
                 v                                                                 v
+---------------------------------+                               +---------------------------------+
| Standard Cluster Converters     |                               | Device / Vendor Quirk Overrides |
| (internal/converter/fromzigbee) |                               | (internal/converter/quirks)     |
| - OnOff                         |                               | - Tuya PJ-1203A Dual Clamp      |
| - Electrical & Metering         |                               | - LiXee ZLinky TIC Meter        |
| - Temperature & Humidity        |                               | - IKEA STYRBAR / SOMRIG         |
| - IAS Zone & IAS ACE            |                               | - Develco KEYZB / SIRZB         |
+---------------------------------+                               +---------------------------------+
                                                  |
                                                  v
                                    +---------------------------+
                                    |  stateUpdates: map[string]|
                                    |             any           |
                                    +-------------+-------------+
                                                  |
                         +------------------------+------------------------+
                         |                        |                        |
                         v                        v                        v
             +-----------------------+ +--------------------+ +-------------------------+
             |   controller.Device   | |      MQTT Bus      | |      EventBus / AI      |
             |       Registry        | |  State & Discovery | |    Telemetry Buffer     |
             +-----------------------+ +--------------------+ +-------------------------+
```

---

## 4. Component Specification

### 4.1 Interface Definitions (`internal/converter/converter.go`)

```go
package converter

import (
	"github.com/julienbreux/zigbridge/internal/zcl"
)

// DeviceContext provides read-only device metadata to converters.
type DeviceContext struct {
	IEEE         string
	NWK          uint16
	FriendlyName string
	Model        string
	Manufacturer string
	Endpoints    []uint16
}

// InboundConverter converts incoming ZCL frames into normalized key-value state updates.
type InboundConverter interface {
	// ID returns a unique identifier for the converter (e.g. "fz_on_off", "fz_pj1203a").
	ID() string

	// Matches returns true if this converter can process the given frame for this device.
	Matches(frame *zcl.Frame, dev DeviceContext) bool

	// Convert parses the frame payload and populates stateUpdates.
	// Returns true if the frame was recognized and handled.
	Convert(frame *zcl.Frame, dev DeviceContext, stateUpdates map[string]any) (bool, error)
}

// OutboundConverter converts high-level state commands (e.g. {"state": "ON"}) into ZCL command frames.
type OutboundConverter interface {
	// ID returns a unique identifier for the outbound converter (e.g. "tz_on_off").
	ID() string

	// Matches returns true if this converter can handle the requested property key.
	Matches(key string, dev DeviceContext) bool

	// Convert creates one or more ZCL command frames to transmit.
	Convert(key string, val any, dev DeviceContext) ([]*zcl.Frame, error)
}
```

### 4.2 Standard Inbound Converters (`internal/converter/fromzigbee/`)

Each standard cluster decoder is encapsulated in its own file with dedicated unit tests:
1. `on_off.go`: Global attribute reports (`AttrOnOff`) and cluster commands (`CmdOnOffOff`, `CmdOnOffOn`, `CmdOnOffToggle`).
2. `electrical.go`: Global attribute reports for `ClusterElectricalMeasurement` (`ActivePower`, `RMSVoltage`, `RMSCurrent`) and `ClusterMetering` (`CurrentSummationDelivered`).
3. `environment.go`: Global attribute reports for `ClusterTemperatureMeasurement` and `ClusterRelativeHumidity` (with `/ 100.0` scaling).
4. `occupancy.go`: Global attribute reports for `ClusterOccupancySensing`.
5. `ias_zone.go`: IAS Zone attribute reports (`AttrZoneStatus`) and status change notification commands (`CmdIASZoneStatusChangeNotification`). Inverts alarm bit according to device type (contact vs leak vs smoke).
6. `ias_ace.go`: Keypad security commands (`CmdIASACEArm`, `CmdIASACEEmergency`, `CmdIASACEPanic`, `CmdIASACEFire`).
7. `level_control.go` & `scenes.go`: Remote dimming and arrow click commands.

### 4.3 Vendor Quirk Converters (`internal/converter/quirks/`)

Specialized device behavior is isolated from generic cluster logic:
1. `tuya_pj1203a.go`:
   - Inspects `frame.SourceEndpoint`.
   - Endpoint 1 -> maps to `power_a`, `current_a`, `energy_a`.
   - Endpoint 2 -> maps to `power_b`, `current_b`, `energy_b`.
   - Automatically sums total active power into `power_ab`.
2. `lixee_zlinky.go`:
   - Decodes French Linky TIC electricity attributes (tariff options, base index `BASE`, peak/off-peak indices `HCHP`/`HCHC`, apparent power `PAPP`).
3. `ikea_styrbar_somrig.go`:
   - Handles payload-differentiated multi-action commands where cluster and command IDs are identical.

---

## 5. Declarative HA Auto-Discovery Generator

Currently, `publishDeviceDiscovery` in `controller.go` uses hardcoded if-checks. 
The refactored design leverages the device definition's declarative `Exposes` list:

```go
func (c *Controller) publishDeviceDiscovery(dev *Device) {
	// 1. Core device information
	haDev := mqtt.HADevice{
		Identifiers:  []string{dev.IEEE},
		Name:         cmp.Or(dev.FriendlyName, dev.IEEE),
		Model:        cmp.Or(dev.Model, "Zigbee Device"),
		Manufacturer: cmp.Or(dev.Manufacturer, "ZigBridge"),
	}

	// 2. Fetch definition from fixture registry
	if def, ok := c.fixtures.Get(dev.Model); ok {
		for _, exp := range def.Device.Exposes {
			if cfg, ok := discovery.FromExpose(haDev, dev.IEEE, c.cfg.MQTT.BaseTopic, exp); ok {
				_ = c.mqtt.PublishDiscovery(cfg)
			}
		}
		return
	}

	// 3. Fallback to cluster introspection if no fixture definition exists
	discovery.FromClusters(haDev, dev.IEEE, c.cfg.MQTT.BaseTopic, dev.InputClusters, dev.OutputClusters, c.mqtt)
}
```

This guarantees that whenever a new fixture is imported or defined, Home Assistant discovery entities (sensors, binary sensors, switches, device triggers) are synthesized **automatically** without requiring any Go code changes.

---

## 6. Implementation & Migration Phases

### Phase 1: Package Scaffolding & Standard Decoders
- Create `internal/converter` package.
- Implement `InboundConverter` interface and registry engine.
- Migrate generic decoders (`on_off.go`, `electrical.go`, `environment.go`, `ias_zone.go`, `ias_ace.go`).
- Write 100% unit test coverage for each decoder.

### Phase 2: Vendor Quirks & Multi-Endpoint Converters
- Implement `tuya_pj1203a.go`, `ikea_remotes.go`, `lixee_zlinky.go`.
- Write dedicated tests for endpoint routing and payload disambiguation.

### Phase 3: Controller Integration
- Replace cluster switch block in `Controller.HandleIncomingFrame` with a single call to `c.converterPipeline.Execute(frame, devContext, stateUpdates)`.
- Verify full backward compatibility with `make test` and `make lint`.

### Phase 4: Declarative Discovery Engine
- Migrate `publishDeviceDiscovery` into `internal/mqtt/discovery` to generate entities dynamically from `definition.Exposes`.

---

## 7. Performance & Memory Budget

| Metric | Target | Verification Method |
|---|---|---|
| RSS Memory | `< 25 MB` | Real-time memory profiling under continuous load |
| Frame Processing Allocations | `0 B / op` | `testing.B` benchmark with `b.ReportAllocs()` |
| Frame Throughput | `> 50,000 frames/sec` | Benchmarks on synthetic radio buffers |
| Concurrency Safety | Data race free | `go test -v -race ./...` |
