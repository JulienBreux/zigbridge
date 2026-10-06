package zcl

import (
	"sync"
)

const (
	// DefaultBufferSize is sized to accommodate maximum 802.15.4 / Zigbee payload (127 bytes)
	DefaultBufferSize = 256
)

// BufferPool maintains reusable byte slices to eliminate heap churn during frame processing.
type BufferPool struct {
	pool sync.Pool
}

// GlobalPool is the default buffer pool for packet operations.
var GlobalPool = NewBufferPool(DefaultBufferSize)

// NewBufferPool creates a new pool with fixed capacity slices.
func NewBufferPool(size int) *BufferPool {
	return &BufferPool{
		pool: sync.Pool{
			New: func() any {
				b := make([]byte, size)
				return &b
			},
		},
	}
}

// Get retrieves a preallocated byte slice from the pool.
func (p *BufferPool) Get() []byte {
	bufPtr := p.pool.Get().(*[]byte)
	return (*bufPtr)[:0]
}

// Put returns a slice to the pool.
func (p *BufferPool) Put(b []byte) {
	if cap(b) < DefaultBufferSize {
		return
	}
	// Reset length and slice
	b = b[:cap(b)]
	p.pool.Put(&b)
}
