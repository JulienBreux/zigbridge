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
		id = fmt.Sprintf("action_%s", e.Subtype)
	}
	return fmt.Sprintf("%s/%s/%s/%s/config", prefix, e.Component, ieee, id)
}

// NewOnOffDiscovery builds HA discovery config for an On/Off switch or plug.
func NewOnOffDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "switch",
		UniqueID:          fmt.Sprintf("%s_switch", ieee),
		Name:              fmt.Sprintf("%s Switch", device.Name),
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		CommandTopic:      fmt.Sprintf("%s/%s/set", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.state }}",
		PayloadOn:         "ON",
		PayloadOff:        "OFF",
		Device:            device,
		AvailabilityTopic: fmt.Sprintf("%s/bridge/state", baseTopic),
	}
}

// NewTemperatureDiscovery builds HA discovery for a temperature sensor.
func NewTemperatureDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "sensor",
		UniqueID:          fmt.Sprintf("%s_temperature", ieee),
		Name:              fmt.Sprintf("%s Temperature", device.Name),
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.temperature }}",
		DeviceClass:       "temperature",
		UnitOfMeasurement: "°C",
		Device:            device,
		AvailabilityTopic: fmt.Sprintf("%s/bridge/state", baseTopic),
	}
}

// NewHumidityDiscovery builds HA discovery for a humidity sensor.
func NewHumidityDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "sensor",
		UniqueID:          fmt.Sprintf("%s_humidity", ieee),
		Name:              fmt.Sprintf("%s Humidity", device.Name),
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.humidity }}",
		DeviceClass:       "humidity",
		UnitOfMeasurement: "%",
		Device:            device,
		AvailabilityTopic: fmt.Sprintf("%s/bridge/state", baseTopic),
	}
}

// NewOccupancyDiscovery builds HA discovery for a motion / occupancy sensor.
func NewOccupancyDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "binary_sensor",
		UniqueID:          fmt.Sprintf("%s_occupancy", ieee),
		Name:              fmt.Sprintf("%s Occupancy", device.Name),
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.occupancy }}",
		DeviceClass:       "motion",
		PayloadOn:         "true",
		PayloadOff:        "false",
		Device:            device,
		AvailabilityTopic: fmt.Sprintf("%s/bridge/state", baseTopic),
	}
}

// NewPowerDiscovery builds HA discovery for an electrical power meter.
func NewPowerDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "sensor",
		UniqueID:          fmt.Sprintf("%s_power", ieee),
		Name:              fmt.Sprintf("%s Power", device.Name),
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.power }}",
		DeviceClass:       "power",
		UnitOfMeasurement: "W",
		Device:            device,
		AvailabilityTopic: fmt.Sprintf("%s/bridge/state", baseTopic),
	}
}

// NewBatteryDiscovery builds HA discovery for a battery sensor.
func NewBatteryDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "sensor",
		UniqueID:          fmt.Sprintf("%s_battery", ieee),
		Name:              fmt.Sprintf("%s Battery", device.Name),
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.battery }}",
		DeviceClass:       "battery",
		UnitOfMeasurement: "%",
		Device:            device,
		AvailabilityTopic: fmt.Sprintf("%s/bridge/state", baseTopic),
	}
}

// NewVoltageDiscovery builds HA discovery for a battery voltage sensor.
func NewVoltageDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "sensor",
		UniqueID:          fmt.Sprintf("%s_voltage", ieee),
		Name:              fmt.Sprintf("%s Voltage", device.Name),
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.voltage }}",
		DeviceClass:       "voltage",
		UnitOfMeasurement: "mV",
		Device:            device,
		AvailabilityTopic: fmt.Sprintf("%s/bridge/state", baseTopic),
	}
}

// NewActionDiscovery builds HA discovery for a button action sensor.
func NewActionDiscovery(device HADevice, ieee, baseTopic string) HAEntityConfig {
	return HAEntityConfig{
		Component:         "sensor",
		UniqueID:          fmt.Sprintf("%s_action", ieee),
		Name:              fmt.Sprintf("%s Action", device.Name),
		StateTopic:        fmt.Sprintf("%s/%s", baseTopic, ieee),
		ValueTemplate:     "{{ value_json.action }}",
		Icon:              "mdi:gesture-tap-button",
		Device:            device,
		AvailabilityTopic: fmt.Sprintf("%s/bridge/state", baseTopic),
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
