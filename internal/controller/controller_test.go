package controller_test

import (
	"encoding/binary"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/julienbreux/zigbridge/internal/adapter"
	"github.com/julienbreux/zigbridge/internal/adapter/mock"
	"github.com/julienbreux/zigbridge/internal/ai"
	"github.com/julienbreux/zigbridge/internal/config"
	"github.com/julienbreux/zigbridge/internal/controller"
	"github.com/julienbreux/zigbridge/internal/mqtt"
	"github.com/julienbreux/zigbridge/internal/transport"
	"github.com/julienbreux/zigbridge/internal/zcl"
)

func setupTestControllerWithConfig(t *testing.T, cfg *config.Config) (*controller.Controller, *mock.MockAdapter, *mqtt.MockClient) {
	if cfg.Storage.DevicesPath == "" || cfg.Storage.DevicesPath == "data/devices.yaml" {
		cfg.Storage.DevicesPath = filepath.Join(t.TempDir(), "devices.yaml")
	}
	mockTrans, _ := transport.NewMockTransport()
	mockAdp := mock.New(20, 0x1A62)
	mockMQTT := mqtt.NewMockClient()

	ctrl := controller.New(cfg, mockTrans, mockAdp, mockMQTT)
	t.Cleanup(func() {
		_ = ctrl.Stop()
	})
	return ctrl, mockAdp, mockMQTT
}

func setupTestController(t *testing.T) (*controller.Controller, *mock.MockAdapter, *mqtt.MockClient) {
	cfg := config.Default()
	cfg.Storage.DevicesPath = filepath.Join(t.TempDir(), "devices.yaml")
	cfg.MQTT.Enabled = true
	return setupTestControllerWithConfig(t, cfg)
}

func TestControllerLifecycleAndStatus(t *testing.T) {
	ctrl, _, _ := setupTestController(t)
	ctx := t.Context()

	if err := ctrl.Start(ctx); err != nil {
		t.Fatalf("failed to start controller: %v", err)
	}

	status := ctrl.Status()
	if !status.Connected {
		t.Error("expected connected status")
	}
	if status.Coordinator.Channel != 20 {
		t.Errorf("expected channel 20, got %d", status.Coordinator.Channel)
	}
	if status.AIEnabled {
		t.Error("expected AIEnabled to be false by default")
	}

	if err := ctrl.Stop(); err != nil {
		t.Fatalf("failed to stop controller: %v", err)
	}
}

func TestControllerPermitJoin(t *testing.T) {
	ctrl, _, _ := setupTestController(t)
	ctx := t.Context()
	_ = ctrl.Start(ctx)

	busCh := ctrl.EventBus().Subscribe(10)
	defer ctrl.EventBus().Unsubscribe(busCh)

	if err := ctrl.PermitJoin(ctx, 3); err != nil {
		t.Fatalf("failed to permit join: %v", err)
	}

	status := ctrl.Status()
	if status.PermitJoinRemaining == 0 {
		t.Error("expected permit join remaining > 0")
	}

	// Close permit join immediately
	if err := ctrl.PermitJoin(ctx, 0); err != nil {
		t.Fatalf("failed to cancel permit join: %v", err)
	}

	status = ctrl.Status()
	if status.PermitJoinRemaining != 0 {
		t.Errorf("expected 0 remaining, got %d", status.PermitJoinRemaining)
	}
}

func TestControllerIncomingFrameAndMQTT(t *testing.T) {
	ctrl, mockAdp, mockMQTT := setupTestController(t)
	ctx := t.Context()
	_ = ctrl.Start(ctx)

	// Simulate device join
	mockAdp.EmitDeviceJoin(adapter.DeviceJoinInfo{
		IEEE: "0x00158D0001",
		NWK:  0x1234,
	})

	time.Sleep(50 * time.Millisecond)
	devs := ctrl.GetDevices()
	if len(devs) != 1 {
		t.Fatalf("expected 1 registered device, got %d", len(devs))
	}

	// Rename device
	if !ctrl.SetDeviceFriendlyName("0x00158D0001", "Living Room Sensor") {
		t.Error("failed to set friendly name")
	}

	// Send an attribute report frame for On/Off = ON
	reportPayload := []byte{0x00, 0x00, zcl.TypeBoolean, 0x01}
	frame := &zcl.Frame{
		Header: zcl.FrameControl{
			Type:      zcl.FrameTypeGlobal,
			Direction: zcl.DirectionServerToClient,
		},
		ClusterID:              zcl.ClusterOnOff,
		CommandID:              zcl.CmdReportAttributes,
		SourceAddress:          "0x00158D0001",
		SourceEndpoint:         1,
		LQI:                    180,
		Payload:                reportPayload,
		TransactionSequenceNum: 1,
	}

	mockAdp.EmitFrame(frame)
	time.Sleep(50 * time.Millisecond)

	// Check device state
	dev, ok := ctrl.GetDevice("0x00158D0001")
	if !ok || dev.State["state"] != "ON" {
		t.Errorf("expected device state ON, got: %+v", dev.State)
	}
	if dev.LQI != 180 {
		t.Errorf("expected LQI 180, got %d", dev.LQI)
	}

	// Verify MQTT published state
	msgs := mockMQTT.GetMessages()
	foundMqttState := false
	for _, m := range msgs {
		if m.Topic == "zigbridge/0x00158D0001" {
			foundMqttState = true
			break
		}
	}
	if !foundMqttState {
		t.Error("expected MQTT state message for device")
	}

	// Verify telemetry is NOT recorded in AI EventCollector when AI is disabled
	if ctrl.EventCollector().Count() != 0 {
		t.Errorf("expected 0 telemetry events in collector when AI is disabled, got %d", ctrl.EventCollector().Count())
	}
}

func TestControllerIncomingFrameWithAIEnabled(t *testing.T) {
	cfg := config.Default()
	cfg.Storage.DevicesPath = filepath.Join(t.TempDir(), "devices.yaml")
	cfg.MQTT.Enabled = true
	cfg.AI.Enabled = true

	ctrl, mockAdp, _ := setupTestControllerWithConfig(t, cfg)
	ctx := t.Context()
	_ = ctrl.Start(ctx)

	mockAdp.EmitDeviceJoin(adapter.DeviceJoinInfo{
		IEEE: "0x00158D0001",
		NWK:  0x1234,
	})
	time.Sleep(50 * time.Millisecond)

	reportPayload := []byte{0x00, 0x00, zcl.TypeBoolean, 0x01}
	frame := &zcl.Frame{
		Header: zcl.FrameControl{
			Type:      zcl.FrameTypeGlobal,
			Direction: zcl.DirectionServerToClient,
		},
		ClusterID:              zcl.ClusterOnOff,
		CommandID:              zcl.CmdReportAttributes,
		SourceAddress:          "0x00158D0001",
		SourceEndpoint:         1,
		LQI:                    180,
		Payload:                reportPayload,
		TransactionSequenceNum: 1,
	}

	mockAdp.EmitFrame(frame)
	time.Sleep(50 * time.Millisecond)

	if ctrl.EventCollector().Count() < 1 {
		t.Errorf("expected at least 1 telemetry event in collector when AI enabled, got %d", ctrl.EventCollector().Count())
	}
}

func TestControllerOptimisticDirectBinding(t *testing.T) {
	ctrl, _, _ := setupTestController(t)
	ctx := t.Context()
	_ = ctrl.Start(ctx)

	// Direct binding between two addresses not yet registered
	req := adapter.BindRequest{
		SrcIEEE:     "0x00158D0001AABBCC",
		SrcEndpoint: 1,
		ClusterID:   zcl.ClusterOnOff,
		DstIEEE:     "0x00158D0002CCDDEE",
		DstEndpoint: 1,
	}

	b, warnings, err := ctrl.CreateDirectBinding(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error creating binding: %v", err)
	}

	if b == nil {
		t.Fatal("expected non-nil binding")
	}

	// Should have 2 warnings for both unregistered endpoints (optimistic binding decision)
	if len(warnings) != 2 {
		t.Errorf("expected 2 optimistic binding warnings, got %d (%v)", len(warnings), warnings)
	}

	bindings := ctrl.GetBindings()
	if len(bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(bindings))
	}

	// Remove binding
	if err := ctrl.RemoveDirectBinding(ctx, req); err != nil {
		t.Fatalf("failed to remove binding: %v", err)
	}

	if len(ctrl.GetBindings()) != 0 {
		t.Errorf("expected 0 bindings after removal, got %d", len(ctrl.GetBindings()))
	}
}

func TestControllerOnDemandRecommendations(t *testing.T) {
	cfg := config.Default()
	cfg.Storage.DevicesPath = filepath.Join(t.TempDir(), "devices.yaml")
	cfg.MQTT.Enabled = true
	cfg.AI.Enabled = true

	ctrl, mockAdp, _ := setupTestControllerWithConfig(t, cfg)
	ctx := t.Context()
	_ = ctrl.Start(ctx)

	// Register a switch (output OnOff) and a bulb (input OnOff)
	mockAdp.EmitDeviceJoin(adapter.DeviceJoinInfo{IEEE: "0x00158D0001111111", NWK: 0x1111})
	mockAdp.EmitDeviceJoin(adapter.DeviceJoinInfo{IEEE: "0x00158D0002222222", NWK: 0x2222})
	time.Sleep(50 * time.Millisecond)

	// Fetch on-demand recommendations
	recs, err := ctrl.GetRecommendations(ctx)
	if err != nil {
		t.Fatalf("failed to get recommendations: %v", err)
	}

	if len(recs) == 0 {
		t.Fatal("expected at least 1 on-demand recommendation")
	}

	recID := recs[0].ID
	if recs[0].Type == ai.TypeDirectBinding {
		// Apply recommendation
		if err := ctrl.ApplyRecommendation(ctx, recID); err != nil {
			t.Fatalf("failed to apply recommendation: %v", err)
		}

		if len(ctrl.GetBindings()) != 1 {
			t.Errorf("expected 1 direct binding created by applied recommendation, got %d", len(ctrl.GetBindings()))
		}
	}
}

func TestControllerAIDisabled(t *testing.T) {
	ctrl, _, _ := setupTestController(t) // Default has AI.Enabled = false
	ctx := t.Context()
	_ = ctrl.Start(ctx)

	if ctrl.Status().AIEnabled {
		t.Error("expected AIEnabled to be false in controller status")
	}

	// Recommendations should return ErrAIDisabled
	recs, err := ctrl.GetRecommendations(ctx)
	if !errors.Is(err, controller.ErrAIDisabled) {
		t.Errorf("expected ErrAIDisabled from GetRecommendations, got: %v", err)
	}
	if recs != nil {
		t.Errorf("expected nil recommendations, got: %v", recs)
	}

	// Applying recommendation should return ErrAIDisabled
	err = ctrl.ApplyRecommendation(ctx, "rec-123")
	if !errors.Is(err, controller.ErrAIDisabled) {
		t.Errorf("expected ErrAIDisabled from ApplyRecommendation, got: %v", err)
	}
}

func TestControllerPersistenceAndRehydration(t *testing.T) {
	devicesFile := filepath.Join(t.TempDir(), "devices.yaml")

	cfg1 := config.Default()
	cfg1.Storage.DevicesPath = devicesFile
	cfg1.MQTT.Enabled = true

	mockTrans1, _ := transport.NewMockTransport()
	mockAdp1 := mock.New(20, 0x1A62)
	mockMQTT1 := mqtt.NewMockClient()

	ctrl1 := controller.New(cfg1, mockTrans1, mockAdp1, mockMQTT1)
	ctx := t.Context()
	if err := ctrl1.Start(ctx); err != nil {
		t.Fatalf("failed to start ctrl1: %v", err)
	}

	// Pair device 1 and device 2
	mockAdp1.EmitDeviceJoin(adapter.DeviceJoinInfo{
		IEEE: "0x00158D0001",
		NWK:  0x1234,
	})
	mockAdp1.EmitDeviceJoin(adapter.DeviceJoinInfo{
		IEEE: "0x00158D0002",
		NWK:  0x5678,
	})
	time.Sleep(50 * time.Millisecond)

	// Set friendly name and metadata
	if !ctrl1.SetDeviceFriendlyName("0x00158D0001", "Living Room Switch") {
		t.Fatal("failed to set friendly name for dev1")
	}
	ctrl1.UpdateDeviceModel("0x00158D0001", "IKEA", "E1743")
	ctrl1.UpdateDeviceMetadata("0x00158D0001", []uint16{1}, nil, []zcl.ClusterID{zcl.ClusterOnOff})

	if !ctrl1.SetDeviceFriendlyName("0x00158D0002", "Ceiling Light") {
		t.Fatal("failed to set friendly name for dev2")
	}
	ctrl1.UpdateDeviceModel("0x00158D0002", "Philips", "LWB010")
	ctrl1.UpdateDeviceMetadata("0x00158D0002", []uint16{1}, []zcl.ClusterID{zcl.ClusterOnOff}, nil)

	// Create direct binding
	bindReq := adapter.BindRequest{
		SrcIEEE:     "0x00158D0001",
		SrcEndpoint: 1,
		ClusterID:   zcl.ClusterOnOff,
		DstIEEE:     "0x00158D0002",
		DstEndpoint: 1,
	}
	if _, _, err := ctrl1.CreateDirectBinding(ctx, bindReq); err != nil {
		t.Fatalf("failed to create direct binding: %v", err)
	}

	// Stop ctrl1, which triggers Flush() to disk
	if err := ctrl1.Stop(); err != nil {
		t.Fatalf("failed to stop ctrl1: %v", err)
	}

	// Now spin up ctrl2 with the same devicesFile
	cfg2 := config.Default()
	cfg2.Storage.DevicesPath = devicesFile
	cfg2.MQTT.Enabled = true

	mockTrans2, _ := transport.NewMockTransport()
	mockAdp2 := mock.New(20, 0x1A62)
	mockMQTT2 := mqtt.NewMockClient()

	ctrl2 := controller.New(cfg2, mockTrans2, mockAdp2, mockMQTT2)
	t.Cleanup(func() {
		_ = ctrl2.Stop()
	})
	if err := ctrl2.Start(ctx); err != nil {
		t.Fatalf("failed to start ctrl2: %v", err)
	}

	// Assert devices rehydrated
	devs := ctrl2.GetDevices()
	if len(devs) != 2 {
		t.Fatalf("expected 2 rehydrated devices, got %d", len(devs))
	}

	dev1, ok1 := ctrl2.GetDevice("0x00158D0001")
	if !ok1 || dev1.FriendlyName != "Living Room Switch" {
		t.Errorf("dev1 not properly rehydrated: %+v", dev1)
	}
	if dev1.Manufacturer != "IKEA" || dev1.Model != "E1743" {
		t.Errorf("dev1 metadata mismatch: %s / %s", dev1.Manufacturer, dev1.Model)
	}
	if len(dev1.OutputClusters) != 1 || dev1.OutputClusters[0] != zcl.ClusterOnOff {
		t.Errorf("dev1 output clusters mismatch: %+v", dev1.OutputClusters)
	}

	dev2, ok2 := ctrl2.GetDevice("0x00158D0002")
	if !ok2 || dev2.FriendlyName != "Ceiling Light" {
		t.Errorf("dev2 not properly rehydrated: %+v", dev2)
	}
	if dev2.Manufacturer != "Philips" || dev2.Model != "LWB010" {
		t.Errorf("dev2 metadata mismatch: %s / %s", dev2.Manufacturer, dev2.Model)
	}
	if len(dev2.InputClusters) != 1 || dev2.InputClusters[0] != zcl.ClusterOnOff {
		t.Errorf("dev2 input clusters mismatch: %+v", dev2.InputClusters)
	}

	// Assert bindings rehydrated
	bindings := ctrl2.GetBindings()
	if len(bindings) != 1 {
		t.Fatalf("expected 1 rehydrated binding, got %d", len(bindings))
	}
	if bindings[0].SrcIEEE != "0x00158D0001" || bindings[0].DstIEEE != "0x00158D0002" {
		t.Errorf("unexpected binding: %+v", bindings[0])
	}

	// Assert mockMQTT2 received Home Assistant discovery for rehydrated devices
	messages := mockMQTT2.GetMessages()
	discoveryCount := 0
	for _, msg := range messages {
		if strings.Contains(msg.Topic, "homeassistant/") {
			discoveryCount++
		}
	}
	if discoveryCount == 0 {
		t.Error("expected Home Assistant discovery publications on controller rehydration")
	}
}

func TestVirtualSNZB01PSimulation(t *testing.T) {
	ctrl, _, mockMQTT := setupTestController(t)
	ctx := t.Context()
	if err := ctrl.Start(ctx); err != nil {
		t.Fatalf("failed to start controller: %v", err)
	}

	// 1. Get SNZB-01P definition from embedded registry
	fixtures := ctrl.Fixtures()
	if fixtures == nil {
		t.Fatal("expected fixtures registry in controller")
	}
	def, ok := fixtures.Get("SNZB-01P")
	if !ok {
		t.Fatal("SNZB-01P fixture definition not found in controller registry")
	}

	// 2. Spawn virtual device
	const ieee = "0x00124b00226b8899"
	const nwk = 0x1234
	vdev, err := ctrl.SpawnVirtualDevice(def, ieee, nwk)
	if err != nil {
		t.Fatalf("failed to spawn virtual device: %v", err)
	}
	if vdev == nil {
		t.Fatal("expected spawned virtual device to be non-nil")
	}

	// Allow event bus and frame handler to process
	time.Sleep(50 * time.Millisecond)

	// Verify device registered in controller
	dev, found := ctrl.GetDevice(ieee)
	if !found {
		t.Fatalf("device %s not found in controller", ieee)
	}
	if dev.Manufacturer != "SONOFF" {
		t.Errorf("expected manufacturer SONOFF, got %s", dev.Manufacturer)
	}
	if dev.Model != "SNZB-01P" {
		t.Errorf("expected model SNZB-01P, got %s", dev.Model)
	}
	if len(dev.Endpoints) == 0 {
		t.Error("expected enriched endpoints from fixture")
	}

	// Verify Home Assistant discovery published
	messages := mockMQTT.GetMessages()
	hasActionDiscovery := false
	hasBatteryDiscovery := false
	hasVoltageDiscovery := false
	for _, msg := range messages {
		if strings.Contains(msg.Topic, "homeassistant/sensor/") && strings.Contains(msg.Topic, "_action/config") {
			hasActionDiscovery = true
		}
		if strings.Contains(msg.Topic, "homeassistant/sensor/") && strings.Contains(msg.Topic, "_battery/config") {
			hasBatteryDiscovery = true
		}
		if strings.Contains(msg.Topic, "homeassistant/sensor/") && strings.Contains(msg.Topic, "_voltage/config") {
			hasVoltageDiscovery = true
		}
	}
	if !hasActionDiscovery {
		t.Error("expected action discovery message in MQTT")
	}
	if !hasBatteryDiscovery {
		t.Error("expected battery discovery message in MQTT")
	}
	if !hasVoltageDiscovery {
		t.Error("expected voltage discovery message in MQTT")
	}

	// 3. Test TriggerAction("single")
	if err := vdev.TriggerAction("single"); err != nil {
		t.Fatalf("failed to trigger single action: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	foundSingle := false
	for _, m := range mockMQTT.GetMessages() {
		if m.Topic == "zigbridge/"+ieee && strings.Contains(string(m.Payload), `"action":"single"`) {
			foundSingle = true
			break
		}
	}
	if !foundSingle {
		t.Error("expected MQTT message with action:single")
	}

	// 4. Test TriggerAction("double")
	if err := vdev.TriggerAction("double"); err != nil {
		t.Fatalf("failed to trigger double action: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	foundDouble := false
	for _, m := range mockMQTT.GetMessages() {
		if m.Topic == "zigbridge/"+ieee && strings.Contains(string(m.Payload), `"action":"double"`) {
			foundDouble = true
			break
		}
	}
	if !foundDouble {
		t.Error("expected MQTT message with action:double")
	}

	// 5. Test TriggerAction("long")
	if err := vdev.TriggerAction("long"); err != nil {
		t.Fatalf("failed to trigger long action: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	foundLong := false
	for _, m := range mockMQTT.GetMessages() {
		if m.Topic == "zigbridge/"+ieee && strings.Contains(string(m.Payload), `"action":"long"`) {
			foundLong = true
			break
		}
	}
	if !foundLong {
		t.Error("expected MQTT message with action:long")
	}

	// 6. Test ReportBattery(95, 3000)
	if err := vdev.ReportBattery(95, 3000); err != nil {
		t.Fatalf("failed to report battery: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	foundBattery := false
	for _, m := range mockMQTT.GetMessages() {
		if m.Topic == "zigbridge/"+ieee && strings.Contains(string(m.Payload), `"battery":95`) && strings.Contains(string(m.Payload), `"voltage":3000`) {
			foundBattery = true
			break
		}
	}
	if !foundBattery {
		t.Error("expected MQTT message with battery:95 and voltage:3000")
	}

	// 7. Verify virtual device retrieval from controller
	vdevs := ctrl.GetVirtualDevices()
	if len(vdevs) != 1 {
		t.Errorf("expected 1 virtual device, got %d", len(vdevs))
	}
	vdevFound, ok := ctrl.GetVirtualDevice(ieee)
	if !ok || vdevFound == nil {
		t.Errorf("expected to find virtual device %s", ieee)
	}
}

func TestVirtualA7ZSimulation(t *testing.T) {
	ctrl, _, mockMQTT := setupTestController(t)
	ctx := t.Context()
	if err := ctrl.Start(ctx); err != nil {
		t.Fatalf("failed to start controller: %v", err)
	}

	fixtures := ctrl.Fixtures()
	if fixtures == nil {
		t.Fatal("expected fixtures registry in controller")
	}
	def, ok := fixtures.Get("A7Z")
	if !ok {
		t.Fatal("A7Z fixture definition not found in controller registry")
	}

	const ieee = "0x00124b0099887766"
	const nwk = 0x5678
	vdev, err := ctrl.SpawnVirtualDevice(def, ieee, nwk)
	if err != nil {
		t.Fatalf("failed to spawn virtual device: %v", err)
	}
	if vdev == nil {
		t.Fatal("expected spawned virtual device to be non-nil")
	}

	time.Sleep(50 * time.Millisecond)

	dev, found := ctrl.GetDevice(ieee)
	if !found {
		t.Fatalf("device %s not found in controller", ieee)
	}
	if dev.Manufacturer != "Nous" {
		t.Errorf("expected manufacturer Nous, got %s", dev.Manufacturer)
	}
	if dev.Model != "A7Z" {
		t.Errorf("expected model A7Z, got %s", dev.Model)
	}

	// Verify Home Assistant discovery published
	messages := mockMQTT.GetMessages()
	hasSwitchDiscovery := false
	hasPowerDiscovery := false
	hasCurrentDiscovery := false
	hasVoltageDiscovery := false
	hasEnergyDiscovery := false
	for _, msg := range messages {
		if strings.Contains(msg.Topic, "homeassistant/switch/") {
			hasSwitchDiscovery = true
		}
		if strings.Contains(msg.Topic, "homeassistant/sensor/") && strings.Contains(msg.Topic, "_power/config") {
			hasPowerDiscovery = true
		}
		if strings.Contains(msg.Topic, "homeassistant/sensor/") && strings.Contains(msg.Topic, "_current/config") {
			hasCurrentDiscovery = true
		}
		if strings.Contains(msg.Topic, "homeassistant/sensor/") && strings.Contains(msg.Topic, "_voltage/config") {
			hasVoltageDiscovery = true
		}
		if strings.Contains(msg.Topic, "homeassistant/sensor/") && strings.Contains(msg.Topic, "_energy/config") {
			hasEnergyDiscovery = true
		}
	}
	if !hasSwitchDiscovery {
		t.Error("expected switch discovery message in MQTT")
	}
	if !hasPowerDiscovery {
		t.Error("expected power discovery message in MQTT")
	}
	if !hasCurrentDiscovery {
		t.Error("expected current discovery message in MQTT")
	}
	if !hasVoltageDiscovery {
		t.Error("expected voltage discovery message in MQTT")
	}
	if !hasEnergyDiscovery {
		t.Error("expected energy discovery message in MQTT")
	}

	// Test ReportElectrical(1500, 230, 6, 12)
	if err := vdev.ReportElectrical(1500, 230, 6, 12); err != nil {
		t.Fatalf("failed to report electrical telemetry: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	foundElectrical := false
	for _, m := range mockMQTT.GetMessages() {
		if m.Topic == "zigbridge/"+ieee &&
			strings.Contains(string(m.Payload), `"power":1500`) &&
			strings.Contains(string(m.Payload), `"voltage":230`) &&
			strings.Contains(string(m.Payload), `"current":6`) {
			foundElectrical = true
			break
		}
	}
	if !foundElectrical {
		t.Error("expected MQTT message with power, voltage, and current")
	}

	foundEnergy := false
	for _, m := range mockMQTT.GetMessages() {
		if m.Topic == "zigbridge/"+ieee && strings.Contains(string(m.Payload), `"energy":12`) {
			foundEnergy = true
			break
		}
	}
	if !foundEnergy {
		t.Error("expected MQTT message with energy:12")
	}
}

func TestDeviceRegistryDualLookupAndInterview(t *testing.T) {
	reg := controller.NewDeviceRegistry()
	dev := &controller.Device{
		IEEE:         "0x00124B0001020304",
		NWK:          0x4321,
		FriendlyName: "Living Room Plug",
		Endpoints:    []uint16{1},
		State:        make(map[string]any),
	}
	reg.Upsert(dev)

	// Test lookup by IEEE
	d, ok := reg.Get("0x00124B0001020304")
	if !ok || d.IEEE != dev.IEEE {
		t.Fatalf("failed to get device by IEEE")
	}

	// Test lookup by friendly name
	d, ok = reg.Get("Living Room Plug")
	if !ok || d.IEEE != dev.IEEE {
		t.Fatalf("failed to get device by FriendlyName")
	}

	// Test lookup by NWK hex
	d, ok = reg.Get("0x4321")
	if !ok || d.IEEE != dev.IEEE {
		t.Fatalf("failed to get device by NWK hex 0x4321")
	}

	d, ok = reg.Get("4321")
	if !ok || d.IEEE != dev.IEEE {
		t.Fatalf("failed to get device by NWK hex 4321")
	}

	// Test controller handling of CmdReadAttributesResponse
	cfg := config.Default()
	cfg.Storage.DevicesPath = "" // in-memory
	mockTrans, _ := transport.NewMockTransport()
	mockAdp := mock.New(20, 0x1A62)
	mockMQTT := mqtt.NewMockClient()

	ctrl := controller.New(cfg, mockTrans, mockAdp, mockMQTT)
	t.Cleanup(func() {
		_ = ctrl.Stop()
	})
	ctx := t.Context()
	_ = ctrl.Start(ctx)

	ctrl.HandleDeviceJoin(adapter.DeviceJoinInfo{
		IEEE: "0x00124B000A7Z0001",
		NWK:  0xA701,
	})

	// Send CmdReadAttributesResponse for ClusterBasic
	// Attr 0x0004 = "Nous", Attr 0x0005 = "A7Z"
	readPayload := []byte{
		0x04, 0x00, 0x00, 0x42, 0x04, 'N', 'o', 'u', 's',
		0x05, 0x00, 0x00, 0x42, 0x03, 'A', '7', 'Z',
	}

	ctrl.HandleIncomingFrame(&zcl.Frame{
		Header: zcl.FrameControl{
			Type:                   zcl.FrameTypeGlobal,
			ManufacturerSpecific:   false,
			Direction:              zcl.DirectionServerToClient,
			DisableDefaultResponse: true,
		},
		ClusterID:      zcl.ClusterBasic,
		CommandID:      zcl.CmdReadAttributesResponse,
		SourceAddress:  "0xA701",
		DestAddress:    "0x0000",
		SourceEndpoint: 1,
		DestEndpoint:   1,
		Payload:        readPayload,
		LQI:            255,
	})

	joinedDev, ok := ctrl.GetDevice("0x00124B000A7Z0001")
	if !ok {
		t.Fatalf("expected joined device to be found")
	}
	if joinedDev.Manufacturer != "Nous" {
		t.Errorf("expected manufacturer Nous, got %s", joinedDev.Manufacturer)
	}
	if joinedDev.Model != "A7Z" {
		t.Errorf("expected model A7Z, got %s", joinedDev.Model)
	}
}

func TestVirtualIASZoneDevices(t *testing.T) {
	ctrl, _, mockMQTT := setupTestController(t)
	ctx := t.Context()
	if err := ctrl.Start(ctx); err != nil {
		t.Fatalf("failed to start controller: %v", err)
	}

	fixtures := ctrl.Fixtures()
	if fixtures == nil {
		t.Fatal("expected fixtures registry in controller")
	}

	// 1. Test IKEA BADRING E2202 (Water Leak Sensor)
	e2202Def, ok := fixtures.Get("E2202")
	if !ok {
		t.Fatal("E2202 fixture definition not found")
	}

	const leakIEEE = "0x00124B0000E2202A"
	vdevLeak, err := ctrl.SpawnVirtualDevice(e2202Def, leakIEEE, 0x2202)
	if err != nil {
		t.Fatalf("failed to spawn E2202 virtual device: %v", err)
	}
	time.Sleep(30 * time.Millisecond)

	// Check discovery has moisture sensor
	hasMoistureDiscovery := false
	for _, m := range mockMQTT.GetMessages() {
		if strings.Contains(m.Topic, "homeassistant/binary_sensor/") && strings.Contains(m.Topic, "_water_leak/config") {
			hasMoistureDiscovery = true
			break
		}
	}
	if !hasMoistureDiscovery {
		t.Error("expected moisture binary_sensor discovery for E2202")
	}

	// Action "leak"
	if err := vdevLeak.TriggerAction("leak"); err != nil {
		t.Fatalf("failed to trigger leak: %v", err)
	}
	time.Sleep(30 * time.Millisecond)

	dev, ok := ctrl.GetDevice(leakIEEE)
	if !ok || dev.State["water_leak"] != true {
		t.Errorf("expected water_leak: true on controller dev, got: %v", dev.State)
	}

	// Action "no_leak"
	if err := vdevLeak.TriggerAction("no_leak"); err != nil {
		t.Fatalf("failed to trigger no_leak: %v", err)
	}
	time.Sleep(30 * time.Millisecond)

	dev, _ = ctrl.GetDevice(leakIEEE)
	if dev.State["water_leak"] != false {
		t.Errorf("expected water_leak: false on controller dev, got: %v", dev.State)
	}

	// Direct ReportIASZone(0x0001) -> leak true
	if err := vdevLeak.ReportIASZone(0x0001); err != nil {
		t.Fatalf("failed to report IAS Zone: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	dev, _ = ctrl.GetDevice(leakIEEE)
	if dev.State["water_leak"] != true {
		t.Errorf("expected water_leak: true via ReportIASZone, got: %v", dev.State)
	}

	// 2. Test Aqara MCCGQ11LM (Door/Window Contact Sensor)
	mccgqDef, ok := fixtures.Get("MCCGQ11LM")
	if !ok {
		t.Fatal("MCCGQ11LM fixture definition not found")
	}

	const contactIEEE = "0x00158D000MCCGQ01"
	vdevContact, err := ctrl.SpawnVirtualDevice(mccgqDef, contactIEEE, 0x1111)
	if err != nil {
		t.Fatalf("failed to spawn MCCGQ11LM virtual device: %v", err)
	}
	time.Sleep(30 * time.Millisecond)

	// Check discovery has door/contact sensor
	hasContactDiscovery := false
	for _, m := range mockMQTT.GetMessages() {
		if strings.Contains(m.Topic, "homeassistant/binary_sensor/") && strings.Contains(m.Topic, "_contact/config") {
			hasContactDiscovery = true
			break
		}
	}
	if !hasContactDiscovery {
		t.Error("expected contact binary_sensor discovery for MCCGQ11LM")
	}

	// Action "open" -> contact: false (alarm1=1)
	if err := vdevContact.TriggerAction("open"); err != nil {
		t.Fatalf("failed to trigger open action: %v", err)
	}
	time.Sleep(30 * time.Millisecond)

	dev, _ = ctrl.GetDevice(contactIEEE)
	if dev.State["contact"] != false {
		t.Errorf("expected contact: false for open door, got: %v", dev.State)
	}

	// Action "closed" -> contact: true (alarm1=0)
	if err := vdevContact.TriggerAction("closed"); err != nil {
		t.Fatalf("failed to trigger closed action: %v", err)
	}
	time.Sleep(30 * time.Millisecond)

	dev, _ = ctrl.GetDevice(contactIEEE)
	if dev.State["contact"] != true {
		t.Errorf("expected contact: true for closed door, got: %v", dev.State)
	}
}

func TestVirtualIASACEDevices(t *testing.T) {
	ctrl, _, mockMQTT := setupTestController(t)
	ctx := t.Context()
	if err := ctrl.Start(ctx); err != nil {
		t.Fatalf("failed to start controller: %v", err)
	}

	fixtures := ctrl.Fixtures()
	keyzbDef, ok := fixtures.Get("KEYZB-110")
	if !ok {
		t.Fatal("KEYZB-110 fixture definition not found")
	}

	const keypadIEEE = "0x0015BC000KEYZB01"
	vdev, err := ctrl.SpawnVirtualDevice(keyzbDef, keypadIEEE, 0x1101)
	if err != nil {
		t.Fatalf("failed to spawn KEYZB-110 virtual device: %v", err)
	}
	time.Sleep(30 * time.Millisecond)

	actions := []string{"disarm", "arm_all_zones", "emergency"}
	for _, act := range actions {
		if err := vdev.TriggerAction(act); err != nil {
			t.Fatalf("failed to trigger %s: %v", act, err)
		}
		time.Sleep(30 * time.Millisecond)

		dev, ok := ctrl.GetDevice(keypadIEEE)
		if !ok || dev.State["action"] != act {
			t.Errorf("expected action %q in device state, got: %v", act, dev.State)
		}

		foundMQTT := false
		for _, m := range mockMQTT.GetMessages() {
			if m.Topic == "zigbridge/"+keypadIEEE && strings.Contains(string(m.Payload), fmt.Sprintf(`"action":%q`, act)) {
				foundMQTT = true
				break
			}
		}
		if !foundMQTT {
			t.Errorf("expected MQTT publication with action %q", act)
		}
	}
}

func TestVirtualPayloadDisambiguation(t *testing.T) {
	ctrl, _, _ := setupTestController(t)
	ctx := t.Context()
	if err := ctrl.Start(ctx); err != nil {
		t.Fatalf("failed to start controller: %v", err)
	}

	fixtures := ctrl.Fixtures()

	// 1. SOMRIG E2213
	e2213Def, ok := fixtures.Get("E2213")
	if !ok {
		t.Fatal("E2213 fixture definition not found")
	}

	const somrigIEEE = "0x00124B0000E2213A"
	somrigDev, err := ctrl.SpawnVirtualDevice(e2213Def, somrigIEEE, 0x2213)
	if err != nil {
		t.Fatalf("failed to spawn E2213 virtual device: %v", err)
	}
	time.Sleep(30 * time.Millisecond)

	if err := somrigDev.TriggerAction("1_initial_press"); err != nil {
		t.Fatalf("failed to trigger 1_initial_press: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	dev, _ := ctrl.GetDevice(somrigIEEE)
	if dev.State["action"] != "1_initial_press" {
		t.Errorf("expected action '1_initial_press', got: %v", dev.State["action"])
	}

	if err := somrigDev.TriggerAction("2_initial_press"); err != nil {
		t.Fatalf("failed to trigger 2_initial_press: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	dev, _ = ctrl.GetDevice(somrigIEEE)
	if dev.State["action"] != "2_initial_press" {
		t.Errorf("expected action '2_initial_press', got: %v", dev.State["action"])
	}

	// 2. STYRBAR E2001/E2002/E2313
	styrbarDef, ok := fixtures.Get("E2001/E2002/E2313")
	if !ok {
		t.Fatal("STYRBAR fixture definition not found")
	}

	const styrbarIEEE = "0x00124B0000STYR01"
	styrbarDev, err := ctrl.SpawnVirtualDevice(styrbarDef, styrbarIEEE, 0x2001)
	if err != nil {
		t.Fatalf("failed to spawn STYRBAR virtual device: %v", err)
	}
	time.Sleep(30 * time.Millisecond)

	styrbarActions := []string{"arrow_left_click", "arrow_right_click", "brightness_move_up", "brightness_move_down"}
	for _, act := range styrbarActions {
		if err := styrbarDev.TriggerAction(act); err != nil {
			t.Fatalf("failed to trigger STYRBAR %s: %v", act, err)
		}
		time.Sleep(30 * time.Millisecond)
		dev, _ = ctrl.GetDevice(styrbarIEEE)
		if dev.State["action"] != act {
			t.Errorf("expected STYRBAR action %q, got: %v", act, dev.State["action"])
		}
	}
}

func TestDualPhaseEnergyMeter(t *testing.T) {
	ctrl, _, mockMQTT := setupTestController(t)
	ctx := t.Context()
	if err := ctrl.Start(ctx); err != nil {
		t.Fatalf("failed to start controller: %v", err)
	}

	fixtures := ctrl.Fixtures()
	meterDef, ok := fixtures.Get("PJ-1203A")
	if !ok {
		t.Fatal("PJ-1203A fixture definition not found")
	}

	const meterIEEE = "0x00124B0000PJ1203"
	_, err := ctrl.SpawnVirtualDevice(meterDef, meterIEEE, 0x1203)
	if err != nil {
		t.Fatalf("failed to spawn PJ-1203A: %v", err)
	}
	time.Sleep(30 * time.Millisecond)

	// Check multi-phase sensor discovery in MQTT
	hasPowerA := false
	hasPowerB := false
	for _, m := range mockMQTT.GetMessages() {
		if strings.Contains(m.Topic, "homeassistant/sensor/") {
			if strings.Contains(m.Topic, "_power_a/config") {
				hasPowerA = true
			}
			if strings.Contains(m.Topic, "_power_b/config") {
				hasPowerB = true
			}
		}
	}
	if !hasPowerA || !hasPowerB {
		t.Errorf("expected power_a and power_b discovery, got powerA=%v powerB=%v", hasPowerA, hasPowerB)
	}

	// 1. Report phase A telemetry on Endpoint 1 (ActivePower 0x050B = 350W)
	payloadEp1 := make([]byte, 5)
	binary.LittleEndian.PutUint16(payloadEp1[0:2], 0x050B)
	payloadEp1[2] = zcl.TypeInt16
	binary.LittleEndian.PutUint16(payloadEp1[3:5], 350)

	ctrl.HandleIncomingFrame(&zcl.Frame{
		Header: zcl.FrameControl{
			Type:      zcl.FrameTypeGlobal,
			Direction: zcl.DirectionServerToClient,
		},
		ClusterID:      zcl.ClusterElectricalMeasurement,
		CommandID:      zcl.CmdReportAttributes,
		SourceAddress:  meterIEEE,
		SourceEndpoint: 1,
		DestEndpoint:   1,
		Payload:        payloadEp1,
	})

	// 2. Report phase B telemetry on Endpoint 2 (ActivePower 0x050B = 720W)
	payloadEp2 := make([]byte, 5)
	binary.LittleEndian.PutUint16(payloadEp2[0:2], 0x050B)
	payloadEp2[2] = zcl.TypeInt16
	binary.LittleEndian.PutUint16(payloadEp2[3:5], 720)

	ctrl.HandleIncomingFrame(&zcl.Frame{
		Header: zcl.FrameControl{
			Type:      zcl.FrameTypeGlobal,
			Direction: zcl.DirectionServerToClient,
		},
		ClusterID:      zcl.ClusterElectricalMeasurement,
		CommandID:      zcl.CmdReportAttributes,
		SourceAddress:  meterIEEE,
		SourceEndpoint: 2,
		DestEndpoint:   1,
		Payload:        payloadEp2,
	})

	time.Sleep(30 * time.Millisecond)

	dev, ok := ctrl.GetDevice(meterIEEE)
	if !ok {
		t.Fatalf("failed to retrieve meter device")
	}
	if dev.State["power_a"] != float64(350) {
		t.Errorf("expected power_a=350, got: %v", dev.State["power_a"])
	}
	if dev.State["power_b"] != float64(720) {
		t.Errorf("expected power_b=720, got: %v", dev.State["power_b"])
	}
}

func TestControllerSetDeviceStateAndAction(t *testing.T) {
	ctrl, _, mockMQTT := setupTestController(t)
	ctx := t.Context()
	if err := ctrl.Start(ctx); err != nil {
		t.Fatalf("failed to start controller: %v", err)
	}

	const ieee = "0x00124B0011223344"
	ctrl.HandleDeviceJoin(adapter.DeviceJoinInfo{
		IEEE: ieee,
		NWK:  0x1122,
	})

	// 1. Set Device State
	updates := map[string]any{
		"state":      "ON",
		"brightness": 200,
	}
	dev, err := ctrl.SetDeviceState(ctx, ieee, updates)
	if err != nil {
		t.Fatalf("failed to set device state: %v", err)
	}
	if dev.State["state"] != "ON" || dev.State["brightness"] != 200 {
		t.Errorf("expected state ON and brightness 200, got: %v", dev.State)
	}

	// Verify MQTT published state
	time.Sleep(30 * time.Millisecond)
	published := mockMQTT.GetMessages()
	foundMQTT := false
	for _, m := range published {
		if strings.Contains(m.Topic, ieee) && strings.Contains(string(m.Payload), "ON") {
			foundMQTT = true
			break
		}
	}
	if !foundMQTT {
		t.Error("expected MQTT publication with device state update")
	}

	// 2. Trigger Action
	if err := ctrl.TriggerDeviceAction(ctx, ieee, "identify"); err != nil {
		t.Fatalf("failed to trigger action: %v", err)
	}

	// 3. Error cases
	if _, err := ctrl.SetDeviceState(ctx, "0xNONEXISTENT", updates); err == nil {
		t.Error("expected error for non-existent device in SetDeviceState")
	}
	if err := ctrl.TriggerDeviceAction(ctx, "0xNONEXISTENT", "identify"); err == nil {
		t.Error("expected error for non-existent device in TriggerDeviceAction")
	}
}

