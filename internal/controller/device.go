package controller

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/julienbreux/zigbridge/internal/ai"
	"github.com/julienbreux/zigbridge/internal/zcl"
)

// Device represents a Zigbee node registered on the network.
type Device struct {
	IEEE           string                 `json:"ieee"`
	NWK            uint16                 `json:"nwk"`
	FriendlyName   string                 `json:"friendly_name"`
	Model          string                 `json:"model"`
	Manufacturer   string                 `json:"manufacturer"`
	Endpoints      []uint16               `json:"endpoints"`
	InputClusters  []zcl.ClusterID        `json:"input_clusters"`  // Server clusters (controllable)
	OutputClusters []zcl.ClusterID        `json:"output_clusters"` // Client clusters (controllers/sensors)
	State          map[string]interface{} `json:"state"`
	LQI            uint8                  `json:"lqi"`
	Battery        uint8                  `json:"battery"`
	Available      bool                   `json:"available"`
	LastSeen       time.Time              `json:"last_seen"`
}

// ToSnapshot converts a Device to an AI DeviceSnapshot for topology analysis.
func (d *Device) ToSnapshot() ai.DeviceSnapshot {
	stateCopy := make(map[string]interface{}, len(d.State))
	for k, v := range d.State {
		stateCopy[k] = v
	}

	return ai.DeviceSnapshot{
		IEEE:           d.IEEE,
		FriendlyName:   d.FriendlyName,
		Model:          d.Model,
		Manufacturer:   d.Manufacturer,
		Endpoints:      d.Endpoints,
		InputClusters:  d.InputClusters,
		OutputClusters: d.OutputClusters,
		State:          stateCopy,
		LQI:            d.LQI,
		LastSeen:       d.LastSeen,
	}
}

// Clone returns a deep copy of the Device for safe concurrent consumption.
func (d *Device) Clone() *Device {
	if d == nil {
		return nil
	}
	cp := *d
	if d.Endpoints != nil {
		cp.Endpoints = append([]uint16(nil), d.Endpoints...)
	}
	if d.InputClusters != nil {
		cp.InputClusters = append([]zcl.ClusterID(nil), d.InputClusters...)
	}
	if d.OutputClusters != nil {
		cp.OutputClusters = append([]zcl.ClusterID(nil), d.OutputClusters...)
	}
	if d.State != nil {
		cp.State = make(map[string]interface{}, len(d.State))
		for k, v := range d.State {
			cp.State[k] = v
		}
	}
	return &cp
}

// DeviceRegistry provides concurrency-safe storage and retrieval for Zigbee devices.
type DeviceRegistry struct {
	mu      sync.RWMutex
	devices map[string]*Device
}

// NewDeviceRegistry creates an empty registry.
func NewDeviceRegistry() *DeviceRegistry {
	return &DeviceRegistry{
		devices: make(map[string]*Device),
	}
}

// findDeviceLocked searches for a device by IEEE, friendly name, or 16-bit NWK address.
// Caller MUST hold r.mu (either RLock or Lock).
func (r *DeviceRegistry) findDeviceLocked(key string) *Device {
	if key == "" {
		return nil
	}
	// 1. Direct match by IEEE key
	if d, ok := r.devices[key]; ok {
		return d
	}

	// 2. Parse key if it is in NWK hex format: "0x1234" or "1234"
	var searchNWK uint16
	hasNWK := false
	trimmed := strings.TrimPrefix(strings.ToLower(key), "0x")
	if len(trimmed) <= 4 && len(trimmed) > 0 {
		if val, err := strconv.ParseUint(trimmed, 16, 16); err == nil {
			searchNWK = uint16(val)
			hasNWK = true
		}
	}

	// 3. Scan registered devices
	for _, d := range r.devices {
		if strings.EqualFold(d.IEEE, key) {
			return d
		}
		if strings.EqualFold(d.FriendlyName, key) {
			return d
		}
		if hasNWK && d.NWK == searchNWK {
			return d
		}
	}
	return nil
}

// Get looks up a device by its 64-bit IEEE address, friendly name, or NWK hex address and returns a safe clone.
func (r *DeviceRegistry) Get(key string) (*Device, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d := r.findDeviceLocked(key)
	if d == nil {
		return nil, false
	}
	return d.Clone(), true
}

// GetAll returns a slice of all registered devices as safe clones.
func (r *DeviceRegistry) GetAll() []*Device {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Device, 0, len(r.devices))
	for _, d := range r.devices {
		result = append(result, d.Clone())
	}
	return result
}

// Upsert registers or updates an existing device in the registry.
func (r *DeviceRegistry) Upsert(d *Device) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing := r.findDeviceLocked(d.IEEE)
	if existing != nil {
		if d.FriendlyName == "" || d.FriendlyName == d.IEEE {
			if existing.FriendlyName != "" {
				d.FriendlyName = existing.FriendlyName
			}
		}
		if d.Manufacturer == "" {
			d.Manufacturer = existing.Manufacturer
		}
		if d.Model == "" {
			d.Model = existing.Model
		}
		if len(d.Endpoints) == 0 && len(existing.Endpoints) > 0 {
			d.Endpoints = existing.Endpoints
		}
		if len(d.InputClusters) == 0 && len(existing.InputClusters) > 0 {
			d.InputClusters = existing.InputClusters
		}
		if len(d.OutputClusters) == 0 && len(existing.OutputClusters) > 0 {
			d.OutputClusters = existing.OutputClusters
		}
		if len(d.State) == 0 {
			d.State = existing.State
		}
	}

	if d.FriendlyName == "" {
		d.FriendlyName = d.IEEE
	}
	if d.State == nil {
		d.State = make(map[string]interface{})
	}

	r.devices[d.IEEE] = d
}

// SetFriendlyName updates the human-readable identifier for a device.
func (r *DeviceRegistry) SetFriendlyName(key, name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if d := r.findDeviceLocked(key); d != nil {
		d.FriendlyName = name
		return true
	}
	return false
}

// UpdateState merges new state attributes into the device record.
func (r *DeviceRegistry) UpdateState(key string, updates map[string]interface{}, lqi uint8) (*Device, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	d := r.findDeviceLocked(key)
	if d == nil {
		return nil, false
	}

	d.LQI = lqi
	d.LastSeen = time.Now().UTC()

	for k, v := range updates {
		d.State[k] = v
		if k == "battery" {
			if bat, ok := v.(uint8); ok {
				d.Battery = bat
			}
		}
	}
	return d.Clone(), true
}

// UpdateMetadata updates endpoints and clusters for a registered device in a concurrency-safe manner.
func (r *DeviceRegistry) UpdateMetadata(key string, endpoints []uint16, inClusters, outClusters []zcl.ClusterID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	d := r.findDeviceLocked(key)
	if d == nil {
		return false
	}
	if endpoints != nil {
		d.Endpoints = append([]uint16(nil), endpoints...)
	}
	if inClusters != nil {
		d.InputClusters = append([]zcl.ClusterID(nil), inClusters...)
	}
	if outClusters != nil {
		d.OutputClusters = append([]zcl.ClusterID(nil), outClusters...)
	}
	return true
}

// UpdateModelInfo updates the manufacturer and model for a registered device.
func (r *DeviceRegistry) UpdateModelInfo(key, manufacturer, model string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	d := r.findDeviceLocked(key)
	if d == nil {
		return false
	}
	if manufacturer != "" {
		d.Manufacturer = manufacturer
	}
	if model != "" {
		d.Model = model
	}
	return true
}

