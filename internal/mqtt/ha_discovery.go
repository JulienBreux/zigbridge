package mqtt

import (
	"fmt"
)

// HADevice represents the device registration block inside Home Assistant discovery.
type HADevice struct {
	Identifiers  []string `json:"identifiers"`
	Name         string   `json:"name"`
	Model        string   `json:"model,omitempty"`
	Manufacturer string   `json:"manufacturer,omitempty"`
	SWVersion    string   `json:"sw_version,omitempty"`
}

// HAEntityConfig models Home Assistant MQTT Discovery configuration.
type HAEntityConfig struct {
	Component           string   `json:"-"` // e.g. "sensor", "binary_sensor", "switch", "light", "device_automation"
	UniqueID            string   `json:"unique_id,omitempty"`
	AutomationType      string   `json:"automation_type,omitempty"`
	Type                string   `json:"type,omitempty"`
	Subtype             string   `json:"subtype,omitempty"`
	Payload             string   `json:"payload,omitempty"`
	TopicStr            string   `json:"topic,omitempty"`
	Name                string   `json:"name,omitempty"`
	StateTopic          string   `json:"state_topic,omitempty"`
	CommandTopic        string   `json:"command_topic,omitempty"`
	ValueTemplate       string   `json:"value_template,omitempty"`
	DeviceClass         string   `json:"device_class,omitempty"`
	UnitOfMeasurement   string   `json:"unit_of_measurement,omitempty"`
	Icon                string   `json:"icon,omitempty"`
	PayloadOn           string   `json:"payload_on,omitempty"`
	PayloadOff          string   `json:"payload_off,omitempty"`
	Device              HADevice `json:"device"`
	AvailabilityTopic   string   `json:"availability_topic,omitempty"`
	PayloadAvailable    string   `json:"payload_available,omitempty"`
	PayloadNotAvailable string   `json:"payload_not_available,omitempty"`
}

// Topic returns the canonical Home Assistant discovery MQTT topic.
func (e *HAEntityConfig) Topic(prefix, baseTopic, ieee string) string {
	if prefix == "" {
		prefix = "homeassistant"
	}
	id := e.UniqueID
	if id == "" {
		id = "action_" + e.Subtype
	}
	return fmt.Sprintf("%s/%s/%s/%s/config", prefix, e.Component, ieee, id)
}

// NewOnOffDiscovery builds HA discovery config for an On/Off switch or plug.
func NewOnOffDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "switch",
		UniqueID:          ieee + "_switch",
		Name:              device.Name + " Switch",
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		CommandTopic:      fmt.Sprintf("%s/%s/set", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.state }}",
		PayloadOn:         "ON",
		PayloadOff:        "OFF",
		Device:            device,
		AvailabilityTopic: baseTopic + "/bridge/state",
	}
}

// NewTemperatureDiscovery builds HA discovery for a temperature sensor.
func NewTemperatureDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "sensor",
		UniqueID:          ieee + "_temperature",
		Name:              device.Name + " Temperature",
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.temperature }}",
		DeviceClass:       "temperature",
		UnitOfMeasurement: "°C",
		Device:            device,
		AvailabilityTopic: baseTopic + "/bridge/state",
	}
}

// NewHumidityDiscovery builds HA discovery for a humidity sensor.
func NewHumidityDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "sensor",
		UniqueID:          ieee + "_humidity",
		Name:              device.Name + " Humidity",
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.humidity }}",
		DeviceClass:       "humidity",
		UnitOfMeasurement: "%",
		Device:            device,
		AvailabilityTopic: baseTopic + "/bridge/state",
	}
}

// NewOccupancyDiscovery builds HA discovery for a motion / occupancy sensor.
func NewOccupancyDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "binary_sensor",
		UniqueID:          ieee + "_occupancy",
		Name:              device.Name + " Occupancy",
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.occupancy }}",
		DeviceClass:       "motion",
		PayloadOn:         "true",
		PayloadOff:        "false",
		Device:            device,
		AvailabilityTopic: baseTopic + "/bridge/state",
	}
}

// NewPowerDiscovery builds HA discovery for an electrical power meter.
func NewPowerDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "sensor",
		UniqueID:          ieee + "_power",
		Name:              device.Name + " Power",
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.power }}",
		DeviceClass:       "power",
		UnitOfMeasurement: "W",
		Device:            device,
		AvailabilityTopic: baseTopic + "/bridge/state",
	}
}

// NewEnergyDiscovery builds HA discovery for total energy delivered (kWh).
func NewEnergyDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "sensor",
		UniqueID:          ieee + "_energy",
		Name:              device.Name + " Energy",
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.energy }}",
		DeviceClass:       "energy",
		UnitOfMeasurement: "kWh",
		Device:            device,
		AvailabilityTopic: baseTopic + "/bridge/state",
	}
}

// NewCurrentDiscovery builds HA discovery for electrical current (A).
func NewCurrentDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "sensor",
		UniqueID:          ieee + "_current",
		Name:              device.Name + " Current",
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.current }}",
		DeviceClass:       "current",
		UnitOfMeasurement: "A",
		Device:            device,
		AvailabilityTopic: baseTopic + "/bridge/state",
	}
}

// NewMainsVoltageDiscovery builds HA discovery for AC mains voltage (V).
func NewMainsVoltageDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "sensor",
		UniqueID:          ieee + "_voltage",
		Name:              device.Name + " Voltage",
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.voltage }}",
		DeviceClass:       "voltage",
		UnitOfMeasurement: "V",
		Device:            device,
		AvailabilityTopic: baseTopic + "/bridge/state",
	}
}

// NewBatteryDiscovery builds HA discovery for a battery sensor.
func NewBatteryDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "sensor",
		UniqueID:          ieee + "_battery",
		Name:              device.Name + " Battery",
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.battery }}",
		DeviceClass:       "battery",
		UnitOfMeasurement: "%",
		Device:            device,
		AvailabilityTopic: baseTopic + "/bridge/state",
	}
}

// NewVoltageDiscovery builds HA discovery for a battery voltage sensor.
func NewVoltageDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "sensor",
		UniqueID:          ieee + "_voltage",
		Name:              device.Name + " Voltage",
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.voltage }}",
		DeviceClass:       "voltage",
		UnitOfMeasurement: "mV",
		Device:            device,
		AvailabilityTopic: baseTopic + "/bridge/state",
	}
}

// NewActionDiscovery builds HA discovery for a button action sensor.
func NewActionDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "sensor",
		UniqueID:          ieee + "_action",
		Name:              device.Name + " Action",
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.action }}",
		Icon:              "mdi:gesture-tap-button",
		Device:            device,
		AvailabilityTopic: baseTopic + "/bridge/state",
	}
}

// NewDeviceTriggerDiscovery builds HA discovery for a device trigger automation.
func NewDeviceTriggerDiscovery(device HADevice, ieee, baseTopic, subtype string) HAEntityConfig {
	return HAEntityConfig{
		Component:      "device_automation",
		AutomationType: "trigger",
		Type:           "action",
		Subtype:        subtype,
		Payload:        subtype,
		TopicStr:       fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:  "{{ value_json.action }}",
		Device:         device,
	}
}

// NewMoistureDiscovery builds HA discovery for a water leak sensor.
func NewMoistureDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "binary_sensor",
		UniqueID:          ieee + "_water_leak",
		Name:              device.Name + " Water Leak",
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.water_leak }}",
		DeviceClass:       "moisture",
		PayloadOn:         "true",
		PayloadOff:        "false",
		Device:            device,
		AvailabilityTopic: baseTopic + "/bridge/state",
	}
}

// NewContactDiscovery builds HA discovery for a door/window contact sensor.
// In HA: for opening/door device class, ON means open (contact: false) and OFF means closed (contact: true).
func NewContactDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "binary_sensor",
		UniqueID:          ieee + "_contact",
		Name:              device.Name + " Contact",
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.contact }}",
		DeviceClass:       "door",
		PayloadOn:         "false",
		PayloadOff:        "true",
		Device:            device,
		AvailabilityTopic: baseTopic + "/bridge/state",
	}
}

// NewTamperDiscovery builds HA discovery for a tamper sensor.
func NewTamperDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "binary_sensor",
		UniqueID:          ieee + "_tamper",
		Name:              device.Name + " Tamper",
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.tamper }}",
		DeviceClass:       "tamper",
		PayloadOn:         "true",
		PayloadOff:        "false",
		Device:            device,
		AvailabilityTopic: baseTopic + "/bridge/state",
	}
}

// NewBatteryLowDiscovery builds HA discovery for a low battery indicator.
func NewBatteryLowDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "binary_sensor",
		UniqueID:          ieee + "_battery_low",
		Name:              device.Name + " Battery Low",
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.battery_low }}",
		DeviceClass:       "battery",
		PayloadOn:         "true",
		PayloadOff:        "false",
		Device:            device,
		AvailabilityTopic: baseTopic + "/bridge/state",
	}
}

// NewSmokeDiscovery builds HA discovery for a smoke detector.
func NewSmokeDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "binary_sensor",
		UniqueID:          ieee + "_smoke",
		Name:              device.Name + " Smoke",
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.smoke }}",
		DeviceClass:       "smoke",
		PayloadOn:         "true",
		PayloadOff:        "false",
		Device:            device,
		AvailabilityTopic: baseTopic + "/bridge/state",
	}
}

// NewSirenDiscovery builds HA discovery for a warning / siren device.
func NewSirenDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "siren",
		UniqueID:          ieee + "_warning",
		Name:              device.Name + " Siren",
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		CommandTopic:      fmt.Sprintf("%s/%s/set", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.warning }}",
		Device:            device,
		AvailabilityTopic: baseTopic + "/bridge/state",
	}
}
