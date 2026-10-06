package mock

import (
	"context"
	"sync"
	"time"

	"github.com/julienbreux/zigbridge/internal/adapter"
	"github.com/julienbreux/zigbridge/internal/fixture"
	"github.com/julienbreux/zigbridge/internal/transport"
	"github.com/julienbreux/zigbridge/internal/zcl"
)

// MockAdapter simulates an adapter with mock devices and simulated events.
type MockAdapter struct {
	mu             sync.RWMutex
	info           adapter.AdapterInfo
	frameHandler   adapter.FrameHandler
	joinHandler    adapter.DeviceJoinHandler
	bindings       []adapter.BindRequest
	running        bool
	permitJoin     uint8
	virtualDevices map[string]*fixture.VirtualDevice
}

// New creates a new MockAdapter.
func New(channel uint8, panID uint16) *MockAdapter {
	return &MockAdapter{
		info: adapter.AdapterInfo{
			Type:     "Virtual Mock Adapter",
			Version:  "1.0.0 (Simulated)",
			Channel:  channel,
			PanID:    panID,
			ExtPanID: "0xFEEDFACECAFEBEEF",
			IEEE:     "0x00124B0014D8B954",
			Status:   "ready",
		},
		bindings:       make([]adapter.BindRequest, 0),
		virtualDevices: make(map[string]*fixture.VirtualDevice),
	}
}

func (m *MockAdapter) Init(ctx context.Context, t transport.Transport) error {
	return nil
}

func (m *MockAdapter) Start(ctx context.Context) error {
	m.mu.Lock()
	m.running = true
	m.info.Status = "running"
	m.mu.Unlock()

	// Seed some initial mock devices after a brief delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		m.EmitDeviceJoin(adapter.DeviceJoinInfo{
			IEEE:         "0x00158D0001928374",
			NWK:          0x1234,
			Capabilities: 0x8E,
		})
		m.EmitDeviceJoin(adapter.DeviceJoinInfo{
			IEEE:         "0x00158D0005A1B2C3",
			NWK:          0x5678,
			Capabilities: 0x8E,
		})
	}()

	return nil
}

func (m *MockAdapter) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = false
	m.info.Status = "stopped"
	return nil
}

func (m *MockAdapter) PermitJoin(ctx context.Context, duration uint8) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.permitJoin = duration
	return nil
}

func (m *MockAdapter) Reset(ctx context.Context) error {
	return nil
}

func (m *MockAdapter) SendZCL(ctx context.Context, frame *zcl.Frame) error {
	return nil
}

func (m *MockAdapter) Bind(ctx context.Context, req adapter.BindRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bindings = append(m.bindings, req)
	return nil
}

func (m *MockAdapter) Unbind(ctx context.Context, req adapter.BindRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	filtered := make([]adapter.BindRequest, 0, len(m.bindings))
	for _, b := range m.bindings {
		if b.SrcIEEE == req.SrcIEEE && b.SrcEndpoint == req.SrcEndpoint &&
			b.ClusterID == req.ClusterID && b.DstIEEE == req.DstIEEE && b.DstEndpoint == req.DstEndpoint {
			continue
		}
		filtered = append(filtered, b)
	}
	m.bindings = filtered
	return nil
}

func (m *MockAdapter) RegisterFrameHandler(handler adapter.FrameHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.frameHandler = handler
}

func (m *MockAdapter) RegisterDeviceJoinHandler(handler adapter.DeviceJoinHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.joinHandler = handler
}

func (m *MockAdapter) Info() adapter.AdapterInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.info
}

// EmitFrame allows tests to inject simulated incoming ZCL frames.
func (m *MockAdapter) EmitFrame(frame *zcl.Frame) {
	m.mu.RLock()
	handler := m.frameHandler
	m.mu.RUnlock()
	if handler != nil {
		handler(frame)
	}
}

// EmitDeviceJoin allows tests to inject simulated device joins.
func (m *MockAdapter) EmitDeviceJoin(info adapter.DeviceJoinInfo) {
	m.mu.RLock()
	handler := m.joinHandler
	m.mu.RUnlock()
	if handler != nil {
		handler(info)
	}
}

// GetBindings returns all active bindings stored in mock.
func (m *MockAdapter) GetBindings() []adapter.BindRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := make([]adapter.BindRequest, len(m.bindings))
	copy(cp, m.bindings)
	return cp
}

// SpawnVirtualDevice creates and registers a simulated device, triggering join and identity announcement.
func (m *MockAdapter) SpawnVirtualDevice(def *fixture.DeviceDefinition, ieee string, nwk uint16) (*fixture.VirtualDevice, error) {
	vdev, err := fixture.Spawn(def, ieee, nwk, m)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.virtualDevices[ieee] = vdev
	m.mu.Unlock()

	return vdev, nil
}

// GetVirtualDevices returns all currently active virtual devices.
func (m *MockAdapter) GetVirtualDevices() []*fixture.VirtualDevice {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*fixture.VirtualDevice, 0, len(m.virtualDevices))
	for _, v := range m.virtualDevices {
		res = append(res, v)
	}
	return res
}

// GetVirtualDevice retrieves a virtual device by IEEE address.
func (m *MockAdapter) GetVirtualDevice(ieee string) (*fixture.VirtualDevice, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.virtualDevices[ieee]
	return v, ok
}

