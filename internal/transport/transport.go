package transport

import (
	"context"
	"errors"
	"io"
)

// Common transport errors.
var (
	ErrNotConnected     = errors.New("transport is not connected")
	ErrAlreadyConnected = errors.New("transport is already connected")
	ErrClosed           = errors.New("transport is closed")
)

// ConnectionStatus indicates the live state of the transport medium.
type ConnectionStatus string

const (
	StatusDisconnected ConnectionStatus = "disconnected"
	StatusConnecting   ConnectionStatus = "connecting"
	StatusConnected    ConnectionStatus = "connected"
	StatusReconnecting ConnectionStatus = "reconnecting"
)

// StatusListener receives notifications on connection status changes.
type StatusListener func(status ConnectionStatus, err error)

// Transport defines the common bidirectional byte stream interface
// used by radio adapters to communicate with the coordinator.
type Transport interface {
	io.ReadWriteCloser

	// Open establishes the initial connection to the coordinator.
	Open(ctx context.Context) error

	// IsConnected reports whether the transport is active and operational.
	IsConnected() bool

	// Status returns the current connection status.
	Status() ConnectionStatus

	// SetStatusListener registers a callback invoked when status changes.
	SetStatusListener(listener StatusListener)
}
