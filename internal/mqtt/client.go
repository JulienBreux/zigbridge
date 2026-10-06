package mqtt

import (
	"context"
	"errors"
)

var (
	ErrNotConnected = errors.New("mqtt: client not connected")
)

// MessageHandler handles incoming messages on subscribed topics.
type MessageHandler func(topic string, payload []byte)

// Client defines the contract for Zigbridge event dispatching and MQTT interaction.
type Client interface {
	// Connect establishes the MQTT connection and registers LWT.
	Connect(ctx context.Context) error

	// Disconnect cleanly disconnects from the broker.
	Disconnect(quiesceMs uint)

	// IsConnected returns whether broker connection is active.
	IsConnected() bool

	// Publish sends a raw byte payload to the specified topic.
	Publish(topic string, qos byte, retained bool, payload []byte) error

	// PublishJSON marshals and sends a JSON payload.
	PublishJSON(topic string, qos byte, retained bool, v interface{}) error

	// Subscribe listens for incoming messages on a topic pattern.
	Subscribe(topic string, qos byte, handler MessageHandler) error

	// PublishDeviceState broadcasts state changes for a device.
	PublishDeviceState(ieee string, state map[string]interface{}) error

	// PublishDiscovery publishes Home Assistant auto-discovery configs.
	PublishDiscovery(entity HAEntityConfig) error

	// PublishBridgeState broadcasts the coordinator online/offline state.
	PublishBridgeState(online bool) error
}
