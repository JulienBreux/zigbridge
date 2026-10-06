package adapter

import (
	"context"

	"github.com/julienbreux/zigbridge/internal/transport"
	"github.com/julienbreux/zigbridge/internal/zcl"
)

// BindRequest defines the parameters for creating or removing a direct Zigbee binding.
type BindRequest struct {
	SrcIEEE     string        `json:"src_ieee"`
	SrcEndpoint uint8         `json:"src_endpoint"`
	ClusterID   zcl.ClusterID `json:"cluster_id"`
	DstIEEE     string        `json:"dst_ieee"`
	DstEndpoint uint8         `json:"dst_endpoint"`
}

// DeviceJoinInfo contains metadata reported by the coordinator when a new device pairs.
type DeviceJoinInfo struct {
	IEEE         string `json:"ieee"`
	NWK          uint16 `json:"nwk"`
	ParentNWK    uint16 `json:"parent_nwk"`
	Capabilities uint8  `json:"capabilities"`
}

// AdapterInfo summarizes coordinator hardware and network state.
type AdapterInfo struct {
	Type     string `json:"type"`       // "Z-Stack 3.x", "EmberZNet/EZSP", etc.
	Version  string `json:"version"`    // Firmware version string
	Channel  uint8  `json:"channel"`    // Current Zigbee channel (11-26)
	PanID    uint16 `json:"pan_id"`     // 16-bit PAN identifier
	ExtPanID string `json:"ext_pan_id"` // 64-bit Extended PAN identifier
	IEEE     string `json:"ieee"`       // Coordinator IEEE address
	Status   string `json:"status"`     // "ready", "running", "stopped"
}

// FrameHandler processes incoming parsed ZCL frames from devices.
type FrameHandler func(frame *zcl.Frame)

// DeviceJoinHandler processes new device association / announcement events.
type DeviceJoinHandler func(info DeviceJoinInfo)

// Adapter defines the decoupled radio interface for Zigbee coprocessors.
type Adapter interface {
	// Init attaches the adapter to the underlying transport stream.
	Init(ctx context.Context, t transport.Transport) error

	// Start initializes firmware configuration, sets channel/PAN, and starts listening.
	Start(ctx context.Context) error

	// Stop cleanly terminates radio listening and resets state.
	Stop() error

	// PermitJoin opens or closes network joining (duration in seconds; 0 = closed, 254 = max).
	PermitJoin(ctx context.Context, duration uint8) error

	// Reset performs a soft reset of the radio coprocessor.
	Reset(ctx context.Context) error

	// SendZCL sends an application-layer ZCL command to a target device endpoint.
	SendZCL(ctx context.Context, frame *zcl.Frame) error

	// Bind requests the coordinator to create a direct binding between two nodes.
	Bind(ctx context.Context, req BindRequest) error

	// Unbind requests the coordinator to remove an existing direct binding.
	Unbind(ctx context.Context, req BindRequest) error

	// RegisterFrameHandler sets the callback for incoming ZCL frames.
	RegisterFrameHandler(handler FrameHandler)

	// RegisterDeviceJoinHandler sets the callback for new device announcements.
	RegisterDeviceJoinHandler(handler DeviceJoinHandler)

	// Info returns the current coordinator firmware, network, and operational info.
	Info() AdapterInfo
}
