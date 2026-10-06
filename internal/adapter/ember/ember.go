package ember

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/julienbreux/zigbridge/internal/adapter"
	"github.com/julienbreux/zigbridge/internal/transport"
	"github.com/julienbreux/zigbridge/internal/zcl"
)

// EZSP Command IDs
const (
	EZSPVersion                uint16 = 0x0000
	EZSPPermitJoining          uint16 = 0x0022
	EZSPSendUnicast            uint16 = 0x0034
	EZSPBindRequest            uint16 = 0x003F
	EZSPIncomingMessageHandler uint16 = 0x0045
	EZSPTrustCenterJoinHandler uint16 = 0x0024
)

// EmberAdapter implements the Adapter interface for Silicon Labs EmberZNet/EZSP coordinators.
type EmberAdapter struct {
	transport transport.Transport
	info      adapter.AdapterInfo

	mu           sync.Mutex
	running      bool
	frameHandler adapter.FrameHandler
	joinHandler  adapter.DeviceJoinHandler
	sequence     uint8

	cancel context.CancelFunc
}

// New creates a new EmberAdapter instance.
func New(channel uint8, panID uint16, extPanID string) *EmberAdapter {
	return &EmberAdapter{
		info: adapter.AdapterInfo{
			Type:     "Silicon Labs EmberZNet (EZSP)",
			Version:  "7.4.0 (EZSP v8+)",
			Channel:  channel,
			PanID:    panID,
			ExtPanID: extPanID,
			IEEE:     "0x000D6F0015ABCDEF",
			Status:   "ready",
		},
	}
}

func (e *EmberAdapter) Init(ctx context.Context, t transport.Transport) error {
	e.transport = t
	return nil
}

func (e *EmberAdapter) Start(ctx context.Context) error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return nil
	}

	readCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	e.running = true
	e.info.Status = "running"
	e.mu.Unlock()

	go e.readLoop(readCtx)
	return nil
}

func (e *EmberAdapter) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return nil
	}

	e.running = false
	e.info.Status = "stopped"
	if e.cancel != nil {
		e.cancel()
	}

	return nil
}

func (e *EmberAdapter) Reset(ctx context.Context) error {
	// Send EZSP Version probe
	return e.sendEZSPCommand(EZSPVersion, []byte{0x08}) // EZSP version 8
}

func (e *EmberAdapter) PermitJoin(ctx context.Context, duration uint8) error {
	// EZSP PermitJoining command
	return e.sendEZSPCommand(EZSPPermitJoining, []byte{duration})
}

func (e *EmberAdapter) Bind(ctx context.Context, req adapter.BindRequest) error {
	// EZSP BindRequest
	payload := make([]byte, 14)
	binary.LittleEndian.PutUint16(payload[0:2], uint16(req.ClusterID))
	payload[2] = req.SrcEndpoint
	payload[3] = req.DstEndpoint
	return e.sendEZSPCommand(EZSPBindRequest, payload)
}

func (e *EmberAdapter) Unbind(ctx context.Context, req adapter.BindRequest) error {
	// In EZSP, unbind is often clearing or setting duration 0
	return e.Bind(ctx, req)
}

func (e *EmberAdapter) SendZCL(ctx context.Context, frame *zcl.Frame) error {
	encoded, err := frame.Encode(nil)
	if err != nil {
		return err
	}

	payload := make([]byte, 8+len(encoded))
	binary.LittleEndian.PutUint16(payload[0:2], 0x0000) // NWK
	binary.LittleEndian.PutUint16(payload[2:4], uint16(frame.ClusterID))
	payload[4] = frame.DestEndpoint
	payload[5] = frame.SourceEndpoint
	binary.LittleEndian.PutUint16(payload[6:8], uint16(len(encoded)))
	copy(payload[8:], encoded)

	return e.sendEZSPCommand(EZSPSendUnicast, payload)
}

func (e *EmberAdapter) sendEZSPCommand(cmdID uint16, params []byte) error {
	e.mu.Lock()
	seq := e.sequence
	e.sequence++
	t := e.transport
	e.mu.Unlock()

	if t == nil {
		return transport.ErrNotConnected
	}

	// Simple EZSP framing: [Seq] [FrameControl] [CmdID (2)] [Params...]
	frame := make([]byte, 4+len(params))
	frame[0] = seq
	frame[1] = 0x00 // standard command
	binary.LittleEndian.PutUint16(frame[2:4], cmdID)
	copy(frame[4:], params)

	_, err := t.Write(frame)
	return err
}

func (e *EmberAdapter) readLoop(ctx context.Context) {
	buf := make([]byte, 256)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if e.transport == nil || !e.transport.IsConnected() {
			continue
		}

		n, err := e.transport.Read(buf)
		if err != nil || n < 4 {
			continue
		}

		cmdID := binary.LittleEndian.Uint16(buf[2:4])
		data := buf[4:n]

		switch cmdID {
		case EZSPIncomingMessageHandler:
			if len(data) >= 8 {
				clusterID := zcl.ClusterID(binary.LittleEndian.Uint16(data[2:4]))
				srcEP := data[4]
				dstEP := data[5]
				zclPayload := data[8:]

				parsedFrame, err := zcl.Decode(zclPayload)
				if err == nil {
					parsedFrame.ClusterID = clusterID
					parsedFrame.SourceAddress = "0x0000"
					parsedFrame.SourceEndpoint = srcEP
					parsedFrame.DestEndpoint = dstEP

					e.mu.Lock()
					handler := e.frameHandler
					e.mu.Unlock()

					if handler != nil {
						handler(parsedFrame)
					}
				}
			}

		case EZSPTrustCenterJoinHandler:
			if len(data) >= 8 {
				ieee := fmt.Sprintf("0x%016X", binary.LittleEndian.Uint64(data[0:8]))
				e.mu.Lock()
				handler := e.joinHandler
				e.mu.Unlock()

				if handler != nil {
					handler(adapter.DeviceJoinInfo{
						IEEE: ieee,
					})
				}
			}
		}
	}
}

func (e *EmberAdapter) RegisterFrameHandler(handler adapter.FrameHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.frameHandler = handler
}

func (e *EmberAdapter) RegisterDeviceJoinHandler(handler adapter.DeviceJoinHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.joinHandler = handler
}

func (e *EmberAdapter) Info() adapter.AdapterInfo {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.info
}
