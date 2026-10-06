package controller

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/julienbreux/zigbridge/internal/binding"
	"github.com/julienbreux/zigbridge/internal/zcl"
	"gopkg.in/yaml.v3"
)

// PersistedDevice contains the static and configuration attributes of a Zigbee device.
// Ephemeral sensor telemetry (such as raw state values, temperature, power) is omitted to reduce flash wear.
type PersistedDevice struct {
	IEEE           string          `yaml:"ieee,omitempty"`
	FriendlyName   string          `yaml:"friendly_name"`
	Model          string          `yaml:"model,omitempty"`
	Manufacturer   string          `yaml:"manufacturer,omitempty"`
	NWK            uint16          `yaml:"nwk"`
	Endpoints      []uint16        `yaml:"endpoints,omitempty"`
	InputClusters  []zcl.ClusterID `yaml:"input_clusters,omitempty"`
	OutputClusters []zcl.ClusterID `yaml:"output_clusters,omitempty"`
	Battery        uint8           `yaml:"battery,omitempty"`
	LastSeen       time.Time       `yaml:"last_seen,omitempty"`
}

// PersistedData represents the on-disk format of data/devices.yaml.
type PersistedData struct {
	Version  int                         `yaml:"version"`
	Devices  map[string]*PersistedDevice `yaml:"devices"`
	Bindings []*binding.Binding          `yaml:"bindings,omitempty"`
}

// DeviceStore handles thread-safe, atomic persistence for Zigbee devices and bindings.
type DeviceStore struct {
	mu          sync.Mutex
	filePath    string
	debounce    time.Duration
	timer       *time.Timer
	registryRef *DeviceRegistry
	bindingsRef *binding.Engine
}

// NewDeviceStore initializes a storage manager.
func NewDeviceStore(filePath string, debounce time.Duration, registry *DeviceRegistry, bindings *binding.Engine) *DeviceStore {
	if filePath == "" {
		filePath = "data/devices.yaml"
	}
	return &DeviceStore{
		filePath:    filePath,
		debounce:    debounce,
		registryRef: registry,
		bindingsRef: bindings,
	}
}

// Load reads and parses devices and bindings from disk, populating the registry and binding engine.
func (s *DeviceStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("failed to read devices storage file %s: %w", s.filePath, err)
	}

	var stored PersistedData
	if err := yaml.Unmarshal(data, &stored); err != nil {
		return fmt.Errorf("failed to parse devices storage yaml %s: %w", s.filePath, err)
	}

	// Restore devices into registry
	if s.registryRef != nil && stored.Devices != nil {
		for ieee, pDev := range stored.Devices {
			if pDev == nil {
				continue
			}
			devIEEE := ieee
			if pDev.IEEE != "" {
				devIEEE = pDev.IEEE
			}
			friendlyName := pDev.FriendlyName
			if friendlyName == "" {
				friendlyName = devIEEE
			}

			dev := &Device{
				IEEE:           devIEEE,
				NWK:            pDev.NWK,
				FriendlyName:   friendlyName,
				Model:          pDev.Model,
				Manufacturer:   pDev.Manufacturer,
				Endpoints:      pDev.Endpoints,
				InputClusters:  pDev.InputClusters,
				OutputClusters: pDev.OutputClusters,
				Battery:        pDev.Battery,
				Available:      true,
				LastSeen:       pDev.LastSeen,
				State:          make(map[string]interface{}),
			}
			if pDev.Battery > 0 {
				dev.State["battery"] = pDev.Battery
			}

			s.registryRef.Upsert(dev)
		}
	}

	// Restore direct bindings into engine
	if s.bindingsRef != nil && stored.Bindings != nil {
		for _, b := range stored.Bindings {
			if b == nil {
				continue
			}
			if b.ID == "" {
				b.ID = binding.GenerateID(b.SrcIEEE, b.SrcEndpoint, b.ClusterID, b.DstIEEE, b.DstEndpoint)
			}
			if b.Status == "" {
				b.Status = "active"
			}
			s.bindingsRef.RestoreBinding(b)
		}
	}

	return nil
}

// SaveAtomic writes the current registry state and direct bindings to a temporary file,
// then replaces the destination file atomically via os.Rename.
func (s *DeviceStore) SaveAtomic() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory %s: %w", dir, err)
	}

	var devices []*Device
	if s.registryRef != nil {
		devices = s.registryRef.GetAll()
	}

	var bindingsList []*binding.Binding
	if s.bindingsRef != nil {
		bindingsList = s.bindingsRef.ListBindings()
	}

	stored := PersistedData{
		Version:  1,
		Devices:  make(map[string]*PersistedDevice, len(devices)),
		Bindings: bindingsList,
	}

	for _, d := range devices {
		stored.Devices[d.IEEE] = &PersistedDevice{
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

	raw, err := yaml.Marshal(stored)
	if err != nil {
		return fmt.Errorf("failed to marshal persisted devices: %w", err)
	}

	tmpFile := s.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, raw, 0644); err != nil {
		return fmt.Errorf("failed to write temporary devices file %s: %w", tmpFile, err)
	}

	if err := os.Rename(tmpFile, s.filePath); err != nil {
		return fmt.Errorf("failed to replace devices storage file atomically: %w", err)
	}

	return nil
}

// ScheduleSave triggers an asynchronous write to disk after the configured debounce interval.
func (s *DeviceStore) ScheduleSave() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.debounce <= 0 {
		go func() {
			_ = s.SaveAtomic()
		}()
		return
	}

	if s.timer != nil {
		s.timer.Stop()
	}

	s.timer = time.AfterFunc(s.debounce, func() {
		_ = s.SaveAtomic()
	})
}

// Flush cancels any pending debounce timer and immediately performs an atomic write to disk.
func (s *DeviceStore) Flush() error {
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.mu.Unlock()

	return s.SaveAtomic()
}
