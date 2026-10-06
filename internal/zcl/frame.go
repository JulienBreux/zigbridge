package zcl

import (
	"encoding/binary"
	"errors"
	"fmt"
)

var (
	ErrFrameTooShort       = errors.New("zcl: frame is too short")
	ErrBufferTooSmall      = errors.New("zcl: target buffer too small for encoding")
	ErrUnsupportedDataType = errors.New("zcl: unsupported attribute data type")
)

// FrameType indicates Global (Profile-wide) or Cluster-Specific command.
type FrameType uint8

const (
	FrameTypeGlobal          FrameType = 0
	FrameTypeClusterSpecific FrameType = 1
)

// Direction indicates whether frame is sent client-to-server or server-to-client.
type Direction uint8

const (
	DirectionClientToServer Direction = 0
	DirectionServerToClient Direction = 1
)

// FrameControl models the 8-bit ZCL Frame Control field.
type FrameControl struct {
	Type                   FrameType
	ManufacturerSpecific   bool
	Direction              Direction
	DisableDefaultResponse bool
}

// Encode serializes the FrameControl into a byte.
func (fc FrameControl) Encode() byte {
	var b byte
	if fc.Type == FrameTypeClusterSpecific {
		b |= 0x01
	}
	if fc.ManufacturerSpecific {
		b |= 0x04
	}
	if fc.Direction == DirectionServerToClient {
		b |= 0x08
	}
	if fc.DisableDefaultResponse {
		b |= 0x10
	}
	return b
}

// DecodeFrameControl extracts FrameControl flags from a raw byte.
func DecodeFrameControl(b byte) FrameControl {
	return FrameControl{
		Type:                   FrameType(b & 0x03),
		ManufacturerSpecific:   (b & 0x04) != 0,
		Direction:              Direction((b >> 3) & 0x01),
		DisableDefaultResponse: (b & 0x10) != 0,
	}
}

// Frame represents a complete ZCL frame with header, payload, and transport metadata.
type Frame struct {
	// ZCL Header
	Header                 FrameControl
	ManufacturerCode       uint16
	TransactionSequenceNum uint8
	CommandID              uint8

	// ZCL Payload (sliced without extra allocation)
	Payload []byte

	// Routing & Addressing Metadata
	ClusterID      ClusterID
	SourceAddress  string // 64-bit IEEE hex or 16-bit NWK hex
	SourceEndpoint uint8
	DestAddress    string
	DestEndpoint   uint8
	LQI            uint8 // Link Quality Indication
}

// Decode parses a raw byte slice into a ZCL Frame using sub-slicing to avoid heap allocations.
func Decode(data []byte) (*Frame, error) {
	if len(data) < 3 {
		return nil, ErrFrameTooShort
	}

	fc := DecodeFrameControl(data[0])
	offset := 1

	var mfgCode uint16
	if fc.ManufacturerSpecific {
		if len(data) < offset+2 {
			return nil, ErrFrameTooShort
		}
		mfgCode = binary.LittleEndian.Uint16(data[offset : offset+2])
		offset += 2
	}

	if len(data) < offset+2 {
		return nil, ErrFrameTooShort
	}

	seq := data[offset]
	cmd := data[offset+1]
	offset += 2

	var payload []byte
	if offset < len(data) {
		payload = data[offset:]
	}

	return &Frame{
		Header:                 fc,
		ManufacturerCode:       mfgCode,
		TransactionSequenceNum: seq,
		CommandID:              cmd,
		Payload:                payload,
	}, nil
}

// Encode appends serialized ZCL frame bytes to dst, reusing dst memory if capacity allows.
func (f *Frame) Encode(dst []byte) ([]byte, error) {
	required := 3 + len(f.Payload)
	if f.Header.ManufacturerSpecific {
		required += 2
	}

	// Ensure capacity
	if cap(dst)-len(dst) < required {
		newDst := make([]byte, len(dst), len(dst)+required)
		copy(newDst, dst)
		dst = newDst
	}

	dst = append(dst, f.Header.Encode())

	if f.Header.ManufacturerSpecific {
		dst = append(dst, byte(f.ManufacturerCode), byte(f.ManufacturerCode>>8))
	}

	dst = append(dst, f.TransactionSequenceNum, f.CommandID)
	dst = append(dst, f.Payload...)

	return dst, nil
}

func (f *Frame) String() string {
	return fmt.Sprintf("ZCL[Cluster: %s, Cmd: 0x%02X, Seq: %d, From: %s:%d, To: %s:%d, Len: %d]",
		f.ClusterID.String(), f.CommandID, f.TransactionSequenceNum,
		f.SourceAddress, f.SourceEndpoint, f.DestAddress, f.DestEndpoint, len(f.Payload))
}
