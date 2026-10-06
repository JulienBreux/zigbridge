package transport

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
)

// MockTransport simulates a transport using an in-memory network pipe.
type MockTransport struct {
	clientConn net.Conn
	serverConn net.Conn

	mu        sync.RWMutex
	status    ConnectionStatus
	listener  StatusListener
	connected atomic.Bool
	closed    atomic.Bool
}

// NewMockTransport creates a paired mock transport.
// clientConn is what the adapter reads/writes.
// serverConn can be used by tests to simulate coordinator responses.
func NewMockTransport() (*MockTransport, net.Conn) {
	c1, c2 := net.Pipe()
	m := &MockTransport{
		clientConn: c1,
		serverConn: c2,
		status:     StatusDisconnected,
	}
	return m, c2
}

func (m *MockTransport) Open(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.connected.Load() {
		return ErrAlreadyConnected
	}

	m.connected.Store(true)
	m.closed.Store(false)
	m.status = StatusConnected
	if m.listener != nil {
		go m.listener(StatusConnected, nil)
	}
	return nil
}

func (m *MockTransport) Read(p []byte) (int, error) {
	if m.closed.Load() {
		return 0, ErrClosed
	}
	if !m.connected.Load() {
		return 0, ErrNotConnected
	}
	return m.clientConn.Read(p)
}

func (m *MockTransport) Write(p []byte) (int, error) {
	if m.closed.Load() {
		return 0, ErrClosed
	}
	if !m.connected.Load() {
		return 0, ErrNotConnected
	}
	return m.clientConn.Write(p)
}

func (m *MockTransport) Close() error {
	m.closed.Store(true)
	m.connected.Store(false)

	m.mu.Lock()
	m.status = StatusDisconnected
	if m.listener != nil {
		go m.listener(StatusDisconnected, nil)
	}
	m.mu.Unlock()

	_ = m.clientConn.Close()
	_ = m.serverConn.Close()
	return nil
}

func (m *MockTransport) IsConnected() bool {
	return m.connected.Load()
}

func (m *MockTransport) Status() ConnectionStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *MockTransport) SetStatusListener(listener StatusListener) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listener = listener
}
