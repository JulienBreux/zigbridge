package zcl

import "fmt"

// ClusterID represents a 16-bit Zigbee Cluster Identifier.
type ClusterID uint16

// Standard Zigbee Clusters (ZCL Specification)
const (
	ClusterBasic                  ClusterID = 0x0000
	ClusterPowerConfiguration     ClusterID = 0x0001
	ClusterDeviceTemperature      ClusterID = 0x0002
	ClusterIdentify               ClusterID = 0x0003
	ClusterGroups                 ClusterID = 0x0004
	ClusterScenes                 ClusterID = 0x0005
	ClusterOnOff                  ClusterID = 0x0006
	ClusterOnOffSwitchConfig      ClusterID = 0x0007
	ClusterLevelControl           ClusterID = 0x0008
	ClusterAlarms                 ClusterID = 0x0009
	ClusterTime                   ClusterID = 0x000A
	ClusterAnalogInputBasic       ClusterID = 0x000C
	ClusterMultistateInputBasic   ClusterID = 0x0012
	ClusterOTAUpgrade             ClusterID = 0x0019
	ClusterColorControl           ClusterID = 0x0300
	ClusterIlluminanceMeasurement ClusterID = 0x0400
	ClusterTemperatureMeasurement ClusterID = 0x0402
	ClusterPressureMeasurement    ClusterID = 0x0403
	ClusterRelativeHumidity       ClusterID = 0x0405
	ClusterOccupancySensing       ClusterID = 0x0406
	ClusterIASZone                ClusterID = 0x0500
	ClusterIASACE                 ClusterID = 0x0501
	ClusterIASWD                  ClusterID = 0x0502
	ClusterMetering               ClusterID = 0x0702
	ClusterElectricalMeasurement  ClusterID = 0x0B04
	ClusterDiagnostic             ClusterID = 0x0B05
)

// ClusterName returns the standard name for known Zigbee clusters.
func (c ClusterID) String() string {
	switch c {
	case ClusterBasic:
		return "Basic"
	case ClusterPowerConfiguration:
		return "PowerConfiguration"
	case ClusterDeviceTemperature:
		return "DeviceTemperature"
	case ClusterIdentify:
		return "Identify"
	case ClusterGroups:
		return "Groups"
	case ClusterScenes:
		return "Scenes"
	case ClusterOnOff:
		return "OnOff"
	case ClusterOnOffSwitchConfig:
		return "OnOffSwitchConfig"
	case ClusterLevelControl:
		return "LevelControl"
	case ClusterColorControl:
		return "ColorControl"
	case ClusterIlluminanceMeasurement:
		return "IlluminanceMeasurement"
	case ClusterTemperatureMeasurement:
		return "TemperatureMeasurement"
	case ClusterPressureMeasurement:
		return "PressureMeasurement"
	case ClusterRelativeHumidity:
		return "RelativeHumidity"
	case ClusterOccupancySensing:
		return "OccupancySensing"
	case ClusterIASZone:
		return "IASZone"
	case ClusterIASACE:
		return "IASACE"
	case ClusterIASWD:
		return "IASWD"
	case ClusterMetering:
		return "Metering"
	case ClusterElectricalMeasurement:
		return "ElectricalMeasurement"
	default:
		return fmt.Sprintf("Cluster(0x%04X)", uint16(c))
	}
}

// Global ZCL Command Identifiers
const (
	CmdReadAttributes                 uint8 = 0x00
	CmdReadAttributesResponse         uint8 = 0x01
	CmdWriteAttributes                uint8 = 0x02
	CmdWriteAttributesUndivided       uint8 = 0x03
	CmdWriteAttributesResponse        uint8 = 0x04
	CmdWriteAttributesNoResponse      uint8 = 0x05
	CmdConfigureReporting             uint8 = 0x06
	CmdConfigureReportingResponse     uint8 = 0x07
	CmdReadReportingConfiguration     uint8 = 0x08
	CmdReadReportingConfigurationResp uint8 = 0x09
	CmdReportAttributes               uint8 = 0x0A
	CmdDefaultResponse                uint8 = 0x0B
	CmdDiscoverAttributes             uint8 = 0x0C
	CmdDiscoverAttributesResponse     uint8 = 0x0D
)

// Cluster-Specific Commands: OnOff
const (
	CmdOnOffOff    uint8 = 0x00
	CmdOnOffOn     uint8 = 0x01
	CmdOnOffToggle uint8 = 0x02
)

// Cluster-Specific Commands: LevelControl
const (
	CmdLevelMoveToLevel          uint8 = 0x00
	CmdLevelMove                 uint8 = 0x01
	CmdLevelStep                 uint8 = 0x02
	CmdLevelStop                 uint8 = 0x03
	CmdLevelMoveToLevelWithOnOff uint8 = 0x04
	CmdLevelMoveWithOnOff        uint8 = 0x05
	CmdLevelStepWithOnOff        uint8 = 0x06
	CmdLevelStopWithOnOff        uint8 = 0x07
)

// Cluster-Specific Commands: IAS Zone (0x0500)
const (
	CmdIASZoneStatusChangeNotification uint8 = 0x00
	CmdIASZoneEnrollRequest            uint8 = 0x01
	CmdIASZoneEnrollResponse           uint8 = 0x00
)

// Cluster-Specific Commands: IAS ACE (0x0501)
const (
	CmdIASACEArm       uint8 = 0x00
	CmdIASACEBypass    uint8 = 0x01
	CmdIASACEEmergency uint8 = 0x02
	CmdIASACEFire      uint8 = 0x03
	CmdIASACEPanic     uint8 = 0x04
)

// Cluster-Specific Commands: IAS WD (0x0502)
const (
	CmdIASWDStartWarning uint8 = 0x00
	CmdIASWDSquawk       uint8 = 0x01
)

// ZCL Data Types
const (
	TypeData8      uint8 = 0x08
	TypeData16     uint8 = 0x09
	TypeBoolean    uint8 = 0x10
	TypeBitmap8    uint8 = 0x18
	TypeBitmap16   uint8 = 0x19
	TypeUint8      uint8 = 0x20
	TypeUint16     uint8 = 0x21
	TypeUint24     uint8 = 0x22
	TypeUint32     uint8 = 0x23
	TypeUint48     uint8 = 0x25
	TypeInt8       uint8 = 0x28
	TypeInt16      uint8 = 0x29
	TypeInt32      uint8 = 0x2B
	TypeEnum8      uint8 = 0x30
	TypeEnum16     uint8 = 0x31
	TypeSinglePrec uint8 = 0x39
	TypeDoublePrec uint8 = 0x3A
	TypeOctetStr   uint8 = 0x41
	TypeCharStr    uint8 = 0x42
	TypeIEEEAddr   uint8 = 0xF0
)
