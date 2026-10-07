# Spec: Device Persistence (data/devices.yaml)

## Objective

Provide persistent local file storage for paired Zigbee devices and user-defined configurations (such as custom **Friendly Names**, models, manufacturers, endpoints, and cluster metadata) in a clean, human-readable YAML file (`data/devices.yaml`).

### The Problem
Currently, the `DeviceRegistry` stores device records exclusively in-memory (`sync.RWMutex` map). When ZigBridge restarts:
1. User-customized **Friendly Names** (e.g. "Living Room Ceiling Light") are lost and revert to raw IEEE hex strings until manually renamed again.
2. Device cluster metadata (endpoints, input clusters, output clusters) is discarded, requiring nodes to be re-interviewed over the air.
3. Home Assistant MQTT discovery entities must wait for devices to broadcast active radio frames before entities appear in Home Assistant.

### The Solution
Implement a thread-safe persistence layer that:
1. Loads existing devices from `data/devices.yaml` on startup into the `DeviceRegistry`.
2. Immediately publishes Home Assistant MQTT Auto-Discovery topics for all persisted devices so entities are available in Home Assistant immediately upon boot.
3. Persists device additions, metadata updates, and user renames atomically to disk (`.tmp` file + `os.Rename`) with a configurable write debouncer to protect flash storage (e.g., SD cards on Raspberry Pi / eMMC).
4. Maintains human-editable formatting so users can inspect or batch-edit device names directly in YAML.

---

## Tech Stack

- **Language**: Go 1.27+ (`go.mod`)
- **YAML Parser**: `gopkg.in/yaml.v3` (already in `go.mod`, zero additional external dependencies)
- **Concurrency & I/O**: `sync.RWMutex`, `sync.Once`, atomic file renaming via Go standard library `os` and `path/filepath`.

---

## Commands

```bash
# Run unit tests with race detector
make test

# Run static analysis (go vet + golangci-lint)
make lint

# Compile standalone static binary
make build

# Run application locally
make run
```

---

## Project Structure

```
zigbridge/
├── config.yaml.dist                 # Configuration template committed to version control
├── data/                            # Persistent runtime data directory (git-ignored, single backup target)
│   ├── config.yaml                  # Runtime configuration with local credentials
│   └── devices.yaml                 # Persisted device registry and direct bindings
├── specs/
│   ├── zigbridge.md
│   ├── ui-simplification.md
│   ├── documentation-overhaul.md
│   ├── device-persistence.md        # This specification
│   └── CAPABILITY-MAP.md
├── internal/
│   ├── config/
│   │   ├── config.go                # StorageConfig struct (devices_path, debounce)
│   │   └── config_test.go
│   ├── controller/
│   │   ├── controller.go            # Bootstraps store, loads on startup, triggers saves
│   │   ├── device.go                # Device YAML struct tags and clone helpers
│   │   ├── store.go                 # Storage engine: atomic read/write, debouncing, serialization
│   │   └── store_test.go            # Unit tests for loading, saving, corrupted files, and concurrency
│   └── web/                         # Renaming endpoints trigger persistent save
└── .gitignore                       # Ignore data/ directory
```

---

## Configuration Schema

In `data/config.yaml` (with fallback to `config.yaml`):

```yaml
storage:
  # Path to the persistent devices storage file
  devices_path: "data/devices.yaml"

  # Debounce duration to batch rapid updates and protect flash storage
  debounce_interval: 2s
```

---

## Storage File Format (`data/devices.yaml`)

The YAML file is structured as a map keyed by 64-bit IEEE address for O(1) human readability and easy manual editing:

```yaml
# ==============================================================================
# ZigBridge Device Registry
# Automatically generated and synchronized. Human-editable.
# ==============================================================================

version: 1
devices:
  "0x00158D0001D45A2B":
    friendly_name: "Living Room Light"
    model: "LED1732G11"
    manufacturer: "IKEA of Sweden"
    nwk: 0x4A21
    endpoints:
      - 1
    input_clusters:
      - 0x0000 # Basic
      - 0x0006 # On/Off
      - 0x0008 # Level Control
      - 0x0300 # Color Control
    output_clusters:
      - 0x0019 # OTA
    battery: 0
    last_seen: "2026-10-06T10:45:00Z"

  "0x00158D0002E391C0":
    friendly_name: "Kitchen Wall Remote"
    model: "E1743"
    manufacturer: "IKEA of Sweden"
    nwk: 0x1B82
    endpoints:
      - 1
    input_clusters:
      - 0x0000 # Basic
      - 0x0001 # Power Configuration
    output_clusters:
      - 0x0006 # On/Off
      - 0x0008 # Level Control
    battery: 95
    last_seen: "2026-10-06T10:48:12Z"

bindings:
  - id: "0x00158D0002E391C0:1->0x0006->0x00158D0001D45A2B:1"
    src_ieee: "0x00158D0002E391C0"
    src_endpoint: 1
    cluster_id: 0x0006
    cluster_name: "On/Off"
    dst_ieee: "0x00158D0001D45A2B"
    dst_endpoint: 1
    status: "active"
    created_at: "2026-10-06T11:00:00Z"
```

---

## Code Style & Architectural Implementation

### Canonical Implementation: Atomic File Writing & Debouncing

```go
package controller

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// DeviceStore handles thread-safe, atomic persistence for Zigbee devices.
type DeviceStore struct {
	mu           sync.Mutex
	filePath     string
	debounce     time.Duration
	timer        *time.Timer
	pendingSave  bool
	registryRef  *DeviceRegistry
}

// NewDeviceStore initializes a storage manager.
func NewDeviceStore(filePath string, debounce time.Duration, registry *DeviceRegistry) *DeviceStore {
	return &DeviceStore{
		filePath:    filePath,
		debounce:    debounce,
		registryRef: registry,
	}
}

// SaveAtomic writes the current registry state to a temporary file, then renames it.
func (s *DeviceStore) SaveAtomic() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create storage dir %s: %w", dir, err)
	}

	devices := s.registryRef.GetAll()
	data := PersistedRegistry{
		Version: 1,
		Devices: make(map[string]*PersistedDevice, len(devices)),
	}

	for _, d := range devices {
		data.Devices[d.IEEE] = &PersistedDevice{
			IEEE:           d.IEEE,
			FriendlyName:   d.FriendlyName,
			Model:          d.Model,
			Manufacturer:   d.Manufacturer,
			NWK:            d.NWK,
			Endpoints:      d.Endpoints,
			InputClusters:  d.InputClusters,
			OutputClusters: d.OutputClusters,
			Battery:        d.Battery,
			LastSeen:       d.LastSeen,
		}
	}

	raw, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal devices to yaml: %w", err)
	}

	tmpFile := s.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, raw, 0644); err != nil {
		return fmt.Errorf("failed to write tmp storage file: %w", err)
	}

	if err := os.Rename(tmpFile, s.filePath); err != nil {
		return fmt.Errorf("failed to replace storage file atomically: %w", err)
	}

	return nil
}

// ScheduleSave triggers a debounced atomic write to disk.
func (s *DeviceStore) ScheduleSave() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.debounce == 0 {
		_ = s.SaveAtomic()
		return
	}

	if s.timer != nil {
		s.timer.Stop()
	}

	s.timer = time.AfterFunc(s.debounce, func() {
		_ = s.SaveAtomic()
	})
}
```

---

## Testing Strategy

1. **Unit Tests (`internal/controller/store_test.go`)**:
   - `TestStoreLoadAndSave`: Verify saving devices writes valid YAML, and loading rehydrates all fields correctly.
   - `TestStoreAtomicFileWrite`: Ensure atomic rename avoids leaving partial `.tmp` files.
   - `TestStoreMissingFile`: Verify clean startup with an empty registry when `data/devices.yaml` does not yet exist.
   - `TestStoreCorruptFile`: Verify robust error handling if YAML syntax is malformed (reports error without crashing).
   - `TestStoreDebouncing`: Verify multiple rapid mutations only cause a single disk write within the debounce window.
   - `TestStoreConcurrentAccess`: Run with `-race` to ensure concurrent reads and saves do not race.
2. **Controller Integration**:
   - Verify `controller.Start()` loads existing devices and immediately broadcasts MQTT discovery messages for them.
   - Verify `POST /api/devices/{ieee}/rename` updates the in-memory registry and commits to `data/devices.yaml`.

---

## Boundaries

- **Always**:
  - Write atomically via `.tmp` file and `os.Rename` to protect against sudden power loss or process kill.
  - Automatically create the parent directory (`data/`) if it does not exist.
  - Add `data/` to `.gitignore` so user device registries and network addresses are not checked into git.
  - Ensure all unit tests pass with `make test` and linter passes with `make lint`.
- **Ask First**:
  - Changing the storage format from YAML to JSON or an embedded database (e.g. SQLite / BoltDB).
  - Persisting high-frequency ephemeral telemetry (LQI, dynamic power measurements) to disk on every packet.
- **Never**:
  - Block radio frame processing or event loops on disk I/O operations (file writes must be asynchronous/debounced).
  - Overwrite or corrupt existing `devices.yaml` if loading fails due to syntax errors (backup file instead).

---

## Success Criteria

1. On clean start without `data/devices.yaml`, the system starts normally and creates `data/devices.yaml` upon the first paired device or rename.
2. Renaming a device via `POST /api/devices/{ieee}/rename` updates `data/devices.yaml` with the new friendly name.
3. Restarting the ZigBridge process restores all saved devices and friendly names into memory.
4. Paired devices saved in `data/devices.yaml` automatically re-publish their Home Assistant MQTT discovery topics on bridge boot.
5. `data/` is excluded by `.gitignore`.
6. `make test` passes with zero race conditions (`-race`).
7. `make lint` passes with 0 issues.

---

## Decisions & Resolved Requirements

1. **Static Metadata vs Ephemeral State**:
   - Persist identity (`ieee`, `nwk`), friendly names, model, manufacturer, endpoints, and cluster configuration (`input_clusters`, `output_clusters`), plus battery percentage.
   - Do NOT persist rapidly fluctuating sensor telemetry (`state`, dynamic power, temperature, LQI) to avoid excessive flash write wear.
2. **Direct Hardware Bindings**:
   - Persist active direct mesh bindings under the `bindings:` key in `data/devices.yaml` so the bridge restores binding table state across restarts.
