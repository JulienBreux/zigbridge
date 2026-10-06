package zstack

import (
	"context"
	"encoding/binary"
	"sync"
	"testing"

	"github.com/julienbreux/zigbridge/internal/adapter"
	"github.com/julienbreux/zigbridge/internal/transport"
	"github.com/julienbreux/zigbridge/internal/zcl"
)

type recordedFrame struct {
	cmd0 byte
	cmd1 byte
	data []byte
}

type mockZStackTransport struct {
	mu       sync.Mutex
	written  []recordedFrame
	rawBytes []byte
	readBuf  []byte
}

func (m *mockZStackTransport) Open(ctx context.Context) error { return nil }
func (m *mockZStackTransport) Close() error                   { return nil }
func (m *mockZStackTransport) IsConnected() bool              { return true }
func (m *mockZStackTransport) Status() transport.ConnectionStatus {
	return transport.StatusConnected
}
func (m *mockZStackTransport) SetStatusListener(listener transport.StatusListener) {}

func (m *mockZStackTransport) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Parse MTFrame: SOF(1) + Len(1) + Cmd0(1) + Cmd1(1) + Data(Len) + FCS(1)
	if len(p) >= 5 && p[0] == SOF {
		length := int(p[1])
		cmd0 := p[2]
		cmd1 := p[3]
		var data []byte
		if length > 0 && len(p) >= 4+length {
			data = make([]byte, length)
			copy(data, p[4:4+length])
		}
		m.written = append(m.written, recordedFrame{cmd0: cmd0, cmd1: cmd1, data: data})
	}
	m.rawBytes = append(m.rawBytes, p...)
	return len(p), nil
}

func (m *mockZStackTransport) Read(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.readBuf) == 0 {
		return 0, nil
	}
	n := copy(p, m.readBuf)
	m.readBuf = m.readBuf[n:]
	return n, nil
}

func (m *mockZStackTransport) getWritten() []recordedFrame {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([]recordedFrame, len(m.written))
	copy(res, m.written)
	return res
}

func TestZStackPermitJoinFraming(t *testing.T) {
	trans := &mockZStackTransport{}
	z := New(20, 0x1A62, "00124B0001020304")
	ctx := context.Background()

	if err := z.Init(ctx, trans); err != nil {
		t.Fatalf("failed to init adapter: %v", err)
	}

	err := z.PermitJoin(ctx, 60)
	if err != nil {
		t.Fatalf("PermitJoin failed: %v", err)
	}

	frames := trans.getWritten()
	if len(frames) != 4 {
		t.Fatalf("expected 4 permit join frames, got %d", len(frames))
	}

	// 1. UTIL_PERMIT_JOIN_REQ: SubsystemUTIL (0x07), Cmd 0x0B, duration 60
	utilFrame := frames[0]
	if (utilFrame.cmd0&0x1F) != SubsystemUTIL || utilFrame.cmd1 != 0x0B {
		t.Errorf("expected UTIL_PERMIT_JOIN_REQ, got cmd0=0x%02X cmd1=0x%02X", utilFrame.cmd0, utilFrame.cmd1)
	}
	if len(utilFrame.data) != 1 || utilFrame.data[0] != 60 {
		t.Errorf("expected util duration 60, got %v", utilFrame.data)
	}

	// 2. ZDO_MGMT_PERMIT_JOIN_REQ broadcast: SubsystemZDO (0x05), Cmd 0x36, AddrMode 0x0F, DstAddr 0xFFFC, TCSig 0x01
	bcastFrame := frames[1]
	if (bcastFrame.cmd0&0x1F) != SubsystemZDO || bcastFrame.cmd1 != 0x36 {
		t.Errorf("expected ZDO_MGMT_PERMIT_JOIN_REQ broadcast, got cmd0=0x%02X cmd1=0x%02X", bcastFrame.cmd0, bcastFrame.cmd1)
	}
	if len(bcastFrame.data) < 5 {
		t.Fatalf("broadcast data too short: %v", bcastFrame.data)
	}
	if bcastFrame.data[0] != 0x0F {
		t.Errorf("expected broadcast AddrMode 0x0F, got 0x%02X", bcastFrame.data[0])
	}
	dstAddr := binary.LittleEndian.Uint16(bcastFrame.data[1:3])
	if dstAddr != 0xFFFC {
		t.Errorf("expected broadcast dstAddr 0xFFFC, got 0x%04X", dstAddr)
	}
	if bcastFrame.data[3] != 60 {
		t.Errorf("expected broadcast duration 60, got %d", bcastFrame.data[3])
	}
	if bcastFrame.data[4] != 0x01 {
		t.Errorf("expected broadcast TCSignificance 0x01, got %d", bcastFrame.data[4])
	}

	// 3. ZDO_MGMT_PERMIT_JOIN_REQ unicast: SubsystemZDO (0x05), Cmd 0x36, AddrMode 0x02, DstAddr 0x0000, TCSig 0x01
	unicastFrame := frames[2]
	if len(unicastFrame.data) < 5 {
		t.Fatalf("unicast data too short: %v", unicastFrame.data)
	}
	if unicastFrame.data[0] != 0x02 {
		t.Errorf("expected unicast AddrMode 0x02, got 0x%02X", unicastFrame.data[0])
	}
	dstAddrU := binary.LittleEndian.Uint16(unicastFrame.data[1:3])
	if dstAddrU != 0x0000 {
		t.Errorf("expected unicast dstAddr 0x0000, got 0x%04X", dstAddrU)
	}
	if unicastFrame.data[4] != 0x01 {
		t.Errorf("expected unicast TCSignificance 0x01, got %d", unicastFrame.data[4])
	}

	// 4. ZB_PERMIT_JOINING_REQUEST: SubsystemSAPI (0x06), Cmd 0x08, Data [0xFC, 0xFF, 60]
	sapiFrame := frames[3]
	if (sapiFrame.cmd0&0x1F) != SubsystemSAPI || sapiFrame.cmd1 != 0x08 {
		t.Errorf("expected ZB_PERMIT_JOINING_REQUEST, got cmd0=0x%02X cmd1=0x%02X", sapiFrame.cmd0, sapiFrame.cmd1)
	}
	if len(sapiFrame.data) != 3 || sapiFrame.data[2] != 60 {
		t.Errorf("expected sapi permit join duration 60, got %v", sapiFrame.data)
	}
}

func TestZStackStartupAndStateChange(t *testing.T) {
	trans := &mockZStackTransport{}
	z := New(20, 0x1A62, "00124B0001020304")
	ctx := context.Background()

	_ = z.Init(ctx, trans)
	_ = z.Start(ctx)

	frames := trans.getWritten()
	if len(frames) < 6 {
		t.Fatalf("expected at least 6 startup frames, got %d", len(frames))
	}

	// Test state change indication 0x45C0
	z.handleIncomingMT(TypeAREQ|SubsystemZDO, 0xC0, []byte{DevStateZbCoord})
	info := z.Info()
	if info.Status != "ready" {
		t.Errorf("expected status ready after DevStateZbCoord indication, got %s", info.Status)
	}
}

func TestZStackDeviceJoinIndicationAndSendZCL(t *testing.T) {
	trans := &mockZStackTransport{}
	z := New(20, 0x1A62, "00124B0001020304")
	ctx := context.Background()

	_ = z.Init(ctx, trans)

	var joinedInfo adapter.DeviceJoinInfo
	z.RegisterDeviceJoinHandler(func(info adapter.DeviceJoinInfo) {
		joinedInfo = info
	})

	// Simulate Trust Center Device Indication (0x45CA)
	// NWK: 0x1234, IEEE: 0x00124B0001020304
	tcData := make([]byte, 10)
	binary.LittleEndian.PutUint16(tcData[0:2], 0x1234)
	binary.LittleEndian.PutUint64(tcData[2:10], 0x00124B0001020304)

	z.handleIncomingMT(TypeAREQ|SubsystemZDO, 0xCA, tcData)

	if joinedInfo.NWK != 0x1234 {
		t.Errorf("expected joined NWK 0x1234, got 0x%04X", joinedInfo.NWK)
	}
	if joinedInfo.IEEE != "0x00124B0001020304" {
		t.Errorf("expected joined IEEE 0x00124B0001020304, got %s", joinedInfo.IEEE)
	}

	// Verify resolveNWK translates IEEE to NWK
	nwk := z.resolveNWK("0x00124B0001020304")
	if nwk != 0x1234 {
		t.Errorf("expected resolveNWK to return 0x1234, got 0x%04X", nwk)
	}

	// Verify SendZCL routes to resolved NWK (0x1234)
	zclFrame := &zcl.Frame{
		Header: zcl.FrameControl{
			Type:                   zcl.FrameTypeGlobal,
			ManufacturerSpecific:   false,
			Direction:              zcl.DirectionClientToServer,
			DisableDefaultResponse: true,
		},
		ClusterID:              zcl.ClusterOnOff,
		CommandID:              zcl.CmdOnOffToggle,
		DestAddress:            "0x00124B0001020304",
		DestEndpoint:           1,
		SourceEndpoint:         1,
		TransactionSequenceNum: 5,
	}

	if err := z.SendZCL(ctx, zclFrame); err != nil {
		t.Fatalf("SendZCL failed: %v", err)
	}

	frames := trans.getWritten()
	if len(frames) == 0 {
		t.Fatalf("expected AF_DATA_REQUEST frame written")
	}

	lastFrame := frames[len(frames)-1]
	if (lastFrame.cmd0&0x1F) != SubsystemAF || lastFrame.cmd1 != 0x01 {
		t.Errorf("expected AF_DATA_REQUEST (0x2401), got cmd0=0x%02X cmd1=0x%02X", lastFrame.cmd0, lastFrame.cmd1)
	}
	targetNWK := binary.LittleEndian.Uint16(lastFrame.data[0:2])
	if targetNWK != 0x1234 {
		t.Errorf("expected target NWK 0x1234 in AF_DATA_REQUEST, got 0x%04X", targetNWK)
	}
}
