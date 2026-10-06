package transport

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.bug.st/serial"
)

// SerialConfig defines serial connection parameters.
type SerialConfig struct {
	Port         string        // e.g. "/dev/ttyUSB0"
	BaudRate     int           // e.g. 115200
	DataBits     int           // e.g. 8
	StopBits     serial.StopBits
	Parity       serial.Parity
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// SerialTransport implements Transport for local serial / USB coordinators.
type SerialTransport struct {
	config SerialConfig

	mu        sync.RWMutex
	port      serial.Port
	status    ConnectionStatus
	listener  StatusListener
	connected atomic.Bool
	closed    atomic.Bool
}

// NewSerialTransport creates a new SerialTransport instance.
func NewSerialTransport(cfg SerialConfig) *SerialTransport {
	if cfg.BaudRate <= 0 {
		cfg.BaudRate = 115200
	}
	if cfg.DataBits <= 0 {
		cfg.DataBits = 8
	}
	if cfg.ReadTimeout <= 0 {
		cfg.ReadTimeout = 1 * time.Second
	}

	return &SerialTransport{
		config: cfg,
		status: StatusDisconnected,
	}
}

// Open opens the configured serial port.
func (s *SerialTransport) Open(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.connected.Load() {
		return ErrAlreadyConnected
	}

	mode := &serial.Mode{
		BaudRate: s.config.BaudRate,
		DataBits: s.config.DataBits,
		Parity:   s.config.Parity,
		StopBits: s.config.StopBits,
	}

	port, err := serial.Open(s.config.Port, mode)
	if err != nil {
		s.updateStatusLocked(StatusDisconnected, err)
		return fmt.Errorf("failed to open serial port %s: %w", s.config.Port, err)
	}

	// Set read timeout
	if err := port.SetReadTimeout(s.config.ReadTimeout); err != nil {
		_ = port.Close()
		return fmt.Errorf("failed to set read timeout on %s: %w", s.config.Port, err)
	}

	// Flush any initial garbage bytes
	_ = port.ResetInputBuffer()
	_ = port.ResetOutputBuffer()

	s.port = port
	s.connected.Store(true)
	s.closed.Store(false)
	s.updateStatusLocked(StatusConnected, nil)

	return nil
}

// Read reads data from the serial port.
func (s *SerialTransport) Read(p []byte) (int, error) {
	if s.closed.Load() {
		return 0, ErrClosed
	}

	s.mu.RLock()
	port := s.port
	s.mu.RUnlock()

	if port == nil || !s.connected.Load() {
		return 0, ErrNotConnected
	}

	n, err := port.Read(p)
	if err != nil {
		s.handleDisconnect(err)
		return n, err
	}

	return n, nil
}

// Write writes data to the serial port.
func (s *SerialTransport) Write(p []byte) (int, error) {
	if s.closed.Load() {
		return 0, ErrClosed
	}

	s.mu.RLock()
	port := s.port
	s.mu.RUnlock()

	if port == nil || !s.connected.Load() {
		return 0, ErrNotConnected
	}

	n, err := port.Write(p)
	if err != nil {
		s.handleDisconnect(err)
		return n, err
	}

	return n, nil
}

// Close closes the serial port.
func (s *SerialTransport) Close() error {
	s.closed.Store(true)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.connected.Store(false)
	var err error
	if s.port != nil {
		err = s.port.Close()
		s.port = nil
	}
	s.updateStatusLocked(StatusDisconnected, nil)
	return err
}

func (s *SerialTransport) handleDisconnect(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.port != nil {
		_ = s.port.Close()
		s.port = nil
	}
	s.connected.Store(false)
	s.updateStatusLocked(StatusDisconnected, err)
}

// IsConnected returns whether the serial port is currently open.
func (s *SerialTransport) IsConnected() bool {
	return s.connected.Load()
}

// Status returns current connection status.
func (s *SerialTransport) Status() ConnectionStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// SetStatusListener registers a state change callback.
func (s *SerialTransport) SetStatusListener(listener StatusListener) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listener = listener
}

func (s *SerialTransport) updateStatusLocked(newStatus ConnectionStatus, err error) {
	if s.status == newStatus {
		return
	}
	s.status = newStatus
	if s.listener != nil {
		go s.listener(newStatus, err)
	}
}
