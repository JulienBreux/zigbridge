package binding_test

import (
	"testing"

	"github.com/julienbreux/zigbridge/internal/adapter"
	"github.com/julienbreux/zigbridge/internal/adapter/mock"
	"github.com/julienbreux/zigbridge/internal/binding"
	"github.com/julienbreux/zigbridge/internal/zcl"
)

func TestBindingEngine(t *testing.T) {
	mockAdapter := mock.New(20, 0x1A62)
	engine := binding.NewEngine(mockAdapter)
	ctx := t.Context()

	changedCount := 0
	engine.SetChangeListener(func(b *binding.Binding, action string) {
		changedCount++
	})

	req := adapter.BindRequest{
		SrcIEEE:     "0x00158D0001",
		SrcEndpoint: 1,
		ClusterID:   zcl.ClusterOnOff,
		DstIEEE:     "0x00158D0002",
		DstEndpoint: 1,
	}

	b, err := engine.CreateBinding(ctx, req)
	if err != nil {
		t.Fatalf("failed to create binding: %v", err)
	}

	if b.ClusterName != "OnOff" {
		t.Errorf("expected OnOff cluster name, got %s", b.ClusterName)
	}

	list := engine.ListBindings()
	if len(list) != 1 {
		t.Fatalf("expected 1 binding in list, got %d", len(list))
	}

	// Duplicate creation should fail
	_, err = engine.CreateBinding(ctx, req)
	if err == nil {
		t.Error("expected error creating duplicate binding, got nil")
	}

	// Remove binding
	if err := engine.RemoveBinding(ctx, req); err != nil {
		t.Fatalf("failed to remove binding: %v", err)
	}

	if len(engine.ListBindings()) != 0 {
		t.Errorf("expected 0 bindings after removal, got %d", len(engine.ListBindings()))
	}

	if changedCount != 2 {
		t.Errorf("expected 2 change events (created, removed), got %d", changedCount)
	}
}
