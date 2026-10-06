package fixture

import (
	"errors"
	"fmt"
)

// DeviceDefinition models a device catalog specification from Zigbee2MQTT.
type DeviceDefinition struct {
	SchemaVersion string     `yaml:"schema_version" json:"schema_version"`
	Device        DeviceMeta `yaml:"device" json:"device"`
}

// DeviceMeta contains the hardware and profile metadata of the device.
type DeviceMeta struct {
	Model        string        `yaml:"model" json:"model"`
	Vendor       string        `yaml:"vendor" json:"vendor"`
	Description  string        `yaml:"description" json:"description"`
	ZigbeeModels []string      `yaml:"zigbee_models" json:"zigbee_models"`
	Endpoints    []EndpointDef `yaml:"endpoints" json:"endpoints"`
	Exposes      []ExposeDef   `yaml:"exposes" json:"exposes"`
	Simulations  SimulationDef `yaml:"simulations" json:"simulations"`
}

// EndpointDef represents a Zigbee application endpoint with its clusters.
type EndpointDef struct {
	Endpoint       uint8    `yaml:"endpoint" json:"endpoint"`
	ProfileID      uint16   `yaml:"profile_id" json:"profile_id"`
	DeviceID       uint16   `yaml:"device_id" json:"device_id"`
	InputClusters  []uint16 `yaml:"input_clusters" json:"input_clusters"`
	OutputClusters []uint16 `yaml:"output_clusters" json:"output_clusters"`
}

// ExposeDef describes an exposed telemetry or control property.
type ExposeDef struct {
	Type        string   `yaml:"type" json:"type"` // enum, numeric, binary, composite
	Name        string   `yaml:"name" json:"name"`
	Property    string   `yaml:"property" json:"property"`
	Description string   `yaml:"description" json:"description"`
	Unit        string   `yaml:"unit,omitempty" json:"unit,omitempty"`
	Values      []string `yaml:"values,omitempty" json:"values,omitempty"`
	Min         *float64 `yaml:"min,omitempty" json:"min,omitempty"`
	Max         *float64 `yaml:"max,omitempty" json:"max,omitempty"`
	Access      uint8    `yaml:"access" json:"access"`
}

// SimulationDef maps high-level test actions to raw ZCL frames and telemetry payloads.
type SimulationDef struct {
	Actions   map[string]ActionSim    `yaml:"actions,omitempty" json:"actions,omitempty"`
	Telemetry map[string]TelemetrySim `yaml:"telemetry,omitempty" json:"telemetry,omitempty"`
}

// ActionSim defines simulated cluster and command invocation for a button action.
type ActionSim struct {
	Cluster     uint16         `yaml:"cluster" json:"cluster"`
	Command     uint8          `yaml:"command" json:"command"`
	Payload     []byte         `yaml:"payload,omitempty" json:"payload,omitempty"`
	MQTTPayload map[string]any `yaml:"mqtt_payload" json:"mqtt_payload"`
}

// TelemetrySim defines cluster and attribute identifiers for sensor reporting.
type TelemetrySim struct {
	Cluster          uint16 `yaml:"cluster" json:"cluster"`
	Attribute        uint16 `yaml:"attribute" json:"attribute"`
	VoltageAttribute uint16 `yaml:"voltage_attribute,omitempty" json:"voltage_attribute,omitempty"`
}

// Validate ensures required fields are set and values are valid.
func (d *DeviceDefinition) Validate() error {
	if d.Device.Model == "" {
		return errors.New("device model cannot be empty")
	}
	if d.Device.Vendor == "" {
		return errors.New("device vendor cannot be empty")
	}
	if len(d.Device.Endpoints) == 0 {
		return fmt.Errorf("device %s must define at least one endpoint", d.Device.Model)
	}
	return nil
}
