package fixture

import (
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/julienbreux/zigbridge/internal/adapter"
	"github.com/julienbreux/zigbridge/internal/zcl"
)

// FrameEmitter represents an adapter or bus capable of receiving simulated device events.
type FrameEmitter interface {
	EmitDeviceJoin(info adapter.DeviceJoinInfo)
	EmitFrame(frame *zcl.Frame)
}

// VirtualDevice represents an active simulated Zigbee device on the network.
type VirtualDevice struct {
	mu      sync.RWMutex
	Def     *DeviceDefinition
	IEEE    string
	NWK     uint16
	emitter FrameEmitter
	seq     uint8
	state   map[string]interface{}
}

// NewVirtualDevice instantiates a virtual device from a definition.
func NewVirtualDevice(def *DeviceDefinition, ieee string, nwk uint16, emitter FrameEmitter) *VirtualDevice {
	return &VirtualDevice{
		Def:     def,
		IEEE:    ieee,
		NWK:     nwk,
		emitter: emitter,
		state:   make(map[string]interface{}),
	}
}

// Spawn instantiates a virtual device and immediately simulates device join.
func Spawn(def *DeviceDefinition, ieee string, nwk uint16, emitter FrameEmitter) (*VirtualDevice, error) {
	v := NewVirtualDevice(def, ieee, nwk, emitter)
	if err := v.SimulateJoin(); err != nil {
		return nil, err
	}
	return v, nil
}

// SimulateJoin fires the device join indication and emits basic identity attribute reports.
func (v *VirtualDevice) SimulateJoin() error {
	if v.emitter == nil {
		return fmt.Errorf("no frame emitter attached to virtual device")
	}

	// 1. Emit association / announcement
	v.emitter.EmitDeviceJoin(adapter.DeviceJoinInfo{
		IEEE:         v.IEEE,
		NWK:          v.NWK,
		Capabilities: 0x8E,
	})

	// 2. Emit basic identity attributes (Manufacturer and Model)
	mfgBytes := []byte(v.Def.Device.Vendor)
	modelBytes := []byte(v.Def.Device.Model)

	var payload []byte
	// Attr 0x0004: Manufacturer Name (TypeCharStr = 0x42)
	payload = append(payload, 0x04, 0x00, zcl.TypeCharStr, byte(len(mfgBytes)))
	payload = append(payload, mfgBytes...)

	// Attr 0x0005: Model Identifier (TypeCharStr = 0x42)
	payload = append(payload, 0x05, 0x00, zcl.TypeCharStr, byte(len(modelBytes)))
	payload = append(payload, modelBytes...)

	v.mu.Lock()
	v.seq++
	seq := v.seq
	v.mu.Unlock()

	v.emitter.EmitFrame(&zcl.Frame{
		Header: zcl.FrameControl{
			Type:      zcl.FrameTypeGlobal,
			Direction: zcl.DirectionServerToClient,
		},
		TransactionSequenceNum: seq,
		CommandID:              zcl.CmdReportAttributes,
		ClusterID:              zcl.ClusterBasic,
		SourceAddress:          v.IEEE,
		SourceEndpoint:         1,
		DestEndpoint:           1,
		LQI:                    255,
		Payload:                payload,
	})

	return nil
}

// TriggerAction triggers a simulated button press or action command.
func (v *VirtualDevice) TriggerAction(actionName string) error {
	if v.emitter == nil {
		return fmt.Errorf("no frame emitter attached to virtual device")
	}

	act, ok := v.Def.Device.Simulations.Actions[actionName]
	if !ok {
		return fmt.Errorf("action %q not defined for device %s", actionName, v.Def.Device.Model)
	}

	v.mu.Lock()
	v.seq++
	seq := v.seq
	v.state["action"] = actionName
	v.mu.Unlock()

	v.emitter.EmitFrame(&zcl.Frame{
		Header: zcl.FrameControl{
			Type:      zcl.FrameTypeClusterSpecific,
			Direction: zcl.DirectionClientToServer,
		},
		TransactionSequenceNum: seq,
		CommandID:              act.Command,
		ClusterID:              zcl.ClusterID(act.Cluster),
		SourceAddress:          v.IEEE,
		SourceEndpoint:         1,
		DestEndpoint:           1,
		LQI:                    255,
		Payload:                act.Payload,
	})

	return nil
}

// ReportBattery simulates a battery percentage and terminal voltage telemetry report.
func (v *VirtualDevice) ReportBattery(percentage uint8, voltageMV uint16) error {
	if v.emitter == nil {
		return fmt.Errorf("no frame emitter attached to virtual device")
	}

	var payload []byte

	// Attribute 0x0021: BatteryPercentageRemaining (uint8, 0-200 for 0-100%)
	zclPct := percentage * 2
	payload = append(payload, 0x21, 0x00, zcl.TypeUint8, zclPct)

	// Attribute 0x0020: BatteryVoltage (uint8, in 100mV units, e.g. 3000mV = 30)
	zclVolt := uint8(voltageMV / 100)
	payload = append(payload, 0x20, 0x00, zcl.TypeUint8, zclVolt)

	v.mu.Lock()
	v.seq++
	seq := v.seq
	v.state["battery"] = percentage
	v.state["voltage"] = voltageMV
	v.mu.Unlock()

	v.emitter.EmitFrame(&zcl.Frame{
		Header: zcl.FrameControl{
			Type:      zcl.FrameTypeGlobal,
			Direction: zcl.DirectionServerToClient,
		},
		TransactionSequenceNum: seq,
		CommandID:              zcl.CmdReportAttributes,
		ClusterID:              zcl.ClusterPowerConfiguration,
		SourceAddress:          v.IEEE,
		SourceEndpoint:         1,
		DestEndpoint:           1,
		LQI:                    255,
		Payload:                payload,
	})

	return nil
}

// ReportTemperatureHumidity simulates environmental sensor reports.
func (v *VirtualDevice) ReportTemperatureHumidity(tempC float64, humidityPct float64) error {
	if v.emitter == nil {
		return fmt.Errorf("no frame emitter attached to virtual device")
	}

	v.mu.Lock()
	v.seq++
	seq := v.seq
	v.state["temperature"] = tempC
	v.state["humidity"] = humidityPct
	v.mu.Unlock()

	// 1. Temperature report (int16, centidegrees)
	rawTemp := int16(tempC * 100.0)
	tempPayload := []byte{0x00, 0x00, zcl.TypeInt16, 0x00, 0x00}
	binary.LittleEndian.PutUint16(tempPayload[3:5], uint16(rawTemp))

	v.emitter.EmitFrame(&zcl.Frame{
		Header: zcl.FrameControl{
			Type:      zcl.FrameTypeGlobal,
			Direction: zcl.DirectionServerToClient,
		},
		TransactionSequenceNum: seq,
		CommandID:              zcl.CmdReportAttributes,
		ClusterID:              zcl.ClusterTemperatureMeasurement,
		SourceAddress:          v.IEEE,
		SourceEndpoint:         1,
		LQI:                    255,
		Payload:                tempPayload,
	})

	// 2. Humidity report (uint16, centipercent)
	rawHumidity := uint16(humidityPct * 100.0)
	humPayload := []byte{0x00, 0x00, zcl.TypeUint16, 0x00, 0x00}
	binary.LittleEndian.PutUint16(humPayload[3:5], rawHumidity)

	v.emitter.EmitFrame(&zcl.Frame{
		Header: zcl.FrameControl{
			Type:      zcl.FrameTypeGlobal,
			Direction: zcl.DirectionServerToClient,
		},
		TransactionSequenceNum: seq + 1,
		CommandID:              zcl.CmdReportAttributes,
		ClusterID:              zcl.ClusterRelativeHumidity,
		SourceAddress:          v.IEEE,
		SourceEndpoint:         1,
		LQI:                    255,
		Payload:                humPayload,
	})

	return nil
}

// GetState returns a snapshot of simulated device state.
func (v *VirtualDevice) GetState() map[string]interface{} {
	v.mu.RLock()
	defer v.mu.RUnlock()

	cp := make(map[string]interface{}, len(v.state))
	for k, val := range v.state {
		cp[k] = val
	}
	return cp
}
