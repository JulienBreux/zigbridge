package mqtt_test

import (
	"strings"
	"testing"

	"github.com/julienbreux/zigbridge/internal/mqtt"
)

func TestMockClientPublishSubscribe(t *testing.T) {
	client := mqtt.NewMockClient()
	ctx := t.Context()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect mock client: %v", err)
	}

	received := false
	err := client.Subscribe("test/topic", 0, func(topic string, payload []byte) {
		if string(payload) == "hello_mqtt" {
			received = true
		}
	})
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	err = client.Publish("test/topic", 0, false, []byte("hello_mqtt"))
	if err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	if !received {
		t.Error("expected message handler to receive published payload")
	}

	msgs := client.GetMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message in history, got %d", len(msgs))
	}
}

func TestHADiscoveryGeneration(t *testing.T) {
	dev := mqtt.HADevice{
		Identifiers:  []string{"0x00158D0001"},
		Name:         "Living Room Motion",
		Manufacturer: "Xiaomi",
		Model:        "RTCGQ11LM",
	}

	config := mqtt.NewOccupancyDiscovery(dev, "0x00158D0001", "zigbridge")
	topic := config.Topic("homeassistant", "zigbridge", "0x00158D0001")

	expectedTopic := "homeassistant/binary_sensor/0x00158D0001/0x00158D0001_occupancy/config"
	if topic != expectedTopic {
		t.Errorf("expected discovery topic %s, got %s", expectedTopic, topic)
	}

	if config.DeviceClass != "motion" {
		t.Errorf("expected device class motion, got %s", config.DeviceClass)
	}

	if !strings.Contains(config.StateTopic, "0x00158D0001") {
		t.Errorf("expected state topic to contain IEEE, got %s", config.StateTopic)
	}

	// Test Battery Discovery
	batConfig := mqtt.NewBatteryDiscovery(dev, "0x00158D0001", "zigbridge")
	if batConfig.DeviceClass != "battery" || batConfig.UnitOfMeasurement != "%" {
		t.Errorf("unexpected battery config: %+v", batConfig)
	}

	// Test Voltage Discovery
	voltConfig := mqtt.NewVoltageDiscovery(dev, "0x00158D0001", "zigbridge")
	if voltConfig.DeviceClass != "voltage" || voltConfig.UnitOfMeasurement != "mV" {
		t.Errorf("unexpected voltage config: %+v", voltConfig)
	}

	// Test Action Discovery
	actConfig := mqtt.NewActionDiscovery(dev, "0x00158D0001", "zigbridge")
	if actConfig.ValueTemplate != "{{ value_json.action }}" {
		t.Errorf("unexpected action config: %+v", actConfig)
	}

	// Test Device Trigger Discovery
	trigConfig := mqtt.NewDeviceTriggerDiscovery(dev, "0x00158D0001", "zigbridge", "single")
	trigTopic := trigConfig.Topic("homeassistant", "zigbridge", "0x00158D0001")
	expectedTrigTopic := "homeassistant/device_automation/0x00158D0001/action_single/config"
	if trigTopic != expectedTrigTopic {
		t.Errorf("expected trigger topic %s, got %s", expectedTrigTopic, trigTopic)
	}

	// Test Power Discovery
	powConfig := mqtt.NewPowerDiscovery(dev, "0x00158D0001", "zigbridge")
	if powConfig.DeviceClass != "power" || powConfig.UnitOfMeasurement != "W" {
		t.Errorf("unexpected power config: %+v", powConfig)
	}

	// Test Energy Discovery
	energyConfig := mqtt.NewEnergyDiscovery(dev, "0x00158D0001", "zigbridge")
	if energyConfig.DeviceClass != "energy" || energyConfig.UnitOfMeasurement != "kWh" {
		t.Errorf("unexpected energy config: %+v", energyConfig)
	}

	// Test Current Discovery
	currConfig := mqtt.NewCurrentDiscovery(dev, "0x00158D0001", "zigbridge")
	if currConfig.DeviceClass != "current" || currConfig.UnitOfMeasurement != "A" {
		t.Errorf("unexpected current config: %+v", currConfig)
	}

	// Test Mains Voltage Discovery
	mainsVoltConfig := mqtt.NewMainsVoltageDiscovery(dev, "0x00158D0001", "zigbridge")
	if mainsVoltConfig.DeviceClass != "voltage" || mainsVoltConfig.UnitOfMeasurement != "V" {
		t.Errorf("unexpected mains voltage config: %+v", mainsVoltConfig)
	}
}
