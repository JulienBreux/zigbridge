package zstack

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
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
	CmdSYSPing              uint16 = 0x2101
	CmdSYSPingRsp           uint16 = 0x6101
	CmdSYSResetReq          uint16 = 0x4100
	CmdSYSResetInd          uint16 = 0x4180
	CmdSYSVersion           uint16 = 0x2102
	CmdSYSVersionRsp        uint16 = 0x6102
	CmdUTILGetDeviceInfo    uint16 = 0x2700
	CmdUTILGetDeviceInfoRsp uint16 = 0x6700
	CmdUTILPermitJoinReq    uint16 = 0x270B
	CmdUTILPermitJoinRsp    uint16 = 0x670B
	CmdZDOMgmtPermitJoinReq uint16 = 0x2536
	CmdZDOMgmtPermitJoinRsp uint16 = 0x6536
	CmdZDOTCDevInd          uint16 = 0x45CA
	CmdZDOEndDeviceAnnceInd uint16 = 0x45C1
	CmdZDOBindReq           uint16 = 0x2521
	CmdZDOUnbindReq         uint16 = 0x2522
	CmdAFDataRequest        uint16 = 0x2401
	CmdAFDataRequestRsp     uint16 = 0x6401
	CmdAFIncomingMsg        uint16 = 0x4481
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

	nwkToIEEE map[uint16]string
	ieeeToNWK map[string]uint16

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
		nwkToIEEE: make(map[uint16]string),
		ieeeToNWK: make(map[string]uint16),
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

	// Send non-disruptive queries to coordinator
	// 1. SYS_PING (0x2101)
	_ = z.sendFrame(&MTFrame{Cmd0: TypeSREQ | SubsystemSYS, Cmd1: 0x01, Data: nil})
	// 2. SYS_VERSION (0x2102)
	_ = z.sendFrame(&MTFrame{Cmd0: TypeSREQ | SubsystemSYS, Cmd1: 0x02, Data: nil})
	// 3. UTIL_GET_DEVICE_INFO (0x2700)
	_ = z.sendFrame(&MTFrame{Cmd0: TypeSREQ | SubsystemUTIL, Cmd1: 0x00, Data: nil})

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
	// 1. UTIL_PERMIT_JOIN_REQ (0x270B) - CC2652 Local Coordinator MAC Layer Permit Join
	utilFrame := MTFrame{
		Cmd0: TypeSREQ | SubsystemUTIL,
		Cmd1: 0x0B,
		Data: []byte{duration},
	}
	if err := z.sendFrame(&utilFrame); err != nil {
		return fmt.Errorf("util permit join failed: %w", err)
	}

	// 2. ZDO_MGMT_PERMIT_JOIN_REQ (0x2536) - Broadcast to all routers & coordinator (0xFFFC)
	// AddrMode: 0x02 (16-bit address), DstAddr: 0xFFFC, Duration: duration, TCSignificance: 0x01
	zdoBroadcastData := []byte{
		0x02,       // AddrMode: 16-bit
		0xFC, 0xFF, // DstAddr: 0xFFFC
		duration,   // Duration in seconds
		0x01,       // TCSignificance: 1 (Trust Center Link Key exchange permitted)
	}
	zdoBroadcast := MTFrame{
		Cmd0: TypeSREQ | SubsystemZDO,
		Cmd1: 0x36,
		Data: zdoBroadcastData,
	}
	_ = z.sendFrame(&zdoBroadcast)

	// 3. ZDO_MGMT_PERMIT_JOIN_REQ (0x2536) - Unicast directly to coordinator (0x0000)
	zdoUnicastData := []byte{
		0x02,       // AddrMode: 16-bit
		0x00, 0x00, // DstAddr: 0x0000
		duration,   // Duration in seconds
		0x01,       // TCSignificance: 1
	}
	zdoUnicast := MTFrame{
		Cmd0: TypeSREQ | SubsystemZDO,
		Cmd1: 0x36,
		Data: zdoUnicastData,
	}
	_ = z.sendFrame(&zdoUnicast)

	return nil
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

func (z *ZStackAdapter) resolveNWK(addr string) uint16 {
	z.mu.Lock()
	defer z.mu.Unlock()

	if nwk, ok := z.ieeeToNWK[addr]; ok {
		return nwk
	}
	if nwk, ok := z.ieeeToNWK[strings.ToLower(addr)]; ok {
		return nwk
	}
	if nwk, ok := z.ieeeToNWK[strings.ToUpper(addr)]; ok {
		return nwk
	}

	clean := strings.TrimPrefix(strings.ToLower(addr), "0x")
	if len(clean) <= 4 && len(clean) > 0 {
		if val, err := strconv.ParseUint(clean, 16, 16); err == nil {
			return uint16(val)
		}
	}
	return 0x0000
}

func (z *ZStackAdapter) SendZCL(ctx context.Context, frame *zcl.Frame) error {
	encodedPayload, err := frame.Encode(nil)
	if err != nil {
		return err
	}

	targetNWK := z.resolveNWK(frame.DestAddress)
	srcEP := frame.SourceEndpoint
	if srcEP == 0 {
		srcEP = 1
	}

	// AF_DATA_REQUEST: DstAddr(2), DstEP(1), SrcEP(1), ClusterID(2), TransID(1), Options(1), Radius(1), Len(1), Data
	buf := make([]byte, 10+len(encodedPayload))
	binary.LittleEndian.PutUint16(buf[0:2], targetNWK)
	buf[2] = frame.DestEndpoint
	buf[3] = srcEP
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
	var reader *bufio.Reader

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if z.transport == nil || !z.transport.IsConnected() {
			reader = nil
			time.Sleep(100 * time.Millisecond)
			continue
		}

		if reader == nil {
			reader = bufio.NewReaderSize(z.transport, 1024)
		}

		// Find SOF byte
		b, err := reader.ReadByte()
		if err != nil {
			reader = nil
			time.Sleep(20 * time.Millisecond)
			continue
		}
		if b != SOF {
			continue
		}

		// Read Len, Cmd0, Cmd1
		_, err = io.ReadFull(reader, header[1:4])
		if err != nil {
			reader = nil
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
			_, err = io.ReadFull(reader, data)
			if err != nil {
				reader = nil
				continue
			}
		}

		fcsByte, err := reader.ReadByte()
		if err != nil {
			reader = nil
			continue
		}

		expectedFCS := CalculateFCS(byte(length), cmd0, cmd1, data)
		if fcsByte != expectedFCS {
			continue // Discard frame on CRC error
		}

		z.handleIncomingMT(cmd0, cmd1, data)
	}
}

func (z *ZStackAdapter) handleIncomingMT(cmd0, cmd1 byte, data []byte) {
	cmdID := (uint16(cmd0) << 8) | uint16(cmd1)

	switch cmdID {
	case CmdUTILGetDeviceInfoRsp: // 0x6700
		if len(data) >= 9 && data[0] == 0 {
			coordIEEE := fmt.Sprintf("0x%016X", binary.LittleEndian.Uint64(data[1:9]))
			z.mu.Lock()
			z.info.IEEE = coordIEEE
			z.info.Status = "ready"
			z.mu.Unlock()
			log.Printf("[ZSTACK] Coordinator device info: IEEE=%s", coordIEEE)
		}

	case CmdSYSVersionRsp: // 0x6102
		if len(data) >= 5 {
			ver := fmt.Sprintf("3.x (TI MT %d.%d.%d)", data[2], data[3], data[4])
			z.mu.Lock()
			z.info.Version = ver
			z.mu.Unlock()
			log.Printf("[ZSTACK] Firmware version: %s", ver)
		}

	case CmdAFIncomingMsg: // 0x4481
		// Parse AF incoming message and convert to ZCL Frame
		if len(data) >= 17 {
			clusterID := zcl.ClusterID(binary.LittleEndian.Uint16(data[2:4]))
			nwk := binary.LittleEndian.Uint16(data[4:6])
			srcAddr := fmt.Sprintf("0x%04X", nwk)

			z.mu.Lock()
			if ieee, ok := z.nwkToIEEE[nwk]; ok {
				srcAddr = ieee
			}
			z.mu.Unlock()

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

	case CmdZDOTCDevInd: // 0x45CA - Trust Center Device Indication
		if len(data) >= 10 {
			nwk := binary.LittleEndian.Uint16(data[0:2])
			ieee := fmt.Sprintf("0x%016X", binary.LittleEndian.Uint64(data[2:10]))

			z.mu.Lock()
			z.nwkToIEEE[nwk] = ieee
			z.ieeeToNWK[ieee] = nwk
			z.ieeeToNWK[strings.ToLower(ieee)] = nwk
			handler := z.joinHandler
			z.mu.Unlock()

			log.Printf("[ZSTACK] Trust Center Device Indication: IEEE=%s NWK=0x%04X", ieee, nwk)

			if handler != nil {
				handler(adapter.DeviceJoinInfo{
					IEEE:         ieee,
					NWK:          nwk,
					Capabilities: 0,
				})
			}
		}

	case CmdZDOEndDeviceAnnceInd: // 0x45C1 - End Device Announcement
		if len(data) >= 11 {
			nwk := binary.LittleEndian.Uint16(data[0:2])
			ieee := fmt.Sprintf("0x%016X", binary.LittleEndian.Uint64(data[2:10]))
			cap := data[10]

			z.mu.Lock()
			z.nwkToIEEE[nwk] = ieee
			z.ieeeToNWK[ieee] = nwk
			z.ieeeToNWK[strings.ToLower(ieee)] = nwk
			handler := z.joinHandler
			z.mu.Unlock()

			log.Printf("[ZSTACK] Device Announcement: IEEE=%s NWK=0x%04X Cap=0x%02X", ieee, nwk, cap)

			if handler != nil {
				handler(adapter.DeviceJoinInfo{
					IEEE:         ieee,
					NWK:          nwk,
					Capabilities: cap,
				})
			}
		}

	case CmdUTILPermitJoinRsp: // 0x670B
		if len(data) > 0 {
			log.Printf("[ZSTACK] UTIL_PERMIT_JOIN_RSP received: Status=0x%02X", data[0])
		}

	case CmdZDOMgmtPermitJoinRsp: // 0x6536
		if len(data) > 0 {
			log.Printf("[ZSTACK] ZDO_MGMT_PERMIT_JOIN_RSP received: Status=0x%02X", data[0])
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
