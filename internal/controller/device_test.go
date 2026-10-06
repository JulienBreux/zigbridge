package controller_test

import (
	"slices"
	"testing"

	"github.com/julienbreux/zigbridge/internal/controller"
)

func TestDeviceRegistryGetAllSortedByFriendlyName(t *testing.T) {
	reg := controller.NewDeviceRegistry()

	devices := []*controller.Device{
		{
			IEEE:         "0x00158D0003",
			FriendlyName: "Zebra Ceiling Lamp",
		},
		{
			IEEE:         "0x00158D0001",
			FriendlyName: "apple Motion Sensor", // lowercase 'a'
		},
		{
			IEEE:         "0x00158D0004",
			FriendlyName: "Banana Smart Plug",
		},
		{
			IEEE:         "0x00158D0002",
			FriendlyName: "", // Fallback to IEEE
		},
		{
			IEEE:         "0x00158D0005",
			FriendlyName: "apple Motion Sensor", // Same name, tie-break by IEEE
		},
	}

	for _, d := range devices {
		reg.Upsert(d)
	}

	all := reg.GetAll()
	if len(all) != len(devices) {
		t.Fatalf("expected %d devices, got %d", len(devices), len(all))
	}

	// Expected order:
	// 1. "0x00158D0002" (fallback IEEE starting with '0')
	// 2. "apple Motion Sensor" (IEEE 0x00158D0001)
	// 3. "apple Motion Sensor" (IEEE 0x00158D0005)
	// 4. "Banana Smart Plug"
	// 5. "Zebra Ceiling Lamp"
	expectedIEEEs := []string{
		"0x00158D0002",
		"0x00158D0001",
		"0x00158D0005",
		"0x00158D0004",
		"0x00158D0003",
	}

	gotIEEEs := make([]string, len(all))
	for i, d := range all {
		gotIEEEs[i] = d.IEEE
	}

	if !slices.Equal(gotIEEEs, expectedIEEEs) {
		t.Fatalf("expected device order %v, got %v", expectedIEEEs, gotIEEEs)
	}
}
