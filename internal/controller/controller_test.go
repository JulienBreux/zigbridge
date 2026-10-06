package controller_test

import (
	"context"
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

func setupTestController(t *testing.T) (*controller.Controller, *mock.MockAdapter, *mqtt.MockClient) {
	cfg := config.Default()
	cfg.Storage.DevicesPath = filepath.Join(t.TempDir(), "devices.yaml")
	cfg.MQTT.Enabled = true

	mockTrans, _ := transport.NewMockTransport()
	mockAdp := mock.New(20, 0x1A62)
	mockMQTT := mqtt.NewMockClient()

	ctrl := controller.New(cfg, mockTrans, mockAdp, mockMQTT)
	return ctrl, mockAdp, mockMQTT
}

func TestControllerLifecycleAndStatus(t *testing.T) {
	ctrl, _, _ := setupTestController(t)
	ctx := context.Background()

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

	if err := ctrl.Stop(); err != nil {
		t.Fatalf("failed to stop controller: %v", err)
	}
}

func TestControllerPermitJoin(t *testing.T) {
	ctrl, _, _ := setupTestController(t)
	ctx := context.Background()
	_ = ctrl.Start(ctx)
	defer func() { _ = ctrl.Stop() }()

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
	ctx := context.Background()
	_ = ctrl.Start(ctx)
	defer func() { _ = ctrl.Stop() }()

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

	// Verify telemetry recorded in AI EventCollector
	if ctrl.EventCollector().Count() < 1 {
		t.Errorf("expected at least 1 telemetry event in collector, got %d", ctrl.EventCollector().Count())
	}
}

func TestControllerOptimisticDirectBinding(t *testing.T) {
	ctrl, _, _ := setupTestController(t)
	ctx := context.Background()
	_ = ctrl.Start(ctx)
	defer func() { _ = ctrl.Stop() }()

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
	ctrl, mockAdp, _ := setupTestController(t)
	ctx := context.Background()
	_ = ctrl.Start(ctx)
	defer func() { _ = ctrl.Stop() }()

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

func TestControllerPersistenceAndRehydration(t *testing.T) {
	devicesFile := filepath.Join(t.TempDir(), "devices.yaml")

	cfg1 := config.Default()
	cfg1.Storage.DevicesPath = devicesFile
	cfg1.MQTT.Enabled = true

	mockTrans1, _ := transport.NewMockTransport()
	mockAdp1 := mock.New(20, 0x1A62)
	mockMQTT1 := mqtt.NewMockClient()

	ctrl1 := controller.New(cfg1, mockTrans1, mockAdp1, mockMQTT1)
	ctx := context.Background()
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
	if err := ctrl2.Start(ctx); err != nil {
		t.Fatalf("failed to start ctrl2: %v", err)
	}
	defer func() { _ = ctrl2.Stop() }()

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
	ctx := context.Background()
	if err := ctrl.Start(ctx); err != nil {
		t.Fatalf("failed to start controller: %v", err)
	}
	defer func() { _ = ctrl.Stop() }()

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
	ctx := context.Background()
	if err := ctrl.Start(ctx); err != nil {
		t.Fatalf("failed to start controller: %v", err)
	}
	defer func() { _ = ctrl.Stop() }()

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


