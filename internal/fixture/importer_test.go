package fixture_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/julienbreux/zigbridge/internal/fixture"
	"gopkg.in/yaml.v3"
)

type mockHTTPFetcher struct {
	statusCode int
	body       string
	err        error
}

func (m *mockHTTPFetcher) Get(url string) (*http.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	resp := &http.Response{
		StatusCode: m.statusCode,
		Status:     http.StatusText(m.statusCode),
		Body:       io.NopCloser(bytes.NewBufferString(m.body)),
	}
	return resp, nil
}

func TestGenerateFilenameSlug(t *testing.T) {
	tests := []struct {
		vendor   string
		model    string
		expected string
	}{
		{"SONOFF", "SNZB-01P", "sonoff_snzb_01p.yaml"},
		{"IKEA", "TRADFRI bulb E27", "ikea_tradfri_bulb_e27.yaml"},
		{"Philips", "Hue Go (White & Color)", "philips_hue_go_white_color.yaml"},
		{"TuYa", "TS004F", "tuya_ts004f.yaml"},
	}

	for _, tt := range tests {
		got := fixture.GenerateFilenameSlug(tt.vendor, tt.model)
		if got != tt.expected {
			t.Errorf("GenerateFilenameSlug(%q, %q) = %q, want %q", tt.vendor, tt.model, got, tt.expected)
		}
	}
}

func TestImportFromHTML(t *testing.T) {
	sampleHTML := `<!DOCTYPE html>
<html>
<head>
    <title>SONOFF SNZB-01P control via MQTT | Zigbee2MQTT</title>
</head>
<body>
    <h1>SNZB-01P</h1>
    <table>
        <tr><td>Model</td><td>SNZB-01P</td></tr>
        <tr><td>Vendor</td><td>SONOFF</td></tr>
        <tr><td>Description</td><td>Wireless button switch</td></tr>
        <tr><td>Zigbee Models</td><td>SNZB-01P, WB01</td></tr>
    </table>
    <div class="content">
        <h3>Action (enum)</h3>
        <p>Triggered action (e.g. a button press). Possible values: single, double, long.</p>
        <h3>Battery (numeric)</h3>
        <p>Remaining battery in %.</p>
        <h3>Voltage (numeric)</h3>
        <p>Reported battery voltage in millivolts.</p>
    </div>
</body>
</html>`

	imp := fixture.NewImporter(nil)
	def, err := imp.ImportFromHTML(sampleHTML, "https://www.zigbee2mqtt.io/devices/SNZB-01P.html")
	if err != nil {
		t.Fatalf("ImportFromHTML failed: %v", err)
	}

	if def.Device.Model != "SNZB-01P" {
		t.Errorf("expected Model SNZB-01P, got %s", def.Device.Model)
	}
	if def.Device.Vendor != "SONOFF" {
		t.Errorf("expected Vendor SONOFF, got %s", def.Device.Vendor)
	}
	if def.Device.Description != "Wireless button switch" {
		t.Errorf("expected Description 'Wireless button switch', got %s", def.Device.Description)
	}
	if len(def.Device.ZigbeeModels) != 2 {
		t.Fatalf("expected 2 zigbee models, got %d", len(def.Device.ZigbeeModels))
	}
	if def.Device.ZigbeeModels[0] != "SNZB-01P" || def.Device.ZigbeeModels[1] != "WB01" {
		t.Errorf("unexpected zigbee models: %v", def.Device.ZigbeeModels)
	}

	// Verify exposes
	hasAction := false
	hasBattery := false
	hasVoltage := false
	hasLQI := false
	for _, exp := range def.Device.Exposes {
		switch exp.Property {
		case "action":
			hasAction = true
		case "battery":
			hasBattery = true
		case "voltage":
			hasVoltage = true
		case "linkquality":
			hasLQI = true
		}
	}
	if !hasAction || !hasBattery || !hasVoltage || !hasLQI {
		t.Errorf("missing expected exposes: action=%v, battery=%v, voltage=%v, lqi=%v", hasAction, hasBattery, hasVoltage, hasLQI)
	}

	// Verify simulations
	if _, ok := def.Device.Simulations.Actions["single"]; !ok {
		t.Errorf("expected 'single' action in simulations")
	}
	if _, ok := def.Device.Simulations.Actions["double"]; !ok {
		t.Errorf("expected 'double' action in simulations")
	}
	if _, ok := def.Device.Simulations.Actions["long"]; !ok {
		t.Errorf("expected 'long' action in simulations")
	}
	if _, ok := def.Device.Simulations.Telemetry["battery"]; !ok {
		t.Errorf("expected 'battery' telemetry in simulations")
	}
}

func TestImportFromURL(t *testing.T) {
	sampleHTML := `<!DOCTYPE html>
<html>
<head><title>SONOFF SNZB-02P control via MQTT | Zigbee2MQTT</title></head>
<body>
    <table>
        <tr><td>Model</td><td>SNZB-02P</td></tr>
        <tr><td>Vendor</td><td>SONOFF</td></tr>
        <tr><td>Description</td><td>Temperature and humidity sensor</td></tr>
        <tr><td>Zigbee Models</td><td>SNZB-02P</td></tr>
    </table>
    <div>
        <h3>Temperature (numeric)</h3>
        <h3>Humidity (numeric)</h3>
        <h3>Battery (numeric)</h3>
    </div>
</body>
</html>`

	mockFetcher := &mockHTTPFetcher{
		statusCode: http.StatusOK,
		body:       sampleHTML,
	}

	imp := fixture.NewImporter(mockFetcher)
	def, err := imp.ImportFromURL("https://www.zigbee2mqtt.io/devices/SNZB-02P.html")
	if err != nil {
		t.Fatalf("ImportFromURL failed: %v", err)
	}

	if def.Device.Model != "SNZB-02P" {
		t.Errorf("expected model SNZB-02P, got %s", def.Device.Model)
	}
	if _, ok := def.Device.Simulations.Telemetry["temperature"]; !ok {
		t.Errorf("expected temperature telemetry in simulations")
	}
	if _, ok := def.Device.Simulations.Telemetry["humidity"]; !ok {
		t.Errorf("expected humidity telemetry in simulations")
	}

	// Error handling: 404
	mockFetcher404 := &mockHTTPFetcher{
		statusCode: http.StatusNotFound,
		body:       "Not found",
	}
	imp404 := fixture.NewImporter(mockFetcher404)
	if _, err := imp404.ImportFromURL("https://example.com/notfound"); err == nil {
		t.Errorf("expected error on 404, got nil")
	}

	// Error handling: Network error
	mockFetcherErr := &mockHTTPFetcher{
		err: errors.New("connection reset"),
	}
	impErr := fixture.NewImporter(mockFetcherErr)
	if _, err := impErr.ImportFromURL("https://example.com/error"); err == nil {
		t.Errorf("expected error on network error, got nil")
	}
}

func TestImportFromJSON(t *testing.T) {
	jsonContent := `{
        "model": "SNZB-01P",
        "vendor": "SONOFF",
        "description": "Wireless button switch",
        "zigbeeModel": ["SNZB-01P", "WB01"],
        "supports": "action, battery, voltage"
    }`

	imp := fixture.NewImporter(nil)
	def, err := imp.ImportFromJSON([]byte(jsonContent))
	if err != nil {
		t.Fatalf("ImportFromJSON failed: %v", err)
	}

	if def.Device.Model != "SNZB-01P" {
		t.Errorf("expected SNZB-01P, got %s", def.Device.Model)
	}
	if len(def.Device.ZigbeeModels) != 2 {
		t.Errorf("expected 2 zigbee models, got %d", len(def.Device.ZigbeeModels))
	}
	if _, ok := def.Device.Simulations.Actions["single"]; !ok {
		t.Errorf("expected single action in simulations")
	}

	// Invalid JSON
	if _, err := imp.ImportFromJSON([]byte("invalid json")); err == nil {
		t.Errorf("expected error on invalid json, got nil")
	}

	// Missing model
	if _, err := imp.ImportFromJSON([]byte(`{"vendor":"SONOFF"}`)); err == nil {
		t.Errorf("expected error on missing model, got nil")
	}
}

func TestSaveDefinitionToFile(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "devices", "sonoff_snzb_01p.yaml")

	minVal, maxVal := 0.0, 100.0
	def := &fixture.DeviceDefinition{
		SchemaVersion: "1.0",
		Device: fixture.DeviceMeta{
			Model:        "SNZB-01P",
			Vendor:       "SONOFF",
			Description:  "Wireless switch",
			ZigbeeModels: []string{"SNZB-01P"},
			Endpoints: []fixture.EndpointDef{
				{Endpoint: 1, ProfileID: 0x0104, DeviceID: 0x0401},
			},
			Exposes: []fixture.ExposeDef{
				{Type: "numeric", Name: "battery", Property: "battery", Min: &minVal, Max: &maxVal},
			},
		},
	}

	if err := fixture.SaveDefinitionToFile(def, targetPath); err != nil {
		t.Fatalf("SaveDefinitionToFile failed: %v", err)
	}

	// Read file back and verify
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}

	var loaded fixture.DeviceDefinition
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal saved YAML: %v", err)
	}

	if loaded.Device.Model != def.Device.Model {
		t.Errorf("model mismatch: got %s, want %s", loaded.Device.Model, def.Device.Model)
	}

	// Nil check
	if err := fixture.SaveDefinitionToFile(nil, targetPath); err == nil {
		t.Errorf("expected error saving nil definition, got nil")
	}
}
