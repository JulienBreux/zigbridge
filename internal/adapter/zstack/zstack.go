package zstack

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/julienbreux/zigbridge/internal/adapter"
	"github.com/julienbreux/zigbridge/internal/transport"
	"github.com/julienbreux/zigbridge/internal/zcl"
)

const (
	SOF byte = 0xFE

	// Subsystem IDs (Cmd0 bits 0-4)
	SubsystemSYS  byte = 0x01
	SubsystemAF   byte = 0x04
	SubsystemZDO  byte = 0x05
	SubsystemSAPI byte = 0x06
	SubsystemUTIL byte = 0x07

	// Type Masks (Cmd0 bits 5-7)
	TypePOLL byte = 0x00
	TypeSREQ byte = 0x20
	TypeAREQ byte = 0x40
	TypeSRSP byte = 0x60

	// Commands
	CmdSYSResetReq          uint16 = 0x4100
	CmdSYSVersion           uint16 = 0x2102
	CmdZDOMgmtPermitJoinReq uint16 = 0x2536
	CmdZDOBindReq           uint16 = 0x2521
	CmdZDOUnbindReq         uint16 = 0x2522
	CmdAFDataRequest        uint16 = 0x2401
	CmdAFIncomingMsg        uint16 = 0x4481
	CmdZDOEndDeviceAnnceInd uint16 = 0x45C1
)

var (
	ErrChecksumMismatch = errors.New("zstack: MT frame checksum mismatch")
	ErrTimeout          = errors.New("zstack: timeout waiting for response")
)

// MTFrame represents a Texas Instruments Monitor & Test protocol frame.
type MTFrame struct {
	Cmd0 byte
	Cmd1 byte
	Data []byte
}

// CommandID returns the 16-bit command identifier.
func (f *MTFrame) CommandID() uint16 {
	return (uint16(f.Cmd0) << 8) | uint16(f.Cmd1)
}

// CalculateFCS computes the XOR checksum for an MT frame.
func CalculateFCS(lenByte, cmd0, cmd1 byte, data []byte) byte {
	fcs := lenByte ^ cmd0 ^ cmd1
	for _, b := range data {
		fcs ^= b
	}
	return fcs
}

// ZStackAdapter implements the Adapter interface for TI CC2652/CC1352 coordinators.
type ZStackAdapter struct {
	transport transport.Transport
	info      adapter.AdapterInfo

	mu           sync.Mutex
	running      bool
	frameHandler adapter.FrameHandler
	joinHandler  adapter.DeviceJoinHandler

	cancel context.CancelFunc
}

// New creates a new ZStackAdapter instance.
func New(channel uint8, panID uint16, extPanID string) *ZStackAdapter {
	return &ZStackAdapter{
		info: adapter.AdapterInfo{
			Type:     "TI Z-Stack 3.x",
			Version:  "3.30.0 (TI MT)",
			Channel:  channel,
			PanID:    panID,
			ExtPanID: extPanID,
			IEEE:     "0x00124B001F00ABCD",
			Status:   "ready",
		},
	}
}

func (z *ZStackAdapter) Init(ctx context.Context, t transport.Transport) error {
	z.transport = t
	return nil
}

func (z *ZStackAdapter) Start(ctx context.Context) error {
	z.mu.Lock()
	if z.running {
		z.mu.Unlock()
		return nil
	}

	readCtx, cancel := context.WithCancel(ctx)
	z.cancel = cancel
	z.running = true
	z.info.Status = "running"
	z.mu.Unlock()

	go z.readLoop(readCtx)

	// Ping firmware version
	_ = z.Reset(ctx)
	return nil
}

func (z *ZStackAdapter) Stop() error {
	z.mu.Lock()
	defer z.mu.Unlock()

	if !z.running {
		return nil
	}

	z.running = false
	z.info.Status = "stopped"
	if z.cancel != nil {
		z.cancel()
	}

	return nil
}

func (z *ZStackAdapter) Reset(ctx context.Context) error {
	// Send SYS_RESET_REQ: soft reset
	frame := MTFrame{
		Cmd0: TypeAREQ | SubsystemSYS,
		Cmd1: 0x00,
		Data: []byte{0x01}, // Soft reset
	}
	return z.sendFrame(&frame)
}

func (z *ZStackAdapter) PermitJoin(ctx context.Context, duration uint8) error {
	// ZDO_MGMT_PERMIT_JOIN_REQ
	data := []byte{
		0x02, // AddrMode: 15-bit broadcast
		0xFC, 0xFF, // DstAddr: 0xFFFC (all routers and coordinator)
		duration, // Duration in seconds
		0x00,     // TCSignificance: 0
	}

	frame := MTFrame{
		Cmd0: TypeSREQ | SubsystemZDO,
		Cmd1: 0x36,
		Data: data,
	}

	return z.sendFrame(&frame)
}

func (z *ZStackAdapter) Bind(ctx context.Context, req adapter.BindRequest) error {
	return z.sendBindRequest(ctx, req, true)
}

func (z *ZStackAdapter) Unbind(ctx context.Context, req adapter.BindRequest) error {
	return z.sendBindRequest(ctx, req, false)
}

func (z *ZStackAdapter) sendBindRequest(ctx context.Context, req adapter.BindRequest, bind bool) error {
	cmd1 := byte(0x21) // ZDO_BIND_REQ
	if !bind {
		cmd1 = 0x22 // ZDO_UNBIND_REQ
	}

	// Payload: DstAddr (2) + SrcIEEE (8) + SrcEP (1) + ClusterID (2) + DstAddrMode (1) + DstIEEE (8) + DstEP (1)
	data := make([]byte, 23)
	binary.LittleEndian.PutUint16(data[0:2], 0x0000) // Coordinator address or target address
	// Note: IEEE address conversion would normally parse hex strings into 8 little-endian bytes
	data[10] = req.SrcEndpoint
	binary.LittleEndian.PutUint16(data[11:13], uint16(req.ClusterID))
	data[13] = 0x03 // 64-bit IEEE address mode
	data[22] = req.DstEndpoint

	frame := MTFrame{
		Cmd0: TypeSREQ | SubsystemZDO,
		Cmd1: cmd1,
		Data: data,
	}

	return z.sendFrame(&frame)
}

func (z *ZStackAdapter) SendZCL(ctx context.Context, frame *zcl.Frame) error {
	encodedPayload, err := frame.Encode(nil)
	if err != nil {
		return err
	}

	// AF_DATA_REQUEST: DstAddr(2), DstEP(1), SrcEP(1), ClusterID(2), TransID(1), Options(1), Radius(1), Len(1), Data
	buf := make([]byte, 10+len(encodedPayload))
	binary.LittleEndian.PutUint16(buf[0:2], 0x0000) // target NWK
	buf[2] = frame.DestEndpoint
	buf[3] = frame.SourceEndpoint
	binary.LittleEndian.PutUint16(buf[4:6], uint16(frame.ClusterID))
	buf[6] = frame.TransactionSequenceNum
	buf[7] = 0x00 // Options
	buf[8] = 30   // Radius
	buf[9] = byte(len(encodedPayload))
	copy(buf[10:], encodedPayload)

	mt := MTFrame{
		Cmd0: TypeSREQ | SubsystemAF,
		Cmd1: 0x01,
		Data: buf,
	}

	return z.sendFrame(&mt)
}

func (z *ZStackAdapter) sendFrame(frame *MTFrame) error {
	lenByte := byte(len(frame.Data))
	fcs := CalculateFCS(lenByte, frame.Cmd0, frame.Cmd1, frame.Data)

	packet := make([]byte, 5+len(frame.Data))
	packet[0] = SOF
	packet[1] = lenByte
	packet[2] = frame.Cmd0
	packet[3] = frame.Cmd1
	copy(packet[4:4+len(frame.Data)], frame.Data)
	packet[4+len(frame.Data)] = fcs

	if z.transport == nil {
		return transport.ErrNotConnected
	}

	_, err := z.transport.Write(packet)
	return err
}

func (z *ZStackAdapter) readLoop(ctx context.Context) {
	header := make([]byte, 4) // SOF, Len, Cmd0, Cmd1
	dataBuf := make([]byte, 256)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if z.transport == nil || !z.transport.IsConnected() {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Find SOF byte
		b := make([]byte, 1)
		_, err := io.ReadFull(z.transport, b)
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if b[0] != SOF {
			continue
		}

		// Read Len, Cmd0, Cmd1
		_, err = io.ReadFull(z.transport, header[1:4])
		if err != nil {
			continue
		}

		length := int(header[1])
		cmd0 := header[2]
		cmd1 := header[3]

		var data []byte
		if length > 0 {
			if length > len(dataBuf) {
				dataBuf = make([]byte, length)
			}
			data = dataBuf[:length]
			_, err = io.ReadFull(z.transport, data)
			if err != nil {
				continue
			}
		}

		fcsByte := make([]byte, 1)
		_, err = io.ReadFull(z.transport, fcsByte)
		if err != nil {
			continue
		}

		expectedFCS := CalculateFCS(byte(length), cmd0, cmd1, data)
		if fcsByte[0] != expectedFCS {
			continue // Discard frame on CRC error
		}

		z.handleIncomingMT(cmd0, cmd1, data)
	}
}

func (z *ZStackAdapter) handleIncomingMT(cmd0, cmd1 byte, data []byte) {
	cmdID := (uint16(cmd0) << 8) | uint16(cmd1)

	switch cmdID {
	case CmdAFIncomingMsg:
		// Parse AF incoming message and convert to ZCL Frame
		if len(data) >= 17 {
			clusterID := zcl.ClusterID(binary.LittleEndian.Uint16(data[2:4]))
			srcAddr := fmt.Sprintf("0x%04X", binary.LittleEndian.Uint16(data[4:6]))
			srcEP := data[6]
			dstEP := data[7]
			lqi := data[10]
			payloadLen := int(data[16])

			if len(data) >= 17+payloadLen {
				payload := data[17 : 17+payloadLen]
				parsedFrame, err := zcl.Decode(payload)
				if err == nil {
					parsedFrame.ClusterID = clusterID
					parsedFrame.SourceAddress = srcAddr
					parsedFrame.SourceEndpoint = srcEP
					parsedFrame.DestEndpoint = dstEP
					parsedFrame.LQI = lqi

					z.mu.Lock()
					handler := z.frameHandler
					z.mu.Unlock()

					if handler != nil {
						handler(parsedFrame)
					}
				}
			}
		}

	case CmdZDOEndDeviceAnnceInd:
		// New device joined network
		if len(data) >= 11 {
			nwk := binary.LittleEndian.Uint16(data[0:2])
			ieee := fmt.Sprintf("0x%016X", binary.LittleEndian.Uint64(data[2:10]))
			cap := data[10]

			z.mu.Lock()
			handler := z.joinHandler
			z.mu.Unlock()

			if handler != nil {
				handler(adapter.DeviceJoinInfo{
					IEEE:         ieee,
					NWK:          nwk,
					Capabilities: cap,
				})
			}
		}
	}
}

func (z *ZStackAdapter) RegisterFrameHandler(handler adapter.FrameHandler) {
	z.mu.Lock()
	defer z.mu.Unlock()
	z.frameHandler = handler
}

func (z *ZStackAdapter) RegisterDeviceJoinHandler(handler adapter.DeviceJoinHandler) {
	z.mu.Lock()
	defer z.mu.Unlock()
	z.joinHandler = handler
}

func (z *ZStackAdapter) Info() adapter.AdapterInfo {
	z.mu.Lock()
	defer z.mu.Unlock()
	return z.info
}
