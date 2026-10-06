package controller

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/julienbreux/zigbridge/internal/adapter/mock"
	"github.com/julienbreux/zigbridge/internal/binding"
	"github.com/julienbreux/zigbridge/internal/zcl"
)

func TestStoreMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "devices.yaml")

	reg := NewDeviceRegistry()
	mockAdp := mock.New(11, 0x1234)
	bEngine := binding.NewEngine(mockAdp)

	store := NewDeviceStore(storePath, 50*time.Millisecond, reg, bEngine)

	// Loading non-existent file should succeed without error
	if err := store.Load(); err != nil {
		t.Fatalf("expected nil error on missing file, got %v", err)
	}

	if len(reg.GetAll()) != 0 {
		t.Errorf("expected 0 devices, got %d", len(reg.GetAll()))
	}
}

func TestStoreLoadAndSave(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "data", "devices.yaml")

	reg := NewDeviceRegistry()
	mockAdp := mock.New(11, 0x1234)
	bEngine := binding.NewEngine(mockAdp)

	dev1 := &Device{
		IEEE:           "0x00158D0001D45A2B",
		NWK:            0x4A21,
		FriendlyName:   "Living Room Ceiling Light",
		Model:          "LED1732G11",
		Manufacturer:   "IKEA of Sweden",
		Endpoints:      []uint16{1},
		InputClusters:  []zcl.ClusterID{zcl.ClusterBasic, zcl.ClusterOnOff, zcl.ClusterLevelControl},
		OutputClusters: []zcl.ClusterID{zcl.ClusterOTAUpgrade},
		Battery:        0,
		State:          map[string]interface{}{"state": "ON"},
		LastSeen:       time.Date(2026, 10, 6, 10, 45, 0, 0, time.UTC),
	}

	dev2 := &Device{
		IEEE:           "0x00158D0002E391C0",
		NWK:            0x1B82,
		FriendlyName:   "Kitchen Switch",
		Model:          "E1743",
		Manufacturer:   "IKEA of Sweden",
		Endpoints:      []uint16{1},
		InputClusters:  []zcl.ClusterID{zcl.ClusterBasic, zcl.ClusterPowerConfiguration},
		OutputClusters: []zcl.ClusterID{zcl.ClusterOnOff},
		Battery:        95,
		State:          map[string]interface{}{"battery": uint8(95)},
		LastSeen:       time.Date(2026, 10, 6, 10, 48, 12, 0, time.UTC),
	}

	reg.Upsert(dev1)
	reg.Upsert(dev2)

	b1 := &binding.Binding{
		ID:          "0x00158D0002E391C0:1->0x0006->0x00158D0001D45A2B:1",
		SrcIEEE:     "0x00158D0002E391C0",
		SrcEndpoint: 1,
		ClusterID:   zcl.ClusterOnOff,
		ClusterName: "On/Off",
		DstIEEE:     "0x00158D0001D45A2B",
		DstEndpoint: 1,
		Status:      "active",
		CreatedAt:   time.Date(2026, 10, 6, 11, 0, 0, 0, time.UTC),
	}
	bEngine.RestoreBinding(b1)

	store := NewDeviceStore(storePath, 50*time.Millisecond, reg, bEngine)

	// Save to disk
	if err := store.SaveAtomic(); err != nil {
		t.Fatalf("failed to save devices atomically: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(storePath); err != nil {
		t.Fatalf("devices storage file not found: %v", err)
	}

	// Create a new registry and engine to simulate restart
	newReg := NewDeviceRegistry()
	newBEngine := binding.NewEngine(mockAdp)
	newStore := NewDeviceStore(storePath, 50*time.Millisecond, newReg, newBEngine)

	if err := newStore.Load(); err != nil {
		t.Fatalf("failed to load saved devices: %v", err)
	}

	loadedDevs := newReg.GetAll()
	if len(loadedDevs) != 2 {
		t.Fatalf("expected 2 devices loaded, got %d", len(loadedDevs))
	}

	loadedD1, ok := newReg.Get("0x00158D0001D45A2B")
	if !ok {
		t.Fatal("device 1 not found in loaded registry")
	}
	if loadedD1.FriendlyName != "Living Room Ceiling Light" {
		t.Errorf("expected FriendlyName 'Living Room Ceiling Light', got '%s'", loadedD1.FriendlyName)
	}
	if loadedD1.Model != "LED1732G11" {
		t.Errorf("expected Model 'LED1732G11', got '%s'", loadedD1.Model)
	}
	if len(loadedD1.InputClusters) != 3 {
		t.Errorf("expected 3 input clusters, got %d", len(loadedD1.InputClusters))
	}

	loadedD2, ok := newReg.Get("0x00158D0002E391C0")
	if !ok {
		t.Fatal("device 2 not found in loaded registry")
	}
	if loadedD2.Battery != 95 {
		t.Errorf("expected Battery 95, got %d", loadedD2.Battery)
	}

	loadedBindings := newBEngine.ListBindings()
	if len(loadedBindings) != 1 {
		t.Fatalf("expected 1 binding loaded, got %d", len(loadedBindings))
	}
	if loadedBindings[0].ID != b1.ID {
		t.Errorf("expected binding ID '%s', got '%s'", b1.ID, loadedBindings[0].ID)
	}
}

func TestStoreAtomicFileWrite(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "devices.yaml")

	reg := NewDeviceRegistry()
	reg.Upsert(&Device{
		IEEE:         "0x00124B001CA12345",
		FriendlyName: "Atomic Test Device",
		NWK:          0x1234,
	})

	store := NewDeviceStore(storePath, 10*time.Millisecond, reg, nil)
	if err := store.SaveAtomic(); err != nil {
		t.Fatalf("SaveAtomic failed: %v", err)
	}

	// Verify target file exists
	if _, err := os.Stat(storePath); err != nil {
		t.Fatalf("target file %s does not exist: %v", storePath, err)
	}

	// Verify temporary file does NOT remain
	tmpFile := storePath + ".tmp"
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Fatalf("temporary file %s should have been cleaned up/renamed", tmpFile)
	}
}

func TestStoreCorruptFile(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "devices.yaml")

	// Write invalid YAML
	if err := os.WriteFile(storePath, []byte("version: 1\ndevices: [invalid: yaml:"), 0644); err != nil {
		t.Fatalf("failed to write corrupt file: %v", err)
	}

	reg := NewDeviceRegistry()
	store := NewDeviceStore(storePath, 10*time.Millisecond, reg, nil)

	if err := store.Load(); err == nil {
		t.Fatal("expected error loading corrupt YAML, got nil")
	}
}

func TestStoreDebouncing(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "devices.yaml")

	reg := NewDeviceRegistry()
	store := NewDeviceStore(storePath, 80*time.Millisecond, reg, nil)

	// Rapidly call ScheduleSave
	for i := 0; i < 5; i++ {
		reg.Upsert(&Device{
			IEEE:         "0x00124B001CA12345",
			FriendlyName: "Debounced Device",
			NWK:          uint16(i),
		})
		store.ScheduleSave()
	}

	// Immediately check: file should NOT exist yet due to debounce
	if _, err := os.Stat(storePath); !os.IsNotExist(err) {
		t.Fatal("file should not exist immediately during debounce window")
	}

	// Wait for debounce interval to pass
	time.Sleep(150 * time.Millisecond)

	// Now file must exist
	if _, err := os.Stat(storePath); err != nil {
		t.Fatalf("file should exist after debounce interval: %v", err)
	}

	// Verify Flush immediately flushes pending timer
	reg.Upsert(&Device{
		IEEE:         "0x00124B001CA12345",
		FriendlyName: "Flushed Device",
	})
	store.ScheduleSave()
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	newReg := NewDeviceRegistry()
	newStore := NewDeviceStore(storePath, 10*time.Millisecond, newReg, nil)
	if err := newStore.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	dev, _ := newReg.Get("0x00124B001CA12345")
	if dev.FriendlyName != "Flushed Device" {
		t.Errorf("expected 'Flushed Device', got '%s'", dev.FriendlyName)
	}
}

func TestStoreConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "devices.yaml")

	reg := NewDeviceRegistry()
	mockAdp := mock.New(11, 0x1234)
	bEngine := binding.NewEngine(mockAdp)
	store := NewDeviceStore(storePath, 20*time.Millisecond, reg, bEngine)

	var wg sync.WaitGroup
	workers := 10
	iterations := 20

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				ieee := "0x00124B001CA1000" + string(rune('0'+workerID))
				reg.Upsert(&Device{
					IEEE:         ieee,
					FriendlyName: "Worker Device",
					NWK:          uint16(workerID*100 + i),
				})
				store.ScheduleSave()
				_ = reg.GetAll()
			}
		}(w)
	}

	wg.Wait()
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	newReg := NewDeviceRegistry()
	newStore := NewDeviceStore(storePath, 20*time.Millisecond, newReg, nil)
	if err := newStore.Load(); err != nil {
		t.Fatalf("Load after concurrent access failed: %v", err)
	}

	if len(newReg.GetAll()) != workers {
		t.Errorf("expected %d devices after concurrent writes, got %d", workers, len(newReg.GetAll()))
	}
}
