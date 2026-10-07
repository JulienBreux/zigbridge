package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/julienbreux/zigbridge/internal/config"
	"github.com/julienbreux/zigbridge/internal/version"
	"github.com/spf13/viper"
)

func TestVersionCommands(t *testing.T) {
	version.Version = "1.2.3"
	version.Commit = "abcdef"
	version.BuildDate = "2026-10-07"

	t.Run("version subcommand outputs build info", func(t *testing.T) {
		cmd := NewRootCmd(viper.New())
		var stdout bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetArgs([]string{"version"})

		if err := cmd.ExecuteContext(t.Context()); err != nil {
			t.Fatalf("unexpected error running version subcommand: %v", err)
		}

		out := stdout.String()
		if !strings.Contains(out, "zigbridge version 1.2.3") || !strings.Contains(out, "abcdef") {
			t.Errorf("unexpected output: %s", out)
		}
	})

	t.Run("--version flag outputs build info", func(t *testing.T) {
		cmd := NewRootCmd(viper.New())
		var stdout bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetArgs([]string{"--version"})

		if err := cmd.ExecuteContext(t.Context()); err != nil {
			t.Fatalf("unexpected error running with --version flag: %v", err)
		}

		out := stdout.String()
		if !strings.Contains(out, "zigbridge version 1.2.3") || !strings.Contains(out, "abcdef") {
			t.Errorf("unexpected output: %s", out)
		}
	})

	t.Run("-v flag outputs build info", func(t *testing.T) {
		cmd := NewRootCmd(viper.New())
		var stdout bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetArgs([]string{"-v"})

		if err := cmd.ExecuteContext(t.Context()); err != nil {
			t.Fatalf("unexpected error running with -v flag: %v", err)
		}

		out := stdout.String()
		if !strings.Contains(out, "zigbridge version 1.2.3") {
			t.Errorf("unexpected output: %s", out)
		}
	})
}

func TestViperFlagBinding(t *testing.T) {
	v := viper.New()
	cmd := NewRootCmd(v)

	args := []string{
		"--log-level=debug",
		"--web-listen-addr=127.0.0.1:9099",
		"--transport-type=tcp",
		"--transport-url=192.168.1.50:6638",
		"--mqtt-broker=tcp://127.0.0.1:1883",
		"--adapter-type=ember",
		"--adapter-channel=20",
	}

	if err := cmd.Flags().Parse(args); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	if val := v.GetString("log_level"); val != "debug" {
		t.Errorf("expected log_level 'debug', got '%s'", val)
	}
	if val := v.GetString("web.listen_addr"); val != "127.0.0.1:9099" {
		t.Errorf("expected web.listen_addr '127.0.0.1:9099', got '%s'", val)
	}
	if val := v.GetString("transport.type"); val != "tcp" {
		t.Errorf("expected transport.type 'tcp', got '%s'", val)
	}
	if val := v.GetString("transport.url"); val != "192.168.1.50:6638" {
		t.Errorf("expected transport.url '192.168.1.50:6638', got '%s'", val)
	}
	if val := v.GetString("mqtt.broker"); val != "tcp://127.0.0.1:1883" {
		t.Errorf("expected mqtt.broker 'tcp://127.0.0.1:1883', got '%s'", val)
	}
	if val := v.GetString("adapter.type"); val != "ember" {
		t.Errorf("expected adapter.type 'ember', got '%s'", val)
	}
	if val := v.GetUint("adapter.channel"); val != 20 {
		t.Errorf("expected adapter.channel 20, got %d", val)
	}
}

func TestViperEnvOverrides(t *testing.T) {
	t.Setenv("ZIGBRIDGE_LOG_LEVEL", "warn")
	t.Setenv("ZIGBRIDGE_WEB_LISTEN_ADDR", "127.0.0.1:9090")
	t.Setenv("ZIGBRIDGE_TRANSPORT_TYPE", "mock")
	t.Setenv("ZIGBRIDGE_ADAPTER_TYPE", "mock")
	t.Setenv("ZIGBRIDGE_ADAPTER_CHANNEL", "25")

	v := viper.New()
	InitViper(v)

	cfg, _, err := LoadConfig(v, "")
	if err != nil {
		t.Fatalf("failed to load config with env overrides: %v", err)
	}

	if cfg.LogLevel != "warn" {
		t.Errorf("expected LogLevel 'warn', got '%s'", cfg.LogLevel)
	}
	if cfg.Web.ListenAddr != "127.0.0.1:9090" {
		t.Errorf("expected Web.ListenAddr '127.0.0.1:9090', got '%s'", cfg.Web.ListenAddr)
	}
	if cfg.Transport.Type != config.TransportTypeMock {
		t.Errorf("expected Transport.Type 'mock', got '%s'", cfg.Transport.Type)
	}
	if cfg.Adapter.Type != config.AdapterTypeMock {
		t.Errorf("expected Adapter.Type 'mock', got '%s'", cfg.Adapter.Type)
	}
	if cfg.Adapter.Channel != 25 {
		t.Errorf("expected Adapter.Channel 25, got %d", cfg.Adapter.Channel)
	}
}

func TestLoadConfigFileFormats(t *testing.T) {
	t.Run("load YAML configuration", func(t *testing.T) {
		tmpDir := t.TempDir()
		yamlPath := filepath.Join(tmpDir, "zigbridge.yaml")
		yamlContent := `
log_level: debug
transport:
  type: tcp
  url: 192.168.1.100:6638
adapter:
  type: zstack
  channel: 15
  pan_id: 0x1A62
  ext_pan_id: "0x00124B001CD4ABCD"
mqtt:
  broker: tcp://10.0.0.1:1883
  enabled: true
web:
  listen_addr: 0.0.0.0:8088
`
		if err := os.WriteFile(yamlPath, []byte(yamlContent), 0o600); err != nil {
			t.Fatalf("failed to write test yaml: %v", err)
		}

		v := viper.New()
		InitViper(v)

		cfg, loadedFile, err := LoadConfig(v, yamlPath)
		if err != nil {
			t.Fatalf("failed to load YAML config: %v", err)
		}
		if loadedFile != yamlPath {
			t.Errorf("expected loaded file %s, got %s", yamlPath, loadedFile)
		}
		if cfg.LogLevel != "debug" {
			t.Errorf("expected log_level 'debug', got %s", cfg.LogLevel)
		}
		if cfg.Transport.Type != config.TransportTypeTCP || cfg.Transport.URL != "192.168.1.100:6638" {
			t.Errorf("unexpected transport config: %+v", cfg.Transport)
		}
		if cfg.Adapter.PanID != 0x1A62 {
			t.Errorf("expected PanID 0x1A62, got 0x%04X", cfg.Adapter.PanID)
		}
		if cfg.Web.ListenAddr != "0.0.0.0:8088" {
			t.Errorf("expected Web.ListenAddr '0.0.0.0:8088', got %s", cfg.Web.ListenAddr)
		}
	})

	t.Run("load JSON configuration", func(t *testing.T) {
		tmpDir := t.TempDir()
		jsonPath := filepath.Join(tmpDir, "zigbridge.json")
		jsonContent := `{
  "log_level": "info",
  "transport": {
    "type": "serial",
    "port": "/dev/ttyACM0"
  },
  "adapter": {
    "type": "ember",
    "channel": 25,
    "pan_id": 6754
  },
  "mqtt": {
    "enabled": false
  }
}`
		if err := os.WriteFile(jsonPath, []byte(jsonContent), 0o600); err != nil {
			t.Fatalf("failed to write test json: %v", err)
		}

		v := viper.New()
		InitViper(v)

		cfg, loadedFile, err := LoadConfig(v, jsonPath)
		if err != nil {
			t.Fatalf("failed to load JSON config: %v", err)
		}
		if loadedFile != jsonPath {
			t.Errorf("expected loaded file %s, got %s", jsonPath, loadedFile)
		}
		if cfg.Transport.Type != config.TransportTypeSerial || cfg.Transport.Port != "/dev/ttyACM0" {
			t.Errorf("unexpected transport config: %+v", cfg.Transport)
		}
		if cfg.Adapter.PanID != 6754 {
			t.Errorf("expected PanID 6754, got %d", cfg.Adapter.PanID)
		}
	})

	t.Run("load with advanced section and byte array keys", func(t *testing.T) {
		tmpDir := t.TempDir()
		yamlPath := filepath.Join(tmpDir, "zigbridge.yaml")
		yamlContent := `
transport:
  type: mock
adapter:
  type: mock
advanced:
  pan_id: 0x1A62
  channel: 25
  ext_pan_id: [0xDD, 0xDD, 0xDD, 0xDD, 0xDD, 0xDD, 0xDD, 0xDD]
  network_key: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16]
`
		if err := os.WriteFile(yamlPath, []byte(yamlContent), 0o600); err != nil {
			t.Fatalf("failed to write test yaml: %v", err)
		}

		v := viper.New()
		InitViper(v)

		cfg, _, err := LoadConfig(v, yamlPath)
		if err != nil {
			t.Fatalf("failed to load advanced config: %v", err)
		}

		if cfg.Adapter.PanID != 0x1A62 {
			t.Errorf("expected PanID 0x1A62, got 0x%04X", cfg.Adapter.PanID)
		}
		if cfg.Adapter.Channel != 25 {
			t.Errorf("expected Channel 25, got %d", cfg.Adapter.Channel)
		}
		if cfg.Adapter.ExtPanID != "0xDDDDDDDDDDDDDDDD" {
			t.Errorf("expected ExtPanID 0xDDDDDDDDDDDDDDDD, got %s", cfg.Adapter.ExtPanID)
		}
		if cfg.Adapter.NetworkKey != "0102030405060708090A0B0C0D0E0F10" {
			t.Errorf("expected NetworkKey 0102030405060708090A0B0C0D0E0F10, got %s", cfg.Adapter.NetworkKey)
		}
	})

	t.Run("validation failure on invalid channel", func(t *testing.T) {
		tmpDir := t.TempDir()
		yamlPath := filepath.Join(tmpDir, "invalid.yaml")
		yamlContent := `
adapter:
  channel: 99
`
		if err := os.WriteFile(yamlPath, []byte(yamlContent), 0o600); err != nil {
			t.Fatalf("failed to write invalid yaml: %v", err)
		}

		v := viper.New()
		InitViper(v)

		_, _, err := LoadConfig(v, yamlPath)
		if err == nil {
			t.Fatal("expected error on invalid channel, got nil")
		}
		if !strings.Contains(err.Error(), "invalid configuration") {
			t.Errorf("expected invalid configuration error, got: %v", err)
		}
	})

	t.Run("bridge graceful shutdown on context cancellation", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "zigbridge.yaml")
		devPath := filepath.Join(tmpDir, "devices.yaml")
		cfgContent := `
transport:
  type: mock
adapter:
  type: mock
mqtt:
  enabled: false
web:
  listen_addr: 127.0.0.1:0
storage:
  devices_path: ` + devPath + `
`
		if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
			t.Fatalf("failed to write test config: %v", err)
		}

		ctx, cancel := context.WithCancel(t.Context())

		v := viper.New()
		cmd := NewRootCmd(v)
		cmd.SetArgs([]string{"--config", cfgPath})

		// Cancel context shortly after launch
		go func() {
			time.Sleep(100 * time.Millisecond)
			cancel()
		}()

		var stdout bytes.Buffer
		cmd.SetOut(&stdout)

		if err := cmd.ExecuteContext(ctx); err != nil {
			t.Fatalf("expected graceful shutdown without error, got: %v", err)
		}

		if !strings.Contains(stdout.String(), "Zigbee-to-MQTT Bridge") {
			t.Errorf("expected banner in stdout, got: %s", stdout.String())
		}
	})
}
