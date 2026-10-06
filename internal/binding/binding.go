package binding

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/julienbreux/zigbridge/internal/adapter"
	"github.com/julienbreux/zigbridge/internal/zcl"
)

var (
	ErrBindingNotFound = errors.New("binding: not found")
	ErrBindingExists   = errors.New("binding: already exists")
	ErrInvalidRequest  = errors.New("binding: invalid source or destination")
)

// Binding represents an active direct Zigbee link between two devices.
type Binding struct {
	ID          string        `json:"id" yaml:"id"`
	SrcIEEE     string        `json:"src_ieee" yaml:"src_ieee"`
	SrcEndpoint uint8         `json:"src_endpoint" yaml:"src_endpoint"`
	ClusterID   zcl.ClusterID `json:"cluster_id" yaml:"cluster_id"`
	ClusterName string        `json:"cluster_name" yaml:"cluster_name"`
	DstIEEE     string        `json:"dst_ieee" yaml:"dst_ieee"`
	DstEndpoint uint8         `json:"dst_endpoint" yaml:"dst_endpoint"`
	Status      string        `json:"status" yaml:"status"` // "active", "failed"
	CreatedAt   time.Time     `json:"created_at" yaml:"created_at"`
}

// GenerateID produces a deterministic identifier for a binding.
func GenerateID(srcIEEE string, srcEP uint8, cluster zcl.ClusterID, dstIEEE string, dstEP uint8) string {
	return fmt.Sprintf("%s:%d->0x%04X->%s:%d", srcIEEE, srcEP, uint16(cluster), dstIEEE, dstEP)
}

// Engine manages direct Zigbee device bindings and hardware synchronization.
type Engine struct {
	adapter adapter.Adapter

	mu       sync.RWMutex
	bindings map[string]*Binding
	onChange func(b *Binding, action string)
}

// NewEngine creates an extensible binding engine backed by a Zigbee radio adapter.
func NewEngine(adp adapter.Adapter) *Engine {
	return &Engine{
		adapter:  adp,
		bindings: make(map[string]*Binding),
	}
}

// SetChangeListener registers a callback for binding mutations.
func (e *Engine) SetChangeListener(fn func(b *Binding, action string)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onChange = fn
}

// RestoreBinding populates an existing binding into the registry without issuing adapter commands.
func (e *Engine) RestoreBinding(b *Binding) {
	if b == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.bindings[b.ID] = b
}

// CreateBinding requests the adapter to bind two endpoints directly and records the link.
func (e *Engine) CreateBinding(ctx context.Context, req adapter.BindRequest) (*Binding, error) {
	if req.SrcIEEE == "" || req.DstIEEE == "" {
		return nil, ErrInvalidRequest
	}

	id := GenerateID(req.SrcIEEE, req.SrcEndpoint, req.ClusterID, req.DstIEEE, req.DstEndpoint)

	e.mu.RLock()
	if _, exists := e.bindings[id]; exists {
		e.mu.RUnlock()
		return nil, ErrBindingExists
	}
	e.mu.RUnlock()

	// Instruct coordinator radio to perform ZDO Bind Request
	if err := e.adapter.Bind(ctx, req); err != nil {
		return nil, fmt.Errorf("radio adapter failed to bind: %w", err)
	}

	b := &Binding{
		ID:          id,
		SrcIEEE:     req.SrcIEEE,
		SrcEndpoint: req.SrcEndpoint,
		ClusterID:   req.ClusterID,
		ClusterName: req.ClusterID.String(),
		DstIEEE:     req.DstIEEE,
		DstEndpoint: req.DstEndpoint,
		Status:      "active",
		CreatedAt:   time.Now().UTC(),
	}

	e.mu.Lock()
	e.bindings[id] = b
	listener := e.onChange
	e.mu.Unlock()

	if listener != nil {
		listener(b, "created")
	}

	return b, nil
}

// RemoveBinding removes a direct binding from the hardware and local table.
func (e *Engine) RemoveBinding(ctx context.Context, req adapter.BindRequest) error {
	id := GenerateID(req.SrcIEEE, req.SrcEndpoint, req.ClusterID, req.DstIEEE, req.DstEndpoint)

	e.mu.Lock()
	b, exists := e.bindings[id]
	if !exists {
		e.mu.Unlock()
		return ErrBindingNotFound
	}
	delete(e.bindings, id)
	listener := e.onChange
	e.mu.Unlock()

	// Instruct radio to unbind
	if err := e.adapter.Unbind(ctx, req); err != nil {
		return fmt.Errorf("radio adapter unbind warning: %w", err)
	}

	if listener != nil {
		listener(b, "removed")
	}

	return nil
}

// ListBindings returns a snapshot of all active bindings.
func (e *Engine) ListBindings() []*Binding {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return slices.Collect(maps.Values(e.bindings))
}

// GetBinding looks up a binding by its identifier.
func (e *Engine) GetBinding(id string) (*Binding, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	b, ok := e.bindings[id]
	return b, ok
}
