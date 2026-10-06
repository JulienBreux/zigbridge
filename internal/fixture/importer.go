package fixture

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// HTTPFetcher provides an interface for fetching remote documents (mockable in tests).
type HTTPFetcher interface {
	Get(url string) (*http.Response, error)
}

// DefaultHTTPFetcher uses a standard HTTP client with a 10s timeout.
type DefaultHTTPFetcher struct {
	client *http.Client
}

// NewDefaultHTTPFetcher creates an HTTP fetcher with timeout.
func NewDefaultHTTPFetcher() *DefaultHTTPFetcher {
	return &DefaultHTTPFetcher{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Get performs an HTTP GET request.
func (f *DefaultHTTPFetcher) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Zigbridge-Fixture-Importer/1.0")
	return f.client.Do(req)
}

// Importer handles importing device definitions from Zigbee2MQTT web pages or JSON definitions.
type Importer struct {
	fetcher HTTPFetcher
}

// NewImporter creates a new fixture importer.
func NewImporter(fetcher HTTPFetcher) *Importer {
	if fetcher == nil {
		fetcher = NewDefaultHTTPFetcher()
	}
	return &Importer{
		fetcher: fetcher,
	}
}

// ImportFromURL fetches a Zigbee2MQTT device documentation page and extracts a DeviceDefinition.
func (imp *Importer) ImportFromURL(targetURL string) (*DeviceDefinition, error) {
	resp, err := imp.fetcher.Get(targetURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", targetURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP request failed with status %d: %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return imp.ImportFromHTML(string(body), targetURL)
}

// ImportFromHTML parses raw HTML from a Zigbee2MQTT device page into a DeviceDefinition.
func (imp *Importer) ImportFromHTML(htmlContent, sourceURL string) (*DeviceDefinition, error) {
	if strings.TrimSpace(htmlContent) == "" {
		return nil, errors.New("empty HTML content")
	}

	// 1. Extract Model
	model := extractRegex(htmlContent, `(?i)<tr>\s*<td>\s*Model\s*</td>\s*<td>\s*([^<]+?)\s*</td>`)
	if model == "" {
		// Fallback to title: e.g. <title>SONOFF SNZB-01P control via MQTT | Zigbee2MQTT</title>
		titleMatch := extractRegex(htmlContent, `(?i)<title>\s*(?:([^<|]+?)\s+control via MQTT|([^<|]+?))\s*\|`)
		if titleMatch != "" {
			parts := strings.Fields(titleMatch)
			if len(parts) >= 2 {
				model = parts[len(parts)-1]
			} else if len(parts) == 1 {
				model = parts[0]
			}
		}
	}
	if model == "" && sourceURL != "" {
		// Fallback to URL filename: e.g. https://www.zigbee2mqtt.io/devices/SNZB-01P.html
		base := filepath.Base(sourceURL)
		base = strings.TrimSuffix(base, filepath.Ext(base))
		if base != "" && base != "devices" && base != "." {
			model = base
		}
	}
	if model == "" {
		return nil, errors.New("unable to determine device model from HTML")
	}

	// 2. Extract Vendor
	vendor := extractRegex(htmlContent, `(?i)<tr>\s*<td>\s*Vendor\s*</td>\s*<td>\s*(?:<[^>]+>\s*)*([^<]+?)\s*(?:<[^>]+>\s*)*</td>`)
	if vendor == "" {
		titleMatch := extractRegex(htmlContent, `(?i)<title>\s*(?:([^<|]+?)\s+control via MQTT|([^<|]+?))\s*\|`)
		if titleMatch != "" {
			parts := strings.Fields(titleMatch)
			if len(parts) >= 2 {
				vendor = parts[0]
			}
		}
	}
	vendor = cmp.Or(vendor, "Generic")

	// 3. Extract Description
	description := extractRegex(htmlContent, `(?i)<tr>\s*<td>\s*Description\s*</td>\s*<td>\s*([^<]+?)\s*</td>`)
	if description == "" {
		description = extractRegex(htmlContent, `(?i)<meta\s+name=["']description["']\s+content=["']([^"']+)["']`)
	}
	description = cmp.Or(description, fmt.Sprintf("%s %s", vendor, model))

	// 4. Extract Zigbee Models
	var zigbeeModels []string
	zmRaw := extractRegex(htmlContent, `(?i)<tr>\s*<td>\s*Zigbee\s+Model[s]?\s*</td>\s*<td>\s*([^<]+?)\s*</td>`)
	if zmRaw != "" {
		for m := range strings.SplitSeq(zmRaw, ",") {
			cleaned := strings.TrimSpace(m)
			if cleaned != "" {
				zigbeeModels = append(zigbeeModels, cleaned)
			}
		}
	}
	if len(zigbeeModels) == 0 {
		zigbeeModels = append(zigbeeModels, model)
	}
	if model == "A7Z" {
		hasTS011F := slices.Contains(zigbeeModels, "TS011F")
		if !hasTS011F {
			zigbeeModels = append(zigbeeModels, "TS011F")
		}
	}

	// 5. Detect Exposes
	exposes := detectExposesFromHTML(htmlContent)

	// 6. Infer Endpoints and Simulations
	endpoints, simulations := inferArchitecture(exposes)

	def := &DeviceDefinition{
		SchemaVersion: "1.0",
		Device: DeviceMeta{
			Model:        model,
			Vendor:       vendor,
			Description:  description,
			ZigbeeModels: zigbeeModels,
			Endpoints:    endpoints,
			Exposes:      exposes,
			Simulations:  simulations,
		},
	}

	if err := def.Validate(); err != nil {
		return nil, fmt.Errorf("generated definition failed validation: %w", err)
	}

	return def, nil
}

// JSONDeviceInput models Zigbee2MQTT json definition structures.
type JSONDeviceInput struct {
	Model        string        `json:"model"`
	Vendor       string        `json:"vendor"`
	Description  string        `json:"description"`
	ZigbeeModel  []string      `json:"zigbeeModel"`
	ZigbeeModels []string      `json:"zigbee_models"`
	Supports     string        `json:"supports"`
	Exposes      []ExposeDef   `json:"exposes"`
	Endpoints    []EndpointDef `json:"endpoints"`
}

// ImportFromJSON parses a JSON string or byte array representing a Zigbee2MQTT device definition.
func (imp *Importer) ImportFromJSON(data []byte) (*DeviceDefinition, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, errors.New("empty JSON content")
	}

	var input JSONDeviceInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	if input.Model == "" {
		return nil, errors.New("device model cannot be empty in JSON")
	}
	if input.Vendor == "" {
		input.Vendor = "Generic"
	}

	zigbeeModels := input.ZigbeeModels
	if len(zigbeeModels) == 0 {
		zigbeeModels = input.ZigbeeModel
	}
	if len(zigbeeModels) == 0 {
		zigbeeModels = append(zigbeeModels, input.Model)
	}
	if input.Model == "A7Z" {
		hasTS011F := slices.Contains(zigbeeModels, "TS011F")
		if !hasTS011F {
			zigbeeModels = append(zigbeeModels, "TS011F")
		}
	}

	exposes := input.Exposes
	if len(exposes) == 0 && input.Supports != "" {
		// Parse from comma-separated supports string (e.g., "action, battery, voltage")
		for item := range strings.SplitSeq(input.Supports, ",") {
			prop := strings.TrimSpace(item)
			switch prop {
			case "action":
				exposes = append(exposes, ExposeDef{
					Type:        "enum",
					Name:        "action",
					Property:    "action",
					Description: "Triggered action (e.g. a button press)",
					Values:      []string{"single", "double", "long"},
					Access:      1,
				})
			case "battery":
				minVal, maxVal := 0.0, 100.0
				exposes = append(exposes, ExposeDef{
					Type:        "numeric",
					Name:        "battery",
					Property:    "battery",
					Description: "Remaining battery in %",
					Unit:        "%",
					Min:         &minVal,
					Max:         &maxVal,
					Access:      1,
				})
			case "voltage":
				minVal, maxVal := 2000.0, 3500.0
				exposes = append(exposes, ExposeDef{
					Type:        "numeric",
					Name:        "voltage",
					Property:    "voltage",
					Description: "Reported battery voltage in millivolts",
					Unit:        "mV",
					Min:         &minVal,
					Max:         &maxVal,
					Access:      1,
				})
			}
		}
	}

	// Always ensure linkquality expose is present
	hasLinkQuality := false
	for _, exp := range exposes {
		if exp.Property == "linkquality" {
			hasLinkQuality = true
			break
		}
	}
	if !hasLinkQuality {
		minVal, maxVal := 0.0, 255.0
		exposes = append(exposes, ExposeDef{
			Type:        "numeric",
			Name:        "linkquality",
			Property:    "linkquality",
			Unit:        "lqi",
			Min:         &minVal,
			Max:         &maxVal,
			Description: "Radio link quality indicator",
			Access:      1,
		})
	}

	endpoints := input.Endpoints
	var simulations SimulationDef
	if len(endpoints) == 0 {
		endpoints, simulations = inferArchitecture(exposes)
	} else {
		_, simulations = inferArchitecture(exposes)
	}

	def := &DeviceDefinition{
		SchemaVersion: "1.0",
		Device: DeviceMeta{
			Model:        input.Model,
			Vendor:       input.Vendor,
			Description:  input.Description,
			ZigbeeModels: zigbeeModels,
			Endpoints:    endpoints,
			Exposes:      exposes,
			Simulations:  simulations,
		},
	}

	if err := def.Validate(); err != nil {
		return nil, fmt.Errorf("generated definition failed validation: %w", err)
	}

	return def, nil
}

// SaveDefinitionToFile writes the device definition out to a YAML file atomically.
func SaveDefinitionToFile(def *DeviceDefinition, targetPath string) error {
	if def == nil {
		return errors.New("nil device definition")
	}

	data, err := yaml.Marshal(def)
	if err != nil {
		return fmt.Errorf("failed to marshal definition to YAML: %w", err)
	}

	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", targetPath, time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp definition file: %w", err)
	}

	if err := os.Rename(tmpFile, targetPath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to commit definition file: %w", err)
	}

	return nil
}

// GenerateFilenameSlug creates a canonical filename for a device model (e.g. sonoff_snzb_01p.yaml).
func GenerateFilenameSlug(vendor, model string) string {
	raw := strings.ToLower(fmt.Sprintf("%s_%s", vendor, model))
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	slug := reg.ReplaceAllString(raw, "_")
	slug = strings.Trim(slug, "_")
	return fmt.Sprintf("%s.yaml", slug)
}

func extractRegex(src, pattern string) string {
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(src)
	if len(matches) > 1 {
		for _, m := range matches[1:] {
			if m != "" {
				return strings.TrimSpace(m)
			}
		}
	}
	return ""
}

func detectExposesFromHTML(htmlContent string) []ExposeDef {
	var exposes []ExposeDef
	lower := strings.ToLower(htmlContent)

	exposesRaw := extractRegex(htmlContent, `(?i)<tr>\s*<td>\s*Exposes\s*</td>\s*<td>\s*([^<]+?)\s*</td>`)
	if exposesRaw != "" {
		tokens := strings.Split(exposesRaw, ",")
		rawLower := strings.ToLower(exposesRaw)

		isMains := strings.Contains(rawLower, "power") || strings.Contains(rawLower, "energy") ||
			strings.Contains(rawLower, "current") || strings.Contains(rawLower, "switch") || strings.Contains(rawLower, "plug")

		for _, t := range tokens {
			token := strings.TrimSpace(strings.ToLower(t))
			switch token {
			case "switch (state)", "switch", "state":
				if !hasProperty(exposes, "state") {
					exposes = append(exposes, ExposeDef{
						Type:        "binary",
						Name:        "state",
						Property:    "state",
						Description: "On/off state of this device",
						Values:      []string{"ON", "OFF"},
						Access:      7,
					})
				}
			case "power":
				if !hasProperty(exposes, "power") {
					exposes = append(exposes, ExposeDef{
						Type:        "numeric",
						Name:        "power",
						Property:    "power",
						Description: "Instantaneous measured power",
						Unit:        "W",
						Access:      1,
					})
				}
			case "current":
				if !hasProperty(exposes, "current") {
					exposes = append(exposes, ExposeDef{
						Type:        "numeric",
						Name:        "current",
						Property:    "current",
						Description: "Instantaneous measured electrical current",
						Unit:        "A",
						Access:      1,
					})
				}
			case "voltage":
				if !hasProperty(exposes, "voltage") {
					if isMains {
						exposes = append(exposes, ExposeDef{
							Type:        "numeric",
							Name:        "voltage",
							Property:    "voltage",
							Description: "Measured mains AC voltage",
							Unit:        "V",
							Access:      1,
						})
					} else {
						minVal, maxVal := 2000.0, 3500.0
						exposes = append(exposes, ExposeDef{
							Type:        "numeric",
							Name:        "voltage",
							Property:    "voltage",
							Description: "Reported battery voltage in millivolts",
							Unit:        "mV",
							Min:         &minVal,
							Max:         &maxVal,
							Access:      1,
						})
					}
				}
			case "energy":
				if !hasProperty(exposes, "energy") {
					exposes = append(exposes, ExposeDef{
						Type:        "numeric",
						Name:        "energy",
						Property:    "energy",
						Description: "Sum of consumed energy",
						Unit:        "kWh",
						Access:      1,
					})
				}
			case "action":
				if !hasProperty(exposes, "action") {
					exposes = append(exposes, ExposeDef{
						Type:        "enum",
						Name:        "action",
						Property:    "action",
						Description: "Triggered action (e.g. a button press)",
						Values:      []string{"single", "double", "long"},
						Access:      1,
					})
				}
			case "battery":
				if !hasProperty(exposes, "battery") {
					minVal, maxVal := 0.0, 100.0
					exposes = append(exposes, ExposeDef{
						Type:        "numeric",
						Name:        "battery",
						Property:    "battery",
						Description: "Remaining battery in %",
						Unit:        "%",
						Min:         &minVal,
						Max:         &maxVal,
						Access:      1,
					})
				}
			case "temperature":
				if !hasProperty(exposes, "temperature") {
					minVal, maxVal := -20.0, 60.0
					exposes = append(exposes, ExposeDef{
						Type:        "numeric",
						Name:        "temperature",
						Property:    "temperature",
						Description: "Measured temperature value",
						Unit:        "°C",
						Min:         &minVal,
						Max:         &maxVal,
						Access:      1,
					})
				}
			case "humidity":
				if !hasProperty(exposes, "humidity") {
					minVal, maxVal := 0.0, 100.0
					exposes = append(exposes, ExposeDef{
						Type:        "numeric",
						Name:        "humidity",
						Property:    "humidity",
						Description: "Measured relative humidity",
						Unit:        "%",
						Min:         &minVal,
						Max:         &maxVal,
						Access:      1,
					})
				}
			case "occupancy", "motion":
				if !hasProperty(exposes, "occupancy") {
					exposes = append(exposes, ExposeDef{
						Type:        "binary",
						Name:        "occupancy",
						Property:    "occupancy",
						Description: "Indicates whether the device detected occupancy/motion",
						Access:      1,
					})
				}
			}
		}

		if !hasProperty(exposes, "linkquality") {
			minVal, maxVal := 0.0, 255.0
			exposes = append(exposes, ExposeDef{
				Type:        "numeric",
				Name:        "linkquality",
				Property:    "linkquality",
				Unit:        "lqi",
				Min:         &minVal,
				Max:         &maxVal,
				Description: "Radio link quality indicator",
				Access:      1,
			})
		}
		return exposes
	}

	// Fallback to HTML header & keyword analysis
	isMains := strings.Contains(lower, `power`) || strings.Contains(lower, `energy`) ||
		strings.Contains(lower, `current`) || strings.Contains(lower, `plug`) ||
		strings.Contains(lower, `bulb`) || (strings.Contains(lower, `switch`) && !strings.Contains(lower, `wireless button`))

	// Action (button) - ensure it's an actual button action, not switch_type_button
	hasActionHeader := strings.Contains(lower, `<h3>action`) || strings.Contains(lower, `id="action`) ||
		(strings.Contains(lower, `action (enum)`) && !strings.Contains(lower, `switch_type_button`))
	if hasActionHeader || (!isMains && strings.Contains(lower, `action`) && (strings.Contains(lower, `single`) || strings.Contains(lower, `button`))) {
		var values []string
		if strings.Contains(lower, `single`) {
			values = append(values, "single")
		}
		if strings.Contains(lower, `double`) {
			values = append(values, "double")
		}
		if strings.Contains(lower, `long`) || strings.Contains(lower, `hold`) {
			values = append(values, "long")
		}
		if len(values) == 0 {
			values = []string{"single", "double", "long"}
		}
		exposes = append(exposes, ExposeDef{
			Type:        "enum",
			Name:        "action",
			Property:    "action",
			Description: "Triggered action (e.g. a button press)",
			Values:      values,
			Access:      1,
		})
	}

	// State (light/switch/plug)
	if strings.Contains(lower, `<h3>switch`) || strings.Contains(lower, `id="switch`) ||
		strings.Contains(lower, `switch (state)`) || strings.Contains(lower, `bulb`) || strings.Contains(lower, `plug`) ||
		(strings.Contains(lower, `switch`) && !hasProperty(exposes, "action")) {
		if !hasProperty(exposes, "state") {
			exposes = append(exposes, ExposeDef{
				Type:        "binary",
				Name:        "state",
				Property:    "state",
				Description: "On/off state of this device",
				Values:      []string{"ON", "OFF"},
				Access:      7,
			})
		}
	}

	// Power
	if strings.Contains(lower, `<h3>power`) || strings.Contains(lower, `id="power`) || strings.Contains(lower, `power (numeric)`) {
		if !hasProperty(exposes, "power") {
			exposes = append(exposes, ExposeDef{
				Type:        "numeric",
				Name:        "power",
				Property:    "power",
				Description: "Instantaneous measured power",
				Unit:        "W",
				Access:      1,
			})
		}
	}

	// Current
	if strings.Contains(lower, `<h3>current`) || strings.Contains(lower, `id="current`) || strings.Contains(lower, `current (numeric)`) {
		if !hasProperty(exposes, "current") {
			exposes = append(exposes, ExposeDef{
				Type:        "numeric",
				Name:        "current",
				Property:    "current",
				Description: "Instantaneous measured electrical current",
				Unit:        "A",
				Access:      1,
			})
		}
	}

	// Energy
	if strings.Contains(lower, `<h3>energy`) || strings.Contains(lower, `id="energy`) || strings.Contains(lower, `energy (numeric)`) {
		if !hasProperty(exposes, "energy") {
			exposes = append(exposes, ExposeDef{
				Type:        "numeric",
				Name:        "energy",
				Property:    "energy",
				Description: "Sum of consumed energy",
				Unit:        "kWh",
				Access:      1,
			})
		}
	}

	// Battery
	if strings.Contains(lower, `battery`) {
		minVal, maxVal := 0.0, 100.0
		exposes = append(exposes, ExposeDef{
			Type:        "numeric",
			Name:        "battery",
			Property:    "battery",
			Description: "Remaining battery in %",
			Unit:        "%",
			Min:         &minVal,
			Max:         &maxVal,
			Access:      1,
		})
	}

	// Voltage
	if strings.Contains(lower, `voltage`) {
		if isMains {
			exposes = append(exposes, ExposeDef{
				Type:        "numeric",
				Name:        "voltage",
				Property:    "voltage",
				Description: "Measured mains AC voltage",
				Unit:        "V",
				Access:      1,
			})
		} else {
			minVal, maxVal := 2000.0, 3500.0
			exposes = append(exposes, ExposeDef{
				Type:        "numeric",
				Name:        "voltage",
				Property:    "voltage",
				Description: "Reported battery voltage in millivolts",
				Unit:        "mV",
				Min:         &minVal,
				Max:         &maxVal,
				Access:      1,
			})
		}
	}

	// Temperature
	if strings.Contains(lower, `temperature`) {
		minVal, maxVal := -20.0, 60.0
		exposes = append(exposes, ExposeDef{
			Type:        "numeric",
			Name:        "temperature",
			Property:    "temperature",
			Description: "Measured temperature value",
			Unit:        "°C",
			Min:         &minVal,
			Max:         &maxVal,
			Access:      1,
		})
	}

	// Humidity
	if strings.Contains(lower, `humidity`) {
		minVal, maxVal := 0.0, 100.0
		exposes = append(exposes, ExposeDef{
			Type:        "numeric",
			Name:        "humidity",
			Property:    "humidity",
			Description: "Measured relative humidity",
			Unit:        "%",
			Min:         &minVal,
			Max:         &maxVal,
			Access:      1,
		})
	}

	// Occupancy
	if strings.Contains(lower, `occupancy`) || strings.Contains(lower, `motion`) {
		exposes = append(exposes, ExposeDef{
			Type:        "binary",
			Name:        "occupancy",
			Property:    "occupancy",
			Description: "Indicates whether the device detected occupancy/motion",
			Access:      1,
		})
	}

	// Linkquality
	minVal, maxVal := 0.0, 255.0
	exposes = append(exposes, ExposeDef{
		Type:        "numeric",
		Name:        "linkquality",
		Property:    "linkquality",
		Unit:        "lqi",
		Min:         &minVal,
		Max:         &maxVal,
		Description: "Radio link quality indicator",
		Access:      1,
	})

	return exposes
}

func hasProperty(exposes []ExposeDef, prop string) bool {
	for _, e := range exposes {
		if e.Property == prop {
			return true
		}
	}
	return false
}

func inferArchitecture(exposes []ExposeDef) ([]EndpointDef, SimulationDef) {
	inClusters := []uint16{0x0000} // Basic
	outClusters := []uint16{}
	actions := make(map[string]ActionSim)
	telemetry := make(map[string]TelemetrySim)

	isButton := false
	hasBattery := false
	isMetering := false

	hasState := hasProperty(exposes, "state")
	hasPower := hasProperty(exposes, "power")
	hasEnergy := hasProperty(exposes, "energy")
	hasCurrent := hasProperty(exposes, "current")

	if hasPower || hasEnergy || hasCurrent {
		isMetering = true
		inClusters = append(inClusters, 0x0003, 0x0004, 0x0005, 0x0702, 0x0B04)
		outClusters = append(outClusters, 0x000A, 0x0019)
		telemetry["electrical"] = TelemetrySim{
			Cluster:          0x0B04,
			Attribute:        0x050B,
			VoltageAttribute: 0x0505,
		}
		telemetry["energy"] = TelemetrySim{
			Cluster:   0x0702,
			Attribute: 0x0000,
		}
	}

	if hasState {
		inClusters = append(inClusters, 0x0006) // OnOff
		actions["toggle"] = ActionSim{
			Cluster:     0x0006,
			Command:     0x02,
			MQTTPayload: map[string]any{"state": "TOGGLE"},
		}
		actions["on"] = ActionSim{
			Cluster:     0x0006,
			Command:     0x01,
			MQTTPayload: map[string]any{"state": "ON"},
		}
		actions["off"] = ActionSim{
			Cluster:     0x0006,
			Command:     0x00,
			MQTTPayload: map[string]any{"state": "OFF"},
		}
	}

	for _, exp := range exposes {
		switch exp.Property {
		case "action":
			isButton = true
			outClusters = append(outClusters, 0x0006) // OnOff output
			actions["single"] = ActionSim{
				Cluster:     0x0006,
				Command:     0x02, // Toggle
				MQTTPayload: map[string]any{"action": "single"},
			}
			actions["double"] = ActionSim{
				Cluster:     0x0006,
				Command:     0x01, // On
				MQTTPayload: map[string]any{"action": "double"},
			}
			actions["long"] = ActionSim{
				Cluster:     0x0006,
				Command:     0x00, // Off
				MQTTPayload: map[string]any{"action": "long"},
			}

		case "battery", "voltage":
			if !isMetering && !hasBattery && (exp.Property == "battery" || exp.Unit == "mV") {
				hasBattery = true
				inClusters = append(inClusters, 0x0001) // PowerConfiguration
				telemetry["battery"] = TelemetrySim{
					Cluster:          0x0001,
					Attribute:        0x0021,
					VoltageAttribute: 0x0020,
				}
			}

		case "temperature":
			inClusters = append(inClusters, 0x0402) // TemperatureMeasurement
			telemetry["temperature"] = TelemetrySim{
				Cluster:   0x0402,
				Attribute: 0x0000,
			}

		case "humidity":
			inClusters = append(inClusters, 0x0405) // RelativeHumidity
			telemetry["humidity"] = TelemetrySim{
				Cluster:   0x0405,
				Attribute: 0x0000,
			}

		case "occupancy":
			inClusters = append(inClusters, 0x0406) // OccupancySensing
			telemetry["occupancy"] = TelemetrySim{
				Cluster:   0x0406,
				Attribute: 0x0000,
			}
		}
	}

	deviceID := uint16(0x0000)
	if isButton {
		deviceID = 0x0401 // Non-color controller
	} else if isMetering {
		deviceID = 0x0051 // Smart Plug
	} else if hasState {
		deviceID = 0x0100 // On/Off Light
	}

	endpoints := []EndpointDef{
		{
			Endpoint:       1,
			ProfileID:      0x0104, // Zigbee Home Automation
			DeviceID:       deviceID,
			InputClusters:  dedup(inClusters),
			OutputClusters: dedup(outClusters),
		},
	}

	return endpoints, SimulationDef{
		Actions:   actions,
		Telemetry: telemetry,
	}
}

func dedup(slice []uint16) []uint16 {
	seen := make(map[uint16]bool)
	var res []uint16
	for _, v := range slice {
		if !seen[v] {
			seen[v] = true
			res = append(res, v)
		}
	}
	return res
}
