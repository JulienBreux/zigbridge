package transport

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// TCPConfig holds options for the TCP/RFC2217 transport.
type TCPConfig struct {
	Address              string        // e.g. "tcp://192.168.1.50:6638" or "192.168.1.50:6638"
	KeepAlive            time.Duration // Probe frequency (e.g. 10s)
	ReconnectInterval    time.Duration // Initial retry delay
	MaxReconnectInterval time.Duration // Maximum retry delay
	RFC2217              bool          // Strip/negotiate Telnet RFC2217 sequences
	ReadTimeout          time.Duration
	WriteTimeout         time.Duration
}

// TCPTransport implements Transport over network sockets, specifically optimized
// for network coordinators like SMLIGHT SLZB-06, TubeZB, and ZigStar.
type TCPTransport struct {
	config TCPConfig

	mu       sync.RWMutex
	conn     net.Conn
	status   ConnectionStatus
	listener StatusListener

	ctx       context.Context
	cancel    context.CancelFunc
	closed    atomic.Bool
	connected atomic.Bool

	// RFC2217 filter state machine
	iacBuffer []byte
}

// NewTCPTransport creates a new TCP/RFC2217 transport instance.
func NewTCPTransport(cfg TCPConfig) *TCPTransport {
	if cfg.KeepAlive <= 0 {
		cfg.KeepAlive = 10 * time.Second
	}
	if cfg.ReconnectInterval <= 0 {
		cfg.ReconnectInterval = 2 * time.Second
	}
	if cfg.MaxReconnectInterval <= 0 {
		cfg.MaxReconnectInterval = 30 * time.Second
	}

	// Normalize address
	addr := strings.TrimPrefix(cfg.Address, "tcp://")
	cfg.Address = addr

	return &TCPTransport{
		config:    cfg,
		status:    StatusDisconnected,
		iacBuffer: make([]byte, 0, 16),
	}
}

// Open initiates the network connection and starts the supervision/reconnect loop.
func (t *TCPTransport) Open(ctx context.Context) error {
	t.mu.Lock()
	if t.connected.Load() {
		t.mu.Unlock()
		return ErrAlreadyConnected
	}

	t.ctx, t.cancel = context.WithCancel(ctx)
	t.closed.Store(false)
	t.updateStatusLocked(StatusConnecting, nil)
	t.mu.Unlock()

	// Initial connection attempt synchronously
	if err := t.connect(); err != nil {
		t.mu.Lock()
		t.updateStatusLocked(StatusReconnecting, err)
		t.mu.Unlock()
		go t.reconnectLoop()
		return fmt.Errorf("initial tcp connection to %s failed (reconnecting in background): %w", t.config.Address, err)
	}

	go t.superviseLoop()
	return nil
}

func (t *TCPTransport) connect() error {
	dialer := net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: t.config.KeepAlive,
	}

	conn, err := dialer.DialContext(t.ctx, "tcp", t.config.Address)
	if err != nil {
		return err
	}

	// Tune TCP keepalive on underlying TCPConn
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetKeepAlive(true)
		_ = tcpConn.SetKeepAlivePeriod(t.config.KeepAlive)
		_ = tcpConn.SetNoDelay(true) // Minimal latency for Zigbee frames
	}

	t.mu.Lock()
	t.conn = conn
	t.connected.Store(true)
	t.updateStatusLocked(StatusConnected, nil)
	t.mu.Unlock()

	return nil
}

// reconnectLoop handles exponential backoff reconnection for network coordinators.
func (t *TCPTransport) reconnectLoop() {
	backoff := t.config.ReconnectInterval
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-time.After(backoff):
		}

		if t.closed.Load() {
			return
		}

		t.mu.Lock()
		t.updateStatusLocked(StatusReconnecting, nil)
		t.mu.Unlock()

		err := t.connect()
		if err == nil {
			go t.superviseLoop()
			return
		}

		// Calculate jittered exponential backoff
		backoff = time.Duration(float64(backoff) * 1.5)
		jitter := time.Duration(rng.Int63n(int64(backoff) / 4))
		backoff += jitter
		if backoff > t.config.MaxReconnectInterval {
			backoff = t.config.MaxReconnectInterval
		}
	}
}

// superviseLoop monitors active connection health and triggers reconnect on drop.
func (t *TCPTransport) superviseLoop() {
	// Loop exits when connection closes or context ends
	<-t.ctx.Done()
}

func (t *TCPTransport) handleDisconnect(err error) {
	if t.closed.Load() {
		return
	}

	t.mu.Lock()
	if t.conn != nil {
		_ = t.conn.Close()
		t.conn = nil
	}
	t.connected.Store(false)
	t.updateStatusLocked(StatusDisconnected, err)
	t.mu.Unlock()

	if !t.closed.Load() {
		go t.reconnectLoop()
	}
}

// Read reads raw or RFC2217-filtered bytes from the network coordinator.
func (t *TCPTransport) Read(p []byte) (int, error) {
	if t.closed.Load() {
		return 0, ErrClosed
	}

	t.mu.RLock()
	conn := t.conn
	t.mu.RUnlock()

	if conn == nil || !t.connected.Load() {
		return 0, ErrNotConnected
	}

	if t.config.ReadTimeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(t.config.ReadTimeout))
	}

	n, err := conn.Read(p)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return 0, err
		}
		if err == io.EOF || isConnectionError(err) {
			t.handleDisconnect(err)
		}
		return n, err
	}

	if t.config.RFC2217 && n > 0 {
		n = t.filterRFC2217(p[:n])
	}

	return n, nil
}

// filterRFC2217 strips Telnet IAC command byte sequences (0xFF ...) from stream
// while preserving valid binary payload bytes containing 0xFF.
func (t *TCPTransport) filterRFC2217(buf []byte) int {
	outIdx := 0
	i := 0
	for i < len(buf) {
		b := buf[i]
		if b == 0xFF { // Possible Telnet IAC (Interpret As Command)
			if i+1 < len(buf) {
				cmd := buf[i+1]
				if cmd == 0xFF {
					// Escaped 0xFF: keep single 0xFF
					buf[outIdx] = 0xFF
					outIdx++
					i += 2
					continue
				}
				// 3-byte Telnet negotiation commands: WILL(0xFB), WONT(0xFC), DO(0xFD), DONT(0xFE)
				if cmd >= 0xFB && cmd <= 0xFE {
					if i+2 < len(buf) {
						// Skip entire 3-byte negotiation sequence
						i += 3
						continue
					}
					// Partial negotiation command at boundary: retain byte
					buf[outIdx] = b
					outIdx++
					i++
					continue
				}
				// If not followed by standard Telnet verb, treat 0xFF as binary payload data
				buf[outIdx] = b
				outIdx++
				i++
				continue
			}
			// Single 0xFF byte at boundary: retain as binary payload data
			buf[outIdx] = b
			outIdx++
			i++
			continue
		}
		buf[outIdx] = b
		outIdx++
		i++
	}
	return outIdx
}

// Write sends bytes to the coordinator.
func (t *TCPTransport) Write(p []byte) (int, error) {
	if t.closed.Load() {
		return 0, ErrClosed
	}

	t.mu.RLock()
	conn := t.conn
	t.mu.RUnlock()

	if conn == nil || !t.connected.Load() {
		return 0, ErrNotConnected
	}

	if t.config.WriteTimeout > 0 {
		_ = conn.SetWriteDeadline(time.Now().Add(t.config.WriteTimeout))
	}

	n, err := conn.Write(p)
	if err != nil {
		if isConnectionError(err) {
			t.handleDisconnect(err)
		}
		return n, err
	}

	return n, nil
}

// Close gracefully closes the connection and terminates the supervisor/reconnect loop.
func (t *TCPTransport) Close() error {
	t.closed.Store(true)
	if t.cancel != nil {
		t.cancel()
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.connected.Store(false)
	var err error
	if t.conn != nil {
		err = t.conn.Close()
		t.conn = nil
	}
	t.updateStatusLocked(StatusDisconnected, nil)
	return err
}

// IsConnected returns whether the TCP socket is currently established.
func (t *TCPTransport) IsConnected() bool {
	return t.connected.Load()
}

// Status returns the current connection status.
func (t *TCPTransport) Status() ConnectionStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.status
}

// SetStatusListener registers a callback invoked on connection state transitions.
func (t *TCPTransport) SetStatusListener(listener StatusListener) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.listener = listener
}

func (t *TCPTransport) updateStatusLocked(newStatus ConnectionStatus, err error) {
	if t.status == newStatus {
		return
	}
	t.status = newStatus
	if t.listener != nil {
		go t.listener(newStatus, err)
	}
}

func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "EOF")
}
