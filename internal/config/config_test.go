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

func TestLoadZigbee2MQTTByteArrays(t *testing.T) {
	yamlContent := `
log_level: info
transport:
  type: mock
adapter:
  type: zstack
  pan_id: 44982
  ext_pan_id:
    - 109
    - 49
    - 229
    - 86
    - 20
    - 36
    - 157
    - 243
  network_key:
    - 231
    - 91
    - 246
    - 24
    - 24
    - 137
    - 13
    - 121
    - 166
    - 23
    - 76
    - 163
    - 175
    - 54
    - 211
    - 111
web:
  listen_addr: "127.0.0.1:8080"
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
		t.Fatalf("unexpected error loading Zigbee2MQTT config: %v", err)
	}

	if cfg.Adapter.PanID != 44982 {
		t.Errorf("expected PanID 44982 (0x%04X), got %d (0x%04X)", 44982, cfg.Adapter.PanID, cfg.Adapter.PanID)
	}

	expectedExtPanID := "0x6D31E55614249DF3"
	if cfg.Adapter.ExtPanID != expectedExtPanID {
		t.Errorf("expected ExtPanID %s, got %s", expectedExtPanID, cfg.Adapter.ExtPanID)
	}

	expectedNetKey := "E75BF61818890D79A6174CA3AF36D36F"
	if cfg.Adapter.NetworkKey != expectedNetKey {
		t.Errorf("expected NetworkKey %s, got %s", expectedNetKey, cfg.Adapter.NetworkKey)
	}
}

func TestLoadZigbee2MQTTAdvancedSection(t *testing.T) {
	yamlContent := `
log_level: info
transport:
  type: mock
advanced:
  pan_id: 44982
  channel: 15
  ext_pan_id:
    - 109
    - 49
    - 229
    - 86
    - 20
    - 36
    - 157
    - 243
  network_key:
    - 231
    - 91
    - 246
    - 24
    - 24
    - 137
    - 13
    - 121
    - 166
    - 23
    - 76
    - 163
    - 175
    - 54
    - 211
    - 111
web:
  listen_addr: "127.0.0.1:8080"
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
		t.Fatalf("unexpected error loading config with advanced section: %v", err)
	}

	if cfg.Adapter.PanID != 44982 {
		t.Errorf("expected PanID 44982, got %d", cfg.Adapter.PanID)
	}
	if cfg.Adapter.Channel != 15 {
		t.Errorf("expected Channel 15, got %d", cfg.Adapter.Channel)
	}
	if cfg.Adapter.ExtPanID != "0x6D31E55614249DF3" {
		t.Errorf("expected ExtPanID 0x6D31E55614249DF3, got %s", cfg.Adapter.ExtPanID)
	}
	if cfg.Adapter.NetworkKey != "E75BF61818890D79A6174CA3AF36D36F" {
		t.Errorf("expected NetworkKey E75BF61818890D79A6174CA3AF36D36F, got %s", cfg.Adapter.NetworkKey)
	}
}

func TestPanIDAndKeyGenerate(t *testing.T) {
	yamlContent := `
log_level: info
transport:
  type: mock
adapter:
  pan_id: "GENERATE"
  ext_pan_id: "GENERATE"
  network_key: "GENERATE"
web:
  listen_addr: "127.0.0.1:8080"
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
		t.Fatalf("unexpected error with GENERATE config: %v", err)
	}

	if cfg.Adapter.PanID == 0 {
		t.Error("expected generated non-zero PanID")
	}
	if len(cfg.Adapter.ExtPanID) != 18 { // "0x" + 16 hex chars
		t.Errorf("expected 18 char ExtPanID, got %s", cfg.Adapter.ExtPanID)
	}
	if len(cfg.Adapter.NetworkKey) != 32 {
		t.Errorf("expected 32 char NetworkKey, got %s", cfg.Adapter.NetworkKey)
	}
}

