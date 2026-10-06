package mqtt_test

import (
	"context"
	"strings"
	"testing"

	"github.com/julienbreux/zigbridge/internal/mqtt"
)

func TestMockClientPublishSubscribe(t *testing.T) {
	client := mqtt.NewMockClient()
	ctx := context.Background()

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
}
