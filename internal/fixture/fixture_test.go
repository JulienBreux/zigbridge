package fixture_test

import (
	"path/filepath"
	"testing"

	"github.com/julienbreux/zigbridge/internal/adapter"
	"github.com/julienbreux/zigbridge/internal/fixture"
	"github.com/julienbreux/zigbridge/internal/zcl"
)

type mockFrameEmitter struct {
	joins  []adapter.DeviceJoinInfo
	frames []*zcl.Frame
}

func (m *mockFrameEmitter) EmitDeviceJoin(info adapter.DeviceJoinInfo) {
	m.joins = append(m.joins, info)
}

func (m *mockFrameEmitter) EmitFrame(frame *zcl.Frame) {
	m.frames = append(m.frames, frame)
}

func TestRegistryEmbeddedLoading(t *testing.T) {
	reg := fixture.NewRegistry()
	if err := reg.LoadEmbedded(); err != nil {
		t.Fatalf("failed to load embedded fixtures: %v", err)
	}

	defs := reg.List()
	if len(defs) < 3 {
		t.Fatalf("expected at least 3 embedded definitions, got %d", len(defs))
	}

	// Lookup SNZB-01P by model name and alias
	def, ok := reg.Get("SNZB-01P")
	if !ok {
		t.Fatalf("expected to find SNZB-01P definition")
	}
	if def.Device.Vendor != "SONOFF" {
		t.Errorf("expected vendor SONOFF, got %s", def.Device.Vendor)
	}

	// Lookup by alias
	aliasDef, ok := reg.Get("WB01")
	if !ok {
		t.Fatalf("expected to find SNZB-01P by alias WB01")
	}
	if aliasDef.Device.Model != def.Device.Model {
		t.Errorf("alias definition mismatch")
	}

	// Case-insensitivity check
	lowerDef, ok := reg.Get("snzb-01p")
	if !ok || lowerDef.Device.Model != "SNZB-01P" {
		t.Errorf("case-insensitive lookup failed")
	}
}

func TestRegistryLoadFromDir(t *testing.T) {
	reg := fixture.NewRegistry()
	dir := filepath.Join("..", "..", "fixtures", "devices")
	if err := reg.LoadFromDir(dir); err != nil {
		t.Fatalf("failed to load from dir %s: %v", dir, err)
	}

	defs := reg.List()
	if len(defs) < 3 {
		t.Fatalf("expected at least 3 definitions from directory, got %d", len(defs))
	}

	if _, ok := reg.Get("LED1624G9"); !ok {
		t.Errorf("expected to find IKEA bulb LED1624G9")
	}
	if _, ok := reg.Get("SNZB-02P"); !ok {
		t.Errorf("expected to find SONOFF sensor SNZB-02P")
	}
}

func TestDefinitionValidation(t *testing.T) {
	tests := []struct {
		name    string
		def     fixture.DeviceDefinition
		wantErr bool
	}{
		{
			name: "missing model",
			def: fixture.DeviceDefinition{
				Device: fixture.DeviceMeta{
					Vendor: "Vendor",
					Endpoints: []fixture.EndpointDef{
						{Endpoint: 1},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing vendor",
			def: fixture.DeviceDefinition{
				Device: fixture.DeviceMeta{
					Model: "Model",
					Endpoints: []fixture.EndpointDef{
						{Endpoint: 1},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing endpoints",
			def: fixture.DeviceDefinition{
				Device: fixture.DeviceMeta{
					Model:  "Model",
					Vendor: "Vendor",
				},
			},
			wantErr: true,
		},
		{
			name: "valid definition",
			def: fixture.DeviceDefinition{
				Device: fixture.DeviceMeta{
					Model:  "Model",
					Vendor: "Vendor",
					Endpoints: []fixture.EndpointDef{
						{Endpoint: 1},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.def.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVirtualDeviceSimulation(t *testing.T) {
	reg := fixture.NewRegistry()
	if err := reg.LoadEmbedded(); err != nil {
		t.Fatalf("failed to load embedded fixtures: %v", err)
	}

	def, ok := reg.Get("SNZB-01P")
	if !ok {
		t.Fatalf("SNZB-01P not found")
	}

	emitter := &mockFrameEmitter{}
	vdev := fixture.NewVirtualDevice(def, "0x00124b00226b8899", 0x1234, emitter)

	// 1. SimulateJoin
	if err := vdev.SimulateJoin(); err != nil {
		t.Fatalf("SimulateJoin failed: %v", err)
	}

	if len(emitter.joins) != 1 {
		t.Fatalf("expected 1 join event, got %d", len(emitter.joins))
	}
	if emitter.joins[0].IEEE != "0x00124b00226b8899" {
		t.Errorf("join IEEE mismatch: %s", emitter.joins[0].IEEE)
	}

	if len(emitter.frames) != 1 {
		t.Fatalf("expected 1 basic frame, got %d", len(emitter.frames))
	}
	if emitter.frames[0].ClusterID != zcl.ClusterBasic {
		t.Errorf("expected ClusterBasic, got 0x%04x", emitter.frames[0].ClusterID)
	}

	// 2. TriggerAction - single
	if err := vdev.TriggerAction("single"); err != nil {
		t.Fatalf("TriggerAction single failed: %v", err)
	}
	if len(emitter.frames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(emitter.frames))
	}
	lastFrame := emitter.frames[1]
	if lastFrame.ClusterID != zcl.ClusterOnOff {
		t.Errorf("expected ClusterOnOff, got 0x%04x", lastFrame.ClusterID)
	}
	if lastFrame.CommandID != 0x02 { // Toggle
		t.Errorf("expected Toggle command 0x02, got 0x%02x", lastFrame.CommandID)
	}

	// 3. TriggerAction - invalid
	if err := vdev.TriggerAction("non_existent_action"); err == nil {
		t.Errorf("expected error for undefined action, got nil")
	}

	// 4. ReportBattery
	if err := vdev.ReportBattery(80, 2900); err != nil {
		t.Fatalf("ReportBattery failed: %v", err)
	}
	if len(emitter.frames) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(emitter.frames))
	}
	batteryFrame := emitter.frames[2]
	if batteryFrame.ClusterID != zcl.ClusterPowerConfiguration {
		t.Errorf("expected ClusterPowerConfiguration, got 0x%04x", batteryFrame.ClusterID)
	}

	// 5. ReportTemperatureHumidity
	if err := vdev.ReportTemperatureHumidity(22.5, 48.0); err != nil {
		t.Fatalf("ReportTemperatureHumidity failed: %v", err)
	}
	if len(emitter.frames) != 5 {
		t.Fatalf("expected 5 frames, got %d", len(emitter.frames))
	}

	state := vdev.GetState()
	if state["action"] != "single" {
		t.Errorf("expected action single in state, got %v", state["action"])
	}
	if state["battery"] != uint8(80) {
		t.Errorf("expected battery 80 in state, got %v", state["battery"])
	}
	if state["voltage"] != uint16(2900) {
		t.Errorf("expected voltage 2900 in state, got %v", state["voltage"])
	}
	if state["temperature"] != 22.5 {
		t.Errorf("expected temperature 22.5 in state, got %v", state["temperature"])
	}
	if state["humidity"] != 48.0 {
		t.Errorf("expected humidity 48.0 in state, got %v", state["humidity"])
	}
}
