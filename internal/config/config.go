package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// TransportType defines the connection medium to the Zigbee coordinator.
type TransportType string

const (
	TransportTypeTCP    TransportType = "tcp"
	TransportTypeSerial TransportType = "serial"
	TransportTypeMock   TransportType = "mock"
)

// AdapterType defines the coordinator radio protocol implementation.
type AdapterType string

const (
	AdapterTypeZStack AdapterType = "zstack" // TI CC2652/CC1352/CC2530/CC2538 (e.g. SLZB-06, Sonoff-P)
	AdapterTypeEmber  AdapterType = "ember"  // Silicon Labs EmberZNet/EZSP (e.g. SLZB-06M, Sonoff-E, SkyConnect)
	AdapterTypeMock   AdapterType = "mock"   // Mock adapter for testing/development
)

// Config represents the top-level Zigbridge configuration.
type Config struct {
	LogLevel  string          `yaml:"log_level"`
	Transport TransportConfig `yaml:"transport"`
	Adapter   AdapterConfig   `yaml:"adapter"`
	Advanced  *AdapterConfig  `yaml:"advanced,omitempty"` // For compatibility with Zigbee2MQTT configuration.yaml
	Network   NetworkConfig   `yaml:"network"`
	MQTT      MQTTConfig      `yaml:"mqtt"`
	Web       WebConfig       `yaml:"web"`
	AI        AIConfig        `yaml:"ai"`
	Storage   StorageConfig   `yaml:"storage"`
}

// TransportConfig defines transport layer options for serial or networked coordinators.
type TransportConfig struct {
	Type                 TransportType `yaml:"type"`                   // "tcp" or "serial"
	URL                  string        `yaml:"url"`                    // e.g. "tcp://192.168.1.50:6638" for SLZB-06
	Port                 string        `yaml:"port"`                   // e.g. "/dev/ttyUSB0"
	BaudRate             int           `yaml:"baudrate"`               // e.g. 115200
	ReconnectInterval    time.Duration `yaml:"reconnect_interval"`     // Base reconnect delay
	MaxReconnectInterval time.Duration `yaml:"max_reconnect_interval"` // Max exponential backoff
	TCPKeepAlive         time.Duration `yaml:"tcp_keepalive"`          // TCP keepalive probe interval
	RFC2217              bool          `yaml:"rfc2217"`                // Enable RFC2217 telnet negotiation/bypass
	ReadTimeout          time.Duration `yaml:"read_timeout"`
	WriteTimeout         time.Duration `yaml:"write_timeout"`
}

// AdapterConfig specifies the Zigbee radio coprocessor protocol and PAN parameters.
type AdapterConfig struct {
	Type       AdapterType `yaml:"type"`        // "zstack", "ember", or "mock"
	PanID      uint16      `yaml:"pan_id"`      // 16-bit PAN ID
	ExtPanID   string      `yaml:"ext_pan_id"`  // 64-bit Extended PAN ID (hex string or byte array)
	Channel    uint8       `yaml:"channel"`     // Zigbee channel (11-26)
	NetworkKey string      `yaml:"network_key"` // 16-byte network key (hex string or byte array)
}

// UnmarshalYAML implements custom unmarshaling to support both standard hex string formats
// and Zigbee2MQTT byte arrays (sequences of ints) for ext_pan_id and network_key, plus decimal/hex pan_id.
func (a *AdapterConfig) UnmarshalYAML(node *yaml.Node) error {
	type rawAdapterConfig struct {
		Type       AdapterType `yaml:"type"`
		PanID      yaml.Node   `yaml:"pan_id"`
		ExtPanID   yaml.Node   `yaml:"ext_pan_id"`
		Channel    uint8       `yaml:"channel"`
		NetworkKey yaml.Node   `yaml:"network_key"`
	}

	var raw rawAdapterConfig
	if err := node.Decode(&raw); err != nil {
		return err
	}

	if raw.Type != "" {
		a.Type = raw.Type
	}
	if raw.Channel != 0 {
		a.Channel = raw.Channel
	}

	if !raw.PanID.IsZero() {
		panID, err := parsePanIDNode(&raw.PanID)
		if err != nil {
			return err
		}
		a.PanID = panID
	}

	if !raw.ExtPanID.IsZero() {
		extPanID, err := parseExtPanIDNode(&raw.ExtPanID)
		if err != nil {
			return err
		}
		a.ExtPanID = extPanID
	}

	if !raw.NetworkKey.IsZero() {
		netKey, err := parseNetworkKeyNode(&raw.NetworkKey)
		if err != nil {
			return err
		}
		a.NetworkKey = netKey
	}

	return nil
}

func parsePanIDNode(node *yaml.Node) (uint16, error) {
	if node.Kind != yaml.ScalarNode {
		return 0, fmt.Errorf("pan_id must be an integer or hex string, got node kind %d", node.Kind)
	}
	valStr := strings.TrimSpace(node.Value)
	if strings.EqualFold(valStr, "generate") {
		var b [2]byte
		_, _ = rand.Read(b[:])
		val := (uint16(b[0])<<8 | uint16(b[1])) & 0xFFFE
		if val == 0 {
			val = 0x1A62
		}
		return val, nil
	}
	if strings.HasPrefix(strings.ToLower(valStr), "0x") {
		parsed, err := strconv.ParseUint(valStr[2:], 16, 16)
		if err != nil {
			return 0, fmt.Errorf("invalid hex pan_id '%s': %w", valStr, err)
		}
		return uint16(parsed), nil
	}
	// Try decimal first (e.g. 44982 or 6754)
	parsed, err := strconv.ParseUint(valStr, 10, 16)
	if err == nil {
		return uint16(parsed), nil
	}
	// Try hex without 0x
	parsed, err = strconv.ParseUint(valStr, 16, 16)
	if err == nil {
		return uint16(parsed), nil
	}
	return 0, fmt.Errorf("invalid pan_id '%s': must be a valid 16-bit integer", valStr)
}

func parseExtPanIDNode(node *yaml.Node) (string, error) {
	switch node.Kind {
	case yaml.ScalarNode:
		valStr := strings.TrimSpace(node.Value)
		if strings.EqualFold(valStr, "generate") {
			var b [8]byte
			_, _ = rand.Read(b[:])
			return "0x" + strings.ToUpper(hex.EncodeToString(b[:])), nil
		}
		if !strings.HasPrefix(strings.ToLower(valStr), "0x") {
			valStr = "0x" + valStr
		}
		return strings.ToUpper(valStr[:2]) + strings.ToUpper(valStr[2:]), nil
	case yaml.SequenceNode:
		if len(node.Content) != 8 {
			return "", fmt.Errorf("ext_pan_id byte array must contain exactly 8 bytes (got %d)", len(node.Content))
		}
		var b [8]byte
		for i, elem := range node.Content {
			var val int
			if err := elem.Decode(&val); err != nil {
				return "", fmt.Errorf("invalid byte in ext_pan_id at index %d: %w", i, err)
			}
			if val < 0 || val > 255 {
				return "", fmt.Errorf("ext_pan_id byte at index %d out of range (0-255): %d", i, val)
			}
			b[i] = byte(val)
		}
		return "0x" + strings.ToUpper(hex.EncodeToString(b[:])), nil
	default:
		return "", fmt.Errorf("ext_pan_id must be a hex string or an 8-byte array, got node kind %d", node.Kind)
	}
}

func parseNetworkKeyNode(node *yaml.Node) (string, error) {
	switch node.Kind {
	case yaml.ScalarNode:
		valStr := strings.TrimSpace(node.Value)
		if strings.EqualFold(valStr, "generate") {
			var b [16]byte
			_, _ = rand.Read(b[:])
			return strings.ToUpper(hex.EncodeToString(b[:])), nil
		}
		return valStr, nil
	case yaml.SequenceNode:
		if len(node.Content) != 16 {
			return "", fmt.Errorf("network_key byte array must contain exactly 16 bytes (got %d)", len(node.Content))
		}
		var b [16]byte
		for i, elem := range node.Content {
			var val int
			if err := elem.Decode(&val); err != nil {
				return "", fmt.Errorf("invalid byte in network_key at index %d: %w", i, err)
			}
			if val < 0 || val > 255 {
				return "", fmt.Errorf("network_key byte at index %d out of range (0-255): %d", i, val)
			}
			b[i] = byte(val)
		}
		return strings.ToUpper(hex.EncodeToString(b[:])), nil
	default:
		return "", fmt.Errorf("network_key must be a hex string or a 16-byte array, got node kind %d", node.Kind)
	}
}

// NetworkConfig defines mesh network behaviors.
type NetworkConfig struct {
	PermitJoinDuration uint8 `yaml:"permit_join_duration"` // Default duration in seconds (254 = max, 0 = off)
	PermitJoinOnStart  bool  `yaml:"permit_join_on_start"` // Automatically open joining at startup
}

// MQTTConfig defines MQTT broker settings and Home Assistant discovery.
type MQTTConfig struct {
	Enabled           bool          `yaml:"enabled"`
	Broker            string        `yaml:"broker"` // e.g. "tcp://127.0.0.1:1883"
	ClientID          string        `yaml:"client_id"`
	Username          string        `yaml:"username"`
	Password          string        `yaml:"password"`
	BaseTopic         string        `yaml:"base_topic"`          // Default: "zigbridge"
	HADiscovery       bool          `yaml:"ha_discovery"`        // Enable Home Assistant auto-discovery
	HADiscoveryPrefix string        `yaml:"ha_discovery_prefix"` // Default: "homeassistant"
	Retain            bool          `yaml:"retain"`
	QoS               byte          `yaml:"qos"`
	ConnectionTimeout time.Duration `yaml:"connection_timeout"`
}

// WebConfig defines the embedded web dashboard and REST/WebSocket API server.
type WebConfig struct {
	ListenAddr string `yaml:"listen_addr"` // e.g. "0.0.0.0:8080"
	EnableCORS bool   `yaml:"enable_cors"`
}

// AIConfig defines the direct binding and scene recommendation engine settings.
type AIConfig struct {
	Enabled          bool          `yaml:"enabled"`
	Engine           string        `yaml:"engine"`            // "rule_based" or "external_llm"
	AnalysisInterval time.Duration `yaml:"analysis_interval"` // e.g. "5m"
	MinConfidence    float64       `yaml:"min_confidence"`    // 0.0 - 1.0 threshold for auto-proposals
	MaxEventHistory  int           `yaml:"max_event_history"` // Max events retained in ring buffer
	LLMEndpoint      string        `yaml:"llm_endpoint"`      // Optional endpoint for local or cloud LLM
	LLMAPIKey        string        `yaml:"llm_api_key"`       // Optional API key for LLM
}

// StorageConfig defines data persistence and runtime state directory settings.
type StorageConfig struct {
	DevicesPath string        `yaml:"devices_path"`      // e.g. "data/devices.yaml"
	Debounce    time.Duration `yaml:"debounce_interval"` // e.g. 2s
}

// Default returns a configuration populated with safe, production-grade defaults.
func Default() *Config {
	return &Config{
		LogLevel: "info",
		Transport: TransportConfig{
			Type:                 TransportTypeTCP,
			URL:                  "tcp://192.168.1.50:6638",
			Port:                 "/dev/ttyUSB0",
			BaudRate:             115200,
			ReconnectInterval:    2 * time.Second,
			MaxReconnectInterval: 30 * time.Second,
			TCPKeepAlive:         10 * time.Second,
			RFC2217:              false,
			ReadTimeout:          10 * time.Second,
			WriteTimeout:         5 * time.Second,
		},
		Adapter: AdapterConfig{
			Type:       AdapterTypeZStack,
			PanID:      0x1A62,
			ExtPanID:   "0xDDDDDDDDDDDDDDDD",
			Channel:    20,
			NetworkKey: "01030507090B0D0F00020406080A0C0D",
		},
		Network: NetworkConfig{
			PermitJoinDuration: 254,
			PermitJoinOnStart:  false,
		},
		MQTT: MQTTConfig{
			Enabled:           true,
			Broker:            "tcp://localhost:1883",
			ClientID:          "zigbridge",
			BaseTopic:         "zigbridge",
			HADiscovery:       true,
			HADiscoveryPrefix: "homeassistant",
			Retain:            true,
			QoS:               0,
			ConnectionTimeout: 10 * time.Second,
		},
		Web: WebConfig{
			ListenAddr: "0.0.0.0:8080",
			EnableCORS: true,
		},
		AI: AIConfig{
			Enabled:          true,
			Engine:           "rule_based",
			AnalysisInterval: 5 * time.Minute,
			MinConfidence:    0.75,
			MaxEventHistory:  2000,
		},
		Storage: StorageConfig{
			DevicesPath: "data/devices.yaml",
			Debounce:    2 * time.Second,
		},
	}
}

// Load loads configuration from a YAML file, falling back to defaults for omitted fields.
func Load(path string) (*Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found: %s", path)
		}
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse yaml config: %w", err)
	}

	// Support Zigbee2MQTT-style "advanced:" section by merging into Adapter
	if cfg.Advanced != nil {
		if cfg.Advanced.PanID != 0 {
			cfg.Adapter.PanID = cfg.Advanced.PanID
		}
		if cfg.Advanced.ExtPanID != "" {
			cfg.Adapter.ExtPanID = cfg.Advanced.ExtPanID
		}
		if cfg.Advanced.Channel != 0 {
			cfg.Adapter.Channel = cfg.Advanced.Channel
		}
		if cfg.Advanced.NetworkKey != "" {
			cfg.Adapter.NetworkKey = cfg.Advanced.NetworkKey
		}
		if cfg.Advanced.Type != "" {
			cfg.Adapter.Type = cfg.Advanced.Type
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

// Validate verifies that required fields are present and ranges are valid.
func (c *Config) Validate() error {
	switch c.Transport.Type {
	case TransportTypeTCP:
		if c.Transport.URL == "" {
			return fmt.Errorf("transport.url is required when transport.type is tcp")
		}
	case TransportTypeSerial:
		if c.Transport.Port == "" {
			return fmt.Errorf("transport.port is required when transport.type is serial")
		}
		if c.Transport.BaudRate <= 0 {
			return fmt.Errorf("transport.baudrate must be > 0")
		}
	case TransportTypeMock:
		// Always valid
	default:
		return fmt.Errorf("unsupported transport type: %s (supported: tcp, serial, mock)", c.Transport.Type)
	}

	switch c.Adapter.Type {
	case AdapterTypeZStack, AdapterTypeEmber, AdapterTypeMock:
		// Valid
	default:
		return fmt.Errorf("unsupported adapter type: %s (supported: zstack, ember, mock)", c.Adapter.Type)
	}

	if c.Adapter.Channel < 11 || c.Adapter.Channel > 26 {
		return fmt.Errorf("adapter.channel must be between 11 and 26 (got %d)", c.Adapter.Channel)
	}

	if c.Web.ListenAddr == "" {
		return fmt.Errorf("web.listen_addr cannot be empty")
	}

	if c.Storage.DevicesPath == "" {
		c.Storage.DevicesPath = "data/devices.yaml"
	}

	return nil
}

// ResolveConfigPath determines the active configuration file path:
// 1. If explicit is specified and not standard defaults, return explicit.
// 2. If explicit is "config.yaml":
//   - If config.yaml exists -> return "config.yaml"
//   - If data/config.yaml exists -> return "data/config.yaml"
//   - Return "config.yaml"
//
// 3. If explicit is "data/config.yaml", return "data/config.yaml".
// 4. If explicit is empty:
//   - If data/config.yaml exists -> return "data/config.yaml"
//   - If config.yaml exists -> return "config.yaml"
//   - Fall back to "data/config.yaml"
func ResolveConfigPath(explicit string) string {
	if explicit != "" && explicit != "config.yaml" && explicit != "data/config.yaml" {
		return explicit
	}
	if explicit == "config.yaml" {
		if _, err := os.Stat("config.yaml"); err == nil {
			return "config.yaml"
		}
		if _, err := os.Stat("data/config.yaml"); err == nil {
			return "data/config.yaml"
		}
		return "config.yaml"
	}
	if explicit == "data/config.yaml" {
		return "data/config.yaml"
	}

	// Unspecified / default
	if _, err := os.Stat("data/config.yaml"); err == nil {
		return "data/config.yaml"
	}
	if _, err := os.Stat("config.yaml"); err == nil {
		return "config.yaml"
	}
	return "data/config.yaml"
}
