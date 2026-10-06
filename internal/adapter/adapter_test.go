package adapter_test

import (
	"testing"

	"github.com/julienbreux/zigbridge/internal/adapter"
	"github.com/julienbreux/zigbridge/internal/adapter/mock"
	"github.com/julienbreux/zigbridge/internal/adapter/zstack"
	"github.com/julienbreux/zigbridge/internal/zcl"
)

func TestZStackFCS(t *testing.T) {
	// Len=1, Cmd0=0x21, Cmd1=0x02, Data=[0x00] -> FCS = 1 ^ 0x21 ^ 0x02 = 0x22
	fcs := zstack.CalculateFCS(1, 0x21, 0x02, []byte{0x00})
	expected := byte(1 ^ 0x21 ^ 0x02)
	if fcs != expected {
		t.Errorf("expected FCS 0x%02X, got 0x%02X", expected, fcs)
	}
}

func TestMockAdapterLifecycleAndBinding(t *testing.T) {
	mockAdapter := mock.New(20, 0x1A62)
	t.Cleanup(func() {
		_ = mockAdapter.Stop()
	})
	ctx := t.Context()

	if err := mockAdapter.Start(ctx); err != nil {
		t.Fatalf("failed to start mock adapter: %v", err)
	}

	info := mockAdapter.Info()
	if info.Channel != 20 {
		t.Errorf("expected channel 20, got %d", info.Channel)
	}
	if info.PanID != 0x1A62 {
		t.Errorf("expected PanID 0x1A62, got 0x%X", info.PanID)
	}

	req := adapter.BindRequest{
		SrcIEEE:     "0x00158D0001928374",
		SrcEndpoint: 1,
		ClusterID:   zcl.ClusterOnOff,
		DstIEEE:     "0x00158D0005A1B2C3",
		DstEndpoint: 1,
	}

	if err := mockAdapter.Bind(ctx, req); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	bindings := mockAdapter.GetBindings()
	if len(bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(bindings))
	}
	if bindings[0].ClusterID != zcl.ClusterOnOff {
		t.Errorf("expected cluster OnOff, got %v", bindings[0].ClusterID)
	}

	if err := mockAdapter.Unbind(ctx, req); err != nil {
		t.Fatalf("failed to unbind: %v", err)
	}

	bindings = mockAdapter.GetBindings()
	if len(bindings) != 0 {
		t.Errorf("expected 0 bindings after unbind, got %d", len(bindings))
	}
}
