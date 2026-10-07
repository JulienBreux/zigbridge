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
	req.Header.Set("User-Agent", "ZigBridge-Fixture-Importer/1.0")
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
	return slug + ".yaml"
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

func extractActionValues(htmlContent string) []string {
	// 1. Prioritize Action enum section explicitly listing "The possible values are:"
	rePossible := regexp.MustCompile(`(?i)(?:id="action-enum"|id="action")[^>]*>[\s\S]*?The possible values are:\s*([\s\S]*?)</p>`)
	m := rePossible.FindStringSubmatch(htmlContent)
	var values []string
	if len(m) > 1 {
		reCode := regexp.MustCompile(`<code>([^<]+)</code>`)
		for _, cm := range reCode.FindAllStringSubmatch(m[1], -1) {
			val := strings.TrimSpace(cm[1])
			if val != "" && !slices.Contains(values, val) && !strings.Contains(val, " ") && len(val) < 40 {
				values = append(values, val)
			}
		}
	}
	if len(values) > 0 {
		return values
	}

	// 2. Fallback to Action enum header or section
	reSection := regexp.MustCompile(`(?i)(?:<h3>\s*action\b|<h[23][^>]*id="action-enum")[^<]*(?:<[^>]+>)*([\s\S]*?)(?:<h[23]|\z)`)
	m = reSection.FindStringSubmatch(htmlContent)
	if len(m) > 1 {
		sectionText := m[1]
		reCode := regexp.MustCompile(`<code>([^<]+)</code>`)
		codeMatches := reCode.FindAllStringSubmatch(sectionText, -1)
		for _, cm := range codeMatches {
			val := strings.TrimSpace(cm[1])
			if val != "" && !slices.Contains(values, val) && !strings.Contains(val, " ") && len(val) < 40 {
				values = append(values, val)
			}
		}
	}
	return values
}

func detectExposesFromHTML(htmlContent string) []ExposeDef {
	var exposes []ExposeDef
	lower := strings.ToLower(htmlContent)

	exposesRaw := extractRegex(htmlContent, `(?i)<tr>\s*<td>\s*Exposes\s*</td>\s*<td>([\s\S]*?)</td>`)
	if exposesRaw != "" {
		tagRegex := regexp.MustCompile(`<[^>]*>`)
		cleanedExposes := tagRegex.ReplaceAllString(exposesRaw, "")
		tokens := strings.Split(cleanedExposes, ",")
		rawLower := strings.ToLower(cleanedExposes)

		hasBattery := strings.Contains(rawLower, "battery")
		isMains := !hasBattery && (strings.Contains(rawLower, "power") || strings.Contains(rawLower, "energy") ||
			strings.Contains(rawLower, "current") || strings.Contains(rawLower, "plug") ||
			strings.Contains(rawLower, "urms") || strings.Contains(rawLower, "sinsts") ||
			(strings.Contains(rawLower, "switch") && !strings.Contains(rawLower, "wireless") && !strings.Contains(rawLower, "button")))

		actionValues := extractActionValues(htmlContent)

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
			case "power", "sinsts", "apparent_power":
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
			case "power_a":
				if !hasProperty(exposes, "power_a") {
					exposes = append(exposes, ExposeDef{
						Type:        "numeric",
						Name:        "power_a",
						Property:    "power_a",
						Description: "Instantaneous measured power on phase A",
						Unit:        "W",
						Access:      1,
					})
				}
			case "power_b":
				if !hasProperty(exposes, "power_b") {
					exposes = append(exposes, ExposeDef{
						Type:        "numeric",
						Name:        "power_b",
						Property:    "power_b",
						Description: "Instantaneous measured power on phase B",
						Unit:        "W",
						Access:      1,
					})
				}
			case "power_ab":
				if !hasProperty(exposes, "power_ab") {
					exposes = append(exposes, ExposeDef{
						Type:        "numeric",
						Name:        "power_ab",
						Property:    "power_ab",
						Description: "Sum of instantaneous measured power",
						Unit:        "W",
						Access:      1,
					})
				}
			case "current", "irms1":
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
			case "current_a":
				if !hasProperty(exposes, "current_a") {
					exposes = append(exposes, ExposeDef{
						Type:        "numeric",
						Name:        "current_a",
						Property:    "current_a",
						Description: "Instantaneous measured electrical current on phase A",
						Unit:        "A",
						Access:      1,
					})
				}
			case "current_b":
				if !hasProperty(exposes, "current_b") {
					exposes = append(exposes, ExposeDef{
						Type:        "numeric",
						Name:        "current_b",
						Property:    "current_b",
						Description: "Instantaneous measured electrical current on phase B",
						Unit:        "A",
						Access:      1,
					})
				}
			case "voltage", "urms1":
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
			case "energy", "east", "base":
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
			case "energy_a":
				if !hasProperty(exposes, "energy_a") {
					exposes = append(exposes, ExposeDef{
						Type:        "numeric",
						Name:        "energy_a",
						Property:    "energy_a",
						Description: "Sum of consumed energy on phase A",
						Unit:        "kWh",
						Access:      1,
					})
				}
			case "energy_b":
				if !hasProperty(exposes, "energy_b") {
					exposes = append(exposes, ExposeDef{
						Type:        "numeric",
						Name:        "energy_b",
						Property:    "energy_b",
						Description: "Sum of consumed energy on phase B",
						Unit:        "kWh",
						Access:      1,
					})
				}
			case "action":
				if !hasProperty(exposes, "action") {
					vals := actionValues
					if len(vals) == 0 {
						vals = []string{"single", "double", "long"}
					}
					exposes = append(exposes, ExposeDef{
						Type:        "enum",
						Name:        "action",
						Property:    "action",
						Description: "Triggered action (e.g. a button press)",
						Values:      vals,
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
			case "battery_low":
				if !hasProperty(exposes, "battery_low") {
					exposes = append(exposes, ExposeDef{
						Type:        "binary",
						Name:        "battery_low",
						Property:    "battery_low",
						Description: "Empty battery indicator",
						Values:      []string{"true", "false"},
						Access:      1,
					})
				}
			case "temperature", "device_temperature":
				prop := "temperature"
				if token == "device_temperature" {
					prop = "device_temperature"
				}
				if !hasProperty(exposes, prop) {
					minVal, maxVal := -20.0, 60.0
					exposes = append(exposes, ExposeDef{
						Type:        "numeric",
						Name:        prop,
						Property:    prop,
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
			case "water_leak", "water_leakage", "leak":
				if !hasProperty(exposes, "water_leak") {
					exposes = append(exposes, ExposeDef{
						Type:        "binary",
						Name:        "water_leak",
						Property:    "water_leak",
						Description: "Indicates whether the device detected a water leak",
						Values:      []string{"true", "false"},
						Access:      1,
					})
				}
			case "contact":
				if !hasProperty(exposes, "contact") {
					exposes = append(exposes, ExposeDef{
						Type:        "binary",
						Name:        "contact",
						Property:    "contact",
						Description: "Indicates whether the contact is closed (true) or open (false)",
						Values:      []string{"true", "false"},
						Access:      1,
					})
				}
			case "tamper":
				if !hasProperty(exposes, "tamper") {
					exposes = append(exposes, ExposeDef{
						Type:        "binary",
						Name:        "tamper",
						Property:    "tamper",
						Description: "Indicates whether the device is tampered",
						Values:      []string{"true", "false"},
						Access:      1,
					})
				}
			case "smoke":
				if !hasProperty(exposes, "smoke") {
					exposes = append(exposes, ExposeDef{
						Type:        "binary",
						Name:        "smoke",
						Property:    "smoke",
						Description: "Indicates whether the device detected smoke",
						Values:      []string{"true", "false"},
						Access:      1,
					})
				}
			case "warning":
				if !hasProperty(exposes, "warning") {
					exposes = append(exposes, ExposeDef{
						Type:        "composite",
						Name:        "warning",
						Property:    "warning",
						Description: "Trigger siren warning",
						Access:      2,
					})
				}
			case "squawk":
				if !hasProperty(exposes, "squawk") {
					exposes = append(exposes, ExposeDef{
						Type:        "composite",
						Name:        "squawk",
						Property:    "squawk",
						Description: "Trigger siren squawk",
						Access:      2,
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
	hasBattery := strings.Contains(lower, `battery`)
	isMains := !hasBattery && (strings.Contains(lower, `power`) || strings.Contains(lower, `energy`) ||
		strings.Contains(lower, `current`) || strings.Contains(lower, `plug`) ||
		strings.Contains(lower, `bulb`) || (strings.Contains(lower, `switch`) && !strings.Contains(lower, `wireless`) && !strings.Contains(lower, `button`)))

	// Action (button)
	hasActionHeader := strings.Contains(lower, `<h3>action`) || strings.Contains(lower, `id="action`) ||
		(strings.Contains(lower, `action (enum)`) && !strings.Contains(lower, `switch_type_button`))
	if hasActionHeader || (!isMains && strings.Contains(lower, `action`) && (strings.Contains(lower, `single`) || strings.Contains(lower, `button`))) {
		values := extractActionValues(htmlContent)
		if len(values) == 0 {
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

	// Water leak
	if strings.Contains(lower, `water_leak`) || strings.Contains(lower, `leakage`) {
		exposes = append(exposes, ExposeDef{
			Type:        "binary",
			Name:        "water_leak",
			Property:    "water_leak",
			Description: "Indicates whether the device detected a water leak",
			Values:      []string{"true", "false"},
			Access:      1,
		})
	}

	// Contact
	if strings.Contains(lower, `contact`) {
		exposes = append(exposes, ExposeDef{
			Type:        "binary",
			Name:        "contact",
			Property:    "contact",
			Description: "Indicates whether the contact is closed (true) or open (false)",
			Values:      []string{"true", "false"},
			Access:      1,
		})
	}

	// Warning / Siren
	if strings.Contains(lower, `warning`) || strings.Contains(lower, `siren`) {
		exposes = append(exposes, ExposeDef{
			Type:        "composite",
			Name:        "warning",
			Property:    "warning",
			Description: "Trigger siren warning",
			Access:      2,
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
	hasWaterLeak := hasProperty(exposes, "water_leak")
	hasContact := hasProperty(exposes, "contact")
	hasWarning := hasProperty(exposes, "warning") || hasProperty(exposes, "squawk")
	hasTamper := hasProperty(exposes, "tamper")
	hasSmoke := hasProperty(exposes, "smoke")

	hasState := hasProperty(exposes, "state")
	hasPower := hasProperty(exposes, "power") || hasProperty(exposes, "power_a")
	hasEnergy := hasProperty(exposes, "energy") || hasProperty(exposes, "energy_a")
	hasCurrent := hasProperty(exposes, "current") || hasProperty(exposes, "current_a")

	// Check if this is a keypad controller
	hasKeypadAction := false
	for _, exp := range exposes {
		if exp.Property == "action" {
			for _, v := range exp.Values {
				if v == "disarm" || v == "arm_day_zones" || v == "arm_all_zones" || v == "emergency" || v == "panic" {
					hasKeypadAction = true
					break
				}
			}
		}
	}

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

	if hasWaterLeak || hasContact || hasTamper || hasSmoke {
		inClusters = append(inClusters, 0x0500) // IASZone
		if hasWaterLeak {
			telemetry["water_leak"] = TelemetrySim{
				Cluster:   0x0500,
				Attribute: 0x0002,
			}
			actions["leak"] = ActionSim{
				Cluster:     0x0500,
				Command:     0x00,
				Payload:     []byte{0x01, 0x00},
				MQTTPayload: map[string]any{"water_leak": true},
			}
			actions["no_leak"] = ActionSim{
				Cluster:     0x0500,
				Command:     0x00,
				Payload:     []byte{0x00, 0x00},
				MQTTPayload: map[string]any{"water_leak": false},
			}
		}
		if hasContact {
			telemetry["contact"] = TelemetrySim{
				Cluster:   0x0500,
				Attribute: 0x0002,
			}
			actions["open"] = ActionSim{
				Cluster:     0x0500,
				Command:     0x00,
				Payload:     []byte{0x01, 0x00},
				MQTTPayload: map[string]any{"contact": false},
			}
			actions["closed"] = ActionSim{
				Cluster:     0x0500,
				Command:     0x00,
				Payload:     []byte{0x00, 0x00},
				MQTTPayload: map[string]any{"contact": true},
			}
		}
	}

	if hasWarning {
		inClusters = append(inClusters, 0x0500, 0x0502) // IASZone + IASWD
		actions["warning"] = ActionSim{
			Cluster:     0x0502,
			Command:     0x00,
			MQTTPayload: map[string]any{"warning": map[string]any{"mode": "burglar", "level": "very_high"}},
		}
		actions["squawk"] = ActionSim{
			Cluster:     0x0502,
			Command:     0x01,
			MQTTPayload: map[string]any{"squawk": map[string]any{"state": "system_is_armed"}},
		}
	}

	if hasKeypadAction {
		inClusters = append(inClusters, 0x0500, 0x0501)
		outClusters = append(outClusters, 0x0501)
		actions["disarm"] = ActionSim{
			Cluster:     0x0501,
			Command:     0x00,
			Payload:     []byte{0x00},
			MQTTPayload: map[string]any{"action": "disarm"},
		}
		actions["arm_day_zones"] = ActionSim{
			Cluster:     0x0501,
			Command:     0x00,
			Payload:     []byte{0x01},
			MQTTPayload: map[string]any{"action": "arm_day_zones"},
		}
		actions["arm_night_zones"] = ActionSim{
			Cluster:     0x0501,
			Command:     0x00,
			Payload:     []byte{0x02},
			MQTTPayload: map[string]any{"action": "arm_night_zones"},
		}
		actions["arm_all_zones"] = ActionSim{
			Cluster:     0x0501,
			Command:     0x00,
			Payload:     []byte{0x03},
			MQTTPayload: map[string]any{"action": "arm_all_zones"},
		}
		actions["panic"] = ActionSim{
			Cluster:     0x0501,
			Command:     0x04,
			MQTTPayload: map[string]any{"action": "panic"},
		}
		actions["emergency"] = ActionSim{
			Cluster:     0x0501,
			Command:     0x02,
			MQTTPayload: map[string]any{"action": "emergency"},
		}
	}

	for _, exp := range exposes {
		switch exp.Property {
		case "action":
			isButton = true
			outClusters = append(outClusters, 0x0006) // OnOff output

			if !hasKeypadAction {
				hasStyrbar := slices.Contains(exp.Values, "arrow_left_click") || slices.Contains(exp.Values, "brightness_move_up")
				hasSomrig := slices.Contains(exp.Values, "1_initial_press") || slices.Contains(exp.Values, "2_initial_press")

				if hasStyrbar {
					outClusters = append(outClusters, 0x0008, 0x0005) // LevelControl, Scenes
					actions["on"] = ActionSim{Cluster: 0x0006, Command: 0x01, MQTTPayload: map[string]any{"action": "on"}}
					actions["off"] = ActionSim{Cluster: 0x0006, Command: 0x00, MQTTPayload: map[string]any{"action": "off"}}
					actions["brightness_move_up"] = ActionSim{Cluster: 0x0008, Command: 0x01, Payload: []byte{0x00}, MQTTPayload: map[string]any{"action": "brightness_move_up"}}
					actions["brightness_move_down"] = ActionSim{Cluster: 0x0008, Command: 0x01, Payload: []byte{0x01}, MQTTPayload: map[string]any{"action": "brightness_move_down"}}
					actions["brightness_stop"] = ActionSim{Cluster: 0x0008, Command: 0x03, MQTTPayload: map[string]any{"action": "brightness_stop"}}
					actions["arrow_left_click"] = ActionSim{Cluster: 0x0005, Command: 0x07, Payload: []byte{0x01}, MQTTPayload: map[string]any{"action": "arrow_left_click"}}
					actions["arrow_right_click"] = ActionSim{Cluster: 0x0005, Command: 0x07, Payload: []byte{0x00}, MQTTPayload: map[string]any{"action": "arrow_right_click"}}
					actions["arrow_left_hold"] = ActionSim{Cluster: 0x0005, Command: 0x08, Payload: []byte{0x01}, MQTTPayload: map[string]any{"action": "arrow_left_hold"}}
					actions["arrow_right_hold"] = ActionSim{Cluster: 0x0005, Command: 0x08, Payload: []byte{0x00}, MQTTPayload: map[string]any{"action": "arrow_right_hold"}}
					actions["arrow_left_release"] = ActionSim{Cluster: 0x0005, Command: 0x09, Payload: []byte{0x01}, MQTTPayload: map[string]any{"action": "arrow_left_release"}}
					actions["arrow_right_release"] = ActionSim{Cluster: 0x0005, Command: 0x09, Payload: []byte{0x00}, MQTTPayload: map[string]any{"action": "arrow_right_release"}}
				} else if hasSomrig {
					outClusters = append(outClusters, 0x0008)
					for _, btn := range []string{"1", "2"} {
						bByte := byte(0x01)
						if btn == "2" {
							bByte = 0x02
						}
						actions[btn+"_initial_press"] = ActionSim{Cluster: 0x0006, Command: 0x02, Payload: []byte{bByte}, MQTTPayload: map[string]any{"action": btn + "_initial_press"}}
						actions[btn+"_long_press"] = ActionSim{Cluster: 0x0008, Command: 0x01, Payload: []byte{bByte}, MQTTPayload: map[string]any{"action": btn + "_long_press"}}
						actions[btn+"_short_release"] = ActionSim{Cluster: 0x0006, Command: 0x00, Payload: []byte{bByte}, MQTTPayload: map[string]any{"action": btn + "_short_release"}}
						actions[btn+"_long_release"] = ActionSim{Cluster: 0x0008, Command: 0x03, Payload: []byte{bByte}, MQTTPayload: map[string]any{"action": btn + "_long_release"}}
						actions[btn+"_double_press"] = ActionSim{Cluster: 0x0006, Command: 0x01, Payload: []byte{bByte}, MQTTPayload: map[string]any{"action": btn + "_double_press"}}
					}
				} else {
					actions["single"] = ActionSim{Cluster: 0x0006, Command: 0x02, MQTTPayload: map[string]any{"action": "single"}}
					actions["double"] = ActionSim{Cluster: 0x0006, Command: 0x01, MQTTPayload: map[string]any{"action": "double"}}
					if slices.Contains(exp.Values, "hold") {
						actions["hold"] = ActionSim{Cluster: 0x0006, Command: 0x00, MQTTPayload: map[string]any{"action": "hold"}}
					} else {
						actions["long"] = ActionSim{Cluster: 0x0006, Command: 0x00, MQTTPayload: map[string]any{"action": "long"}}
					}
				}
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

		case "temperature", "device_temperature":
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
	if hasWarning {
		deviceID = 0x0403 // IAS Warning Device
	} else if hasWaterLeak || hasContact {
		deviceID = 0x0402 // IAS Zone sensor
	} else if isButton || hasKeypadAction {
		deviceID = 0x0401 // Non-color controller / Keypad
	} else if isMetering {
		deviceID = 0x0051 // Smart Plug / Meter
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
