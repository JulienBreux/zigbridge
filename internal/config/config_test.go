package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := Default()
	if cfg == nil {
		t.Fatal("expected non-nil default config")
	}

	if cfg.Transport.Type != TransportTypeTCP {
		t.Errorf("expected transport type tcp, got %s", cfg.Transport.Type)
	}

	if cfg.Adapter.Channel != 20 {
		t.Errorf("expected channel 20, got %d", cfg.Adapter.Channel)
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config failed validation: %v", err)
	}
}

func TestLoadConfigFile(t *testing.T) {
	yamlContent := `
log_level: debug
transport:
  type: tcp
  url: "tcp://10.0.0.120:6638"
adapter:
  type: ember
  channel: 25
  pan_id: 0x2B34
web:
  listen_addr: "127.0.0.1:9090"
mqtt:
  enabled: false
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	if cfg.LogLevel != "debug" {
		t.Errorf("expected log_level debug, got %s", cfg.LogLevel)
	}

	if cfg.Transport.URL != "tcp://10.0.0.120:6638" {
		t.Errorf("expected url tcp://10.0.0.120:6638, got %s", cfg.Transport.URL)
	}

	if cfg.Adapter.Type != AdapterTypeEmber {
		t.Errorf("expected adapter type ember, got %s", cfg.Adapter.Type)
	}

	if cfg.Adapter.Channel != 25 {
		t.Errorf("expected channel 25, got %d", cfg.Adapter.Channel)
	}

	if cfg.Adapter.PanID != 0x2B34 {
		t.Errorf("expected pan_id 0x2B34, got 0x%X", cfg.Adapter.PanID)
	}

	if cfg.MQTT.Enabled != false {
		t.Errorf("expected mqtt enabled false, got %v", cfg.MQTT.Enabled)
	}
}

func TestConfigValidationErrors(t *testing.T) {
	cfg := Default()
	cfg.Adapter.Channel = 5 // invalid channel
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid channel 5, got nil")
	}

	cfg = Default()
	cfg.Transport.Type = TransportTypeTCP
	cfg.Transport.URL = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty TCP url, got nil")
	}

	cfg = Default()
	cfg.Transport.Type = "unknown"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for unknown transport type, got nil")
	}

	cfg = Default()
	cfg.Storage.DevicesPath = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Storage.DevicesPath != "data/devices.yaml" {
		t.Errorf("expected default devices_path data/devices.yaml, got %s", cfg.Storage.DevicesPath)
	}
}

func TestResolveConfigPath(t *testing.T) {
	// 1. Explicit custom path
	if got := ResolveConfigPath("custom/path.yaml"); got != "custom/path.yaml" {
		t.Errorf("expected custom/path.yaml, got %s", got)
	}

	// 2. Explicit data/config.yaml
	if got := ResolveConfigPath("data/config.yaml"); got != "data/config.yaml" {
		t.Errorf("expected data/config.yaml, got %s", got)
	}

	// 3. In an isolated temp dir: neither exists -> fallback to data/config.yaml
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	if got := ResolveConfigPath(""); got != "data/config.yaml" {
		t.Errorf("expected fallback data/config.yaml, got %s", got)
	}
	if got := ResolveConfigPath("config.yaml"); got != "config.yaml" {
		t.Errorf("expected config.yaml when neither exists, got %s", got)
	}

	// 4. Create root config.yaml
	if err := os.WriteFile("config.yaml", []byte("log_level: info\n"), 0644); err != nil {
		t.Fatalf("failed to write config.yaml: %v", err)
	}
	if got := ResolveConfigPath(""); got != "config.yaml" {
		t.Errorf("expected config.yaml, got %s", got)
	}
	if got := ResolveConfigPath("config.yaml"); got != "config.yaml" {
		t.Errorf("expected config.yaml, got %s", got)
	}

	// 5. Create data/config.yaml (takes precedence over root config.yaml)
	if err := os.MkdirAll("data", 0755); err != nil {
		t.Fatalf("failed to mkdir data: %v", err)
	}
	if err := os.WriteFile("data/config.yaml", []byte("log_level: debug\n"), 0644); err != nil {
		t.Fatalf("failed to write data/config.yaml: %v", err)
	}
	if got := ResolveConfigPath(""); got != "data/config.yaml" {
		t.Errorf("expected data/config.yaml to take precedence, got %s", got)
	}
	// When "config.yaml" is passed but "data/config.yaml" exists and config.yaml also exists:
	// Since config.yaml exists, explicit config.yaml uses config.yaml
	if got := ResolveConfigPath("config.yaml"); got != "config.yaml" {
		t.Errorf("expected config.yaml when explicit, got %s", got)
	}
	// If config.yaml is removed, explicit "config.yaml" falls back to data/config.yaml
	_ = os.Remove("config.yaml")
	if got := ResolveConfigPath("config.yaml"); got != "data/config.yaml" {
		t.Errorf("expected data/config.yaml when config.yaml missing, got %s", got)
	}
}
