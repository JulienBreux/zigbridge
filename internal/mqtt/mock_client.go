package mqtt

import (
	"context"
	"encoding/json"
	"sync"
)

// PublishedMessage records a message dispatched via MockClient.
type PublishedMessage struct {
	Topic    string
	QoS      byte
	Retained bool
	Payload  []byte
}

// MockClient provides an in-memory MQTT client for tests.
type MockClient struct {
	mu          sync.RWMutex
	connected   bool
	messages    []PublishedMessage
	subscribers map[string][]MessageHandler
}

// NewMockClient creates a new MockClient instance.
func NewMockClient() *MockClient {
	return &MockClient{
		messages:    make([]PublishedMessage, 0),
		subscribers: make(map[string][]MessageHandler),
	}
}

func (m *MockClient) Connect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = true
	return nil
}

func (m *MockClient) Disconnect(quiesceMs uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
}

func (m *MockClient) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

func (m *MockClient) Publish(topic string, qos byte, retained bool, payload []byte) error {
	m.mu.Lock()
	m.messages = append(m.messages, PublishedMessage{
		Topic:    topic,
		QoS:      qos,
		Retained: retained,
		Payload:  payload,
	})
	subs := m.subscribers[topic]
	m.mu.Unlock()

	for _, sub := range subs {
		sub(topic, payload)
	}
	return nil
}

func (m *MockClient) PublishJSON(topic string, qos byte, retained bool, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return m.Publish(topic, qos, retained, data)
}

func (m *MockClient) Subscribe(topic string, qos byte, handler MessageHandler) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscribers[topic] = append(m.subscribers[topic], handler)
	return nil
}

func (m *MockClient) PublishDeviceState(ieee string, state map[string]interface{}) error {
	return m.PublishJSON("zigbridge/"+ieee, 0, false, state)
}

func (m *MockClient) PublishDiscovery(entity HAEntityConfig) error {
	topic := entity.Topic("homeassistant", "zigbridge", entity.Device.Identifiers[0])
	return m.PublishJSON(topic, 0, true, entity)
}

func (m *MockClient) PublishBridgeState(online bool) error {
	status := "offline"
	if online {
		status = "online"
	}
	return m.Publish("zigbridge/bridge/state", 0, true, []byte(status))
}

// GetMessages returns a snapshot of all published messages.
func (m *MockClient) GetMessages() []PublishedMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := make([]PublishedMessage, len(m.messages))
	copy(cp, m.messages)
	return cp
}
