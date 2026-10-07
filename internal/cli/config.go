package cli

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/julienbreux/zigbridge/internal/config"
	"github.com/spf13/viper"
)

// DefaultConfigFiles lists configuration filenames checked in priority order
// when no explicit configuration file path is provided.
var DefaultConfigFiles = []string{
	"data/config.yaml",
	"config.yaml",
	"zigbridge.yaml",
	"zigbridge.json",
	"config.yaml.dist",
}

// InitViper sets up Viper with environment variable prefixes, key replacers,
// and default configuration values from config.Default().
func InitViper(v *viper.Viper) {
	v.SetEnvPrefix("ZIGBRIDGE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	def := config.Default()
	v.SetDefault("log_level", def.LogLevel)

	v.SetDefault("transport.type", string(def.Transport.Type))
	v.SetDefault("transport.url", def.Transport.URL)
	v.SetDefault("transport.port", def.Transport.Port)
	v.SetDefault("transport.baudrate", def.Transport.BaudRate)
	v.SetDefault("transport.reconnect_interval", def.Transport.ReconnectInterval)
	v.SetDefault("transport.max_reconnect_interval", def.Transport.MaxReconnectInterval)
	v.SetDefault("transport.tcp_keepalive", def.Transport.TCPKeepAlive)
	v.SetDefault("transport.rfc2217", def.Transport.RFC2217)
	v.SetDefault("transport.read_timeout", def.Transport.ReadTimeout)
	v.SetDefault("transport.write_timeout", def.Transport.WriteTimeout)

	v.SetDefault("adapter.type", string(def.Adapter.Type))
	v.SetDefault("adapter.pan_id", def.Adapter.PanID)
	v.SetDefault("adapter.ext_pan_id", def.Adapter.ExtPanID)
	v.SetDefault("adapter.channel", def.Adapter.Channel)
	v.SetDefault("adapter.network_key", def.Adapter.NetworkKey)

	v.SetDefault("network.permit_join_duration", def.Network.PermitJoinDuration)
	v.SetDefault("network.permit_join_on_start", def.Network.PermitJoinOnStart)

	v.SetDefault("mqtt.enabled", def.MQTT.Enabled)
	v.SetDefault("mqtt.broker", def.MQTT.Broker)
	v.SetDefault("mqtt.client_id", def.MQTT.ClientID)
	v.SetDefault("mqtt.username", def.MQTT.Username)
	v.SetDefault("mqtt.password", def.MQTT.Password)
	v.SetDefault("mqtt.base_topic", def.MQTT.BaseTopic)
	v.SetDefault("mqtt.ha_discovery", def.MQTT.HADiscovery)
	v.SetDefault("mqtt.ha_discovery_prefix", def.MQTT.HADiscoveryPrefix)
	v.SetDefault("mqtt.retain", def.MQTT.Retain)
	v.SetDefault("mqtt.qos", def.MQTT.QoS)
	v.SetDefault("mqtt.connection_timeout", def.MQTT.ConnectionTimeout)

	v.SetDefault("web.listen_addr", def.Web.ListenAddr)
	v.SetDefault("web.enable_cors", def.Web.EnableCORS)

	v.SetDefault("ai.enabled", def.AI.Enabled)
	v.SetDefault("ai.engine", def.AI.Engine)
	v.SetDefault("ai.analysis_interval", def.AI.AnalysisInterval)
	v.SetDefault("ai.min_confidence", def.AI.MinConfidence)
	v.SetDefault("ai.max_event_history", def.AI.MaxEventHistory)
	v.SetDefault("ai.llm_endpoint", def.AI.LLMEndpoint)
	v.SetDefault("ai.llm_api_key", def.AI.LLMAPIKey)

	v.SetDefault("storage.devices_path", def.Storage.DevicesPath)
	v.SetDefault("storage.debounce_interval", def.Storage.Debounce)
}

// LoadConfig resolves, reads, and unmarshals application configuration from Viper.
// If explicitPath is supplied, it is loaded directly. Otherwise, candidate files are checked.
func LoadConfig(v *viper.Viper, explicitPath string) (*config.Config, string, error) {
	var loadedFile string

	if explicitPath != "" {
		v.SetConfigFile(explicitPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, "", fmt.Errorf("failed to read config file '%s': %w", explicitPath, err)
		}
		loadedFile = explicitPath
	} else {
		for _, candidate := range DefaultConfigFiles {
			if _, err := os.Stat(candidate); err == nil {
				v.SetConfigFile(candidate)
				if err := v.ReadInConfig(); err != nil {
					return nil, "", fmt.Errorf("failed to read config file '%s': %w", candidate, err)
				}
				loadedFile = candidate
				break
			}
		}
	}

	cfg := config.Default()
	decoderOpt := viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
		panIDDecodeHook(),
		byteArrayDecodeHook(),
	))

	if err := v.Unmarshal(cfg, decoderOpt); err != nil {
		return nil, loadedFile, fmt.Errorf("failed to unmarshal configuration: %w", err)
	}

	// Normalize ExtPanID and NetworkKey if generated
	if strings.EqualFold(cfg.Adapter.ExtPanID, "generate") {
		genExtPanID, err := config.ParseExtPanID("generate")
		if err != nil {
			return nil, loadedFile, err
		}
		cfg.Adapter.ExtPanID = genExtPanID
	}
	if strings.EqualFold(cfg.Adapter.NetworkKey, "generate") {
		genNetKey, err := config.ParseNetworkKey("generate")
		if err != nil {
			return nil, loadedFile, err
		}
		cfg.Adapter.NetworkKey = genNetKey
	}

	// Merge legacy Zigbee2MQTT advanced section into adapter if provided
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
		return nil, loadedFile, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, loadedFile, nil
}

func panIDDecodeHook() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if t != reflect.TypeFor[uint16]() {
			return data, nil
		}
		return config.ParsePanID(data)
	}
}

func byteArrayDecodeHook() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if t != reflect.TypeFor[string]() {
			return data, nil
		}
		if f.Kind() == reflect.Slice {
			if sliceVal, ok := data.([]any); ok {
				if len(sliceVal) == 8 {
					return config.ParseExtPanID(data)
				}
				if len(sliceVal) == 16 {
					return config.ParseNetworkKey(data)
				}
			}
			if sliceInt, ok := data.([]int); ok {
				if len(sliceInt) == 8 {
					return config.ParseExtPanID(data)
				}
				if len(sliceInt) == 16 {
					return config.ParseNetworkKey(data)
				}
			}
		}
		return data, nil
	}
}
