package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/julienbreux/zigbridge/internal/config"
)

// PahoClient wraps the Eclipse Paho MQTT client for Zigbridge.
type PahoClient struct {
	cfg    config.MQTTConfig
	client paho.Client
	mu     sync.RWMutex
}

// NewPahoClient creates a new PahoClient configured with the specified settings.
func NewPahoClient(cfg config.MQTTConfig) *PahoClient {
	return &PahoClient{
		cfg: cfg,
	}
}

// Connect initializes the connection options, sets LWT, and connects to the broker.
func (c *PahoClient) Connect(ctx context.Context) error {
	opts := paho.NewClientOptions().
		AddBroker(c.cfg.Broker).
		SetClientID(c.cfg.ClientID).
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetMaxReconnectInterval(10 * time.Second).
		SetKeepAlive(30 * time.Second)

	if c.cfg.Username != "" {
		opts.SetUsername(c.cfg.Username)
		opts.SetPassword(c.cfg.Password)
	}

	// Set Last Will and Testament (LWT) for bridge availability
	lwtTopic := c.cfg.BaseTopic + "/bridge/state"
	opts.SetWill(lwtTopic, "offline", c.cfg.QoS, true)

	client := paho.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(c.cfg.ConnectionTimeout) {
		return fmt.Errorf("mqtt connect timed out after %v", c.cfg.ConnectionTimeout)
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("failed to connect to mqtt broker %s: %w", c.cfg.Broker, err)
	}

	c.mu.Lock()
	c.client = client
	c.mu.Unlock()

	// Broadcast online state
	return c.PublishBridgeState(true)
}

func (c *PahoClient) Disconnect(quiesceMs uint) {
	c.mu.Lock()
	client := c.client
	c.client = nil
	c.mu.Unlock()

	if client != nil && client.IsConnected() {
		_ = c.PublishBridgeState(false)
		client.Disconnect(quiesceMs)
	}
}

func (c *PahoClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client != nil && c.client.IsConnected()
}

func (c *PahoClient) Publish(topic string, qos byte, retained bool, payload []byte) error {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return ErrNotConnected
	}

	token := client.Publish(topic, qos, retained, payload)
	if !token.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("mqtt publish timeout on topic %s", topic)
	}
	return token.Error()
}

func (c *PahoClient) PublishJSON(topic string, qos byte, retained bool, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal json payload: %w", err)
	}
	return c.Publish(topic, qos, retained, data)
}

func (c *PahoClient) Subscribe(topic string, qos byte, handler MessageHandler) error {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return ErrNotConnected
	}

	token := client.Subscribe(topic, qos, func(_ paho.Client, msg paho.Message) {
		handler(msg.Topic(), msg.Payload())
	})
	if !token.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("mqtt subscribe timeout on topic %s", topic)
	}
	return token.Error()
}

func (c *PahoClient) PublishDeviceState(ieee string, state map[string]any) error {
	topic := fmt.Sprintf("%s/%s", c.cfg.BaseTopic, ieee)
	return c.PublishJSON(topic, c.cfg.QoS, c.cfg.Retain, state)
}

func (c *PahoClient) PublishDiscovery(entity HAEntityConfig) error {
	if !c.cfg.HADiscovery {
		return nil
	}
	topic := entity.Topic(c.cfg.HADiscoveryPrefix, c.cfg.BaseTopic, entity.Device.Identifiers[0])
	return c.PublishJSON(topic, 0, true, entity)
}

func (c *PahoClient) PublishBridgeState(online bool) error {
	topic := c.cfg.BaseTopic + "/bridge/state"
	payload := "offline"
	if online {
		payload = "online"
	}
	return c.Publish(topic, c.cfg.QoS, true, []byte(payload))
}
