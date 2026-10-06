package transport

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

func TestMockTransportReadWrite(t *testing.T) {
	mock, server := NewMockTransport()
	ctx := context.Background()

	if err := mock.Open(ctx); err != nil {
		t.Fatalf("failed to open mock transport: %v", err)
	}

	if !mock.IsConnected() {
		t.Error("expected connected status")
	}

	// Test writing from client to server
	go func() {
		_, _ = mock.Write([]byte("ping"))
	}()

	buf := make([]byte, 16)
	n, err := server.Read(buf)
	if err != nil {
		t.Fatalf("failed to read on server: %v", err)
	}
	if string(buf[:n]) != "ping" {
		t.Errorf("expected 'ping', got '%s'", string(buf[:n]))
	}

	// Test writing from server to client
	go func() {
		_, _ = server.Write([]byte("pong"))
	}()

	n, err = mock.Read(buf)
	if err != nil {
		t.Fatalf("failed to read on mock client: %v", err)
	}
	if string(buf[:n]) != "pong" {
		t.Errorf("expected 'pong', got '%s'", string(buf[:n]))
	}

	_ = mock.Close()
	if mock.IsConnected() {
		t.Error("expected disconnected after close")
	}
}

func TestTCPTransportRFC2217Filter(t *testing.T) {
	cfg := TCPConfig{
		Address: "127.0.0.1:0",
		RFC2217: true,
	}
	tcp := NewTCPTransport(cfg)

	// Stream with Telnet IAC sequences mixed with Zigbee data
	// 0xFE (SOF) 0x01 (Len) 0xFF 0xFB 0x01 (IAC WILL ECHO) 0x21 (Cmd) 0x00 0xFE
	input := []byte{0xFE, 0x01, 0xFF, 0xFB, 0x01, 0x21, 0x00, 0xFE}
	filteredLen := tcp.filterRFC2217(input)
	filtered := input[:filteredLen]

	expected := []byte{0xFE, 0x01, 0x21, 0x00, 0xFE}
	if len(filtered) != len(expected) {
		t.Fatalf("expected filtered length %d, got %d (%v)", len(expected), len(filtered), filtered)
	}

	for i := range expected {
		if filtered[i] != expected[i] {
			t.Errorf("at index %d: expected 0x%02X, got 0x%02X", i, expected[i], filtered[i])
		}
	}
}

func TestTCPTransportConnectAndReconnect(t *testing.T) {
	// Start a local TCP listener to act as SMLIGHT SLZB-06
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	addr := listener.Addr().String()

	cfg := TCPConfig{
		Address:              addr,
		KeepAlive:            1 * time.Second,
		ReconnectInterval:    100 * time.Millisecond,
		MaxReconnectInterval: 500 * time.Millisecond,
		RFC2217:              true,
		ReadTimeout:          1 * time.Second,
		WriteTimeout:         1 * time.Second,
	}

	transport := NewTCPTransport(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		buf := make([]byte, 32)
		n, _ := conn.Read(buf)
		if n > 0 {
			_, _ = conn.Write(append([]byte("ACK:"), buf[:n]...))
		}
	}()

	if err := transport.Open(ctx); err != nil {
		t.Fatalf("failed to open TCP transport: %v", err)
	}

	if !transport.IsConnected() {
		t.Error("expected connected status")
	}

	_, err = transport.Write([]byte("HELLO"))
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	reply := make([]byte, 32)
	n, err := transport.Read(reply)
	if err != nil {
		t.Fatalf("failed to read reply: %v", err)
	}

	if string(reply[:n]) != "ACK:HELLO" {
		t.Errorf("expected 'ACK:HELLO', got '%s'", string(reply[:n]))
	}

	_ = transport.Close()
	wg.Wait()
}
