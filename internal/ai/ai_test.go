package ai_test

import (
	"context"
	"testing"
	"time"

	"github.com/julienbreux/zigbridge/internal/ai"
	"github.com/julienbreux/zigbridge/internal/zcl"
)

func TestEventCollectorRingBuffer(t *testing.T) {
	collector := ai.NewEventCollector(3) // small ring buffer of 3 items

	collector.Record(ai.DeviceEvent{ID: "1", IEEE: "0x01"})
	collector.Record(ai.DeviceEvent{ID: "2", IEEE: "0x02"})
	collector.Record(ai.DeviceEvent{ID: "3", IEEE: "0x03"})

	if collector.Count() != 3 {
		t.Fatalf("expected count 3, got %d", collector.Count())
	}

	// Overwrite oldest item ("1")
	collector.Record(ai.DeviceEvent{ID: "4", IEEE: "0x04"})

	if collector.Count() != 3 {
		t.Fatalf("expected count 3 after overflow, got %d", collector.Count())
	}

	recent := collector.RecentEvents(3)
	if len(recent) != 3 {
		t.Fatalf("expected 3 recent events, got %d", len(recent))
	}

	if recent[0].ID != "2" || recent[1].ID != "3" || recent[2].ID != "4" {
		t.Errorf("unexpected event ordering: %+v", recent)
	}
}

func TestRuleBasedAnalyzer(t *testing.T) {
	analyzer := ai.NewRuleBasedAnalyzer(0.70)
	ctx := context.Background()

	// Switch with OnOff output (client)
	sw := ai.DeviceSnapshot{
		IEEE:           "0x00158D0001",
		FriendlyName:   "Living Room Switch",
		OutputClusters: []zcl.ClusterID{zcl.ClusterOnOff},
	}

	// Light with OnOff input (server)
	bulb := ai.DeviceSnapshot{
		IEEE:          "0x00158D0002",
		FriendlyName:  "Living Room Ceiling Light",
		InputClusters: []zcl.ClusterID{zcl.ClusterOnOff},
	}

	topology := ai.NetworkTopology{
		Devices: []ai.DeviceSnapshot{sw, bulb},
	}

	// Correlated events: switch button press followed immediately by light state change
	now := time.Now().UTC()
	events := []ai.DeviceEvent{
		{
			ID:        "e1",
			Timestamp: now,
			IEEE:      sw.IEEE,
			EventType: "command",
			CommandID: zcl.CmdOnOffToggle,
		},
		{
			ID:        "e2",
			Timestamp: now.Add(200 * time.Millisecond),
			IEEE:      bulb.IEEE,
			EventType: "state_change",
			Value:     true,
		},
	}

	recs, err := analyzer.Analyze(ctx, topology, events)
	if err != nil {
		t.Fatalf("unexpected error analyzing: %v", err)
	}

	if len(recs) == 0 {
		t.Fatal("expected at least 1 direct binding recommendation")
	}

	foundBinding := false
	for _, r := range recs {
		if r.Type == ai.TypeDirectBinding && r.SourceIEEE == sw.IEEE && r.TargetIEEE == bulb.IEEE {
			foundBinding = true
			if r.ClusterID != zcl.ClusterOnOff {
				t.Errorf("expected cluster OnOff, got %v", r.ClusterID)
			}
			if r.Confidence < 0.70 {
				t.Errorf("expected confidence >= 0.70, got %f", r.Confidence)
			}
			break
		}
	}

	if !foundBinding {
		t.Errorf("direct binding between %s and %s not found in recommendations: %+v", sw.IEEE, bulb.IEEE, recs)
	}
}

func TestLLMAnalyzerHook(t *testing.T) {
	hook := ai.NewLLMAnalyzerHook("", "")
	ctx := context.Background()

	sw := ai.DeviceSnapshot{
		IEEE:           "0x00158D0001",
		FriendlyName:   "Switch",
		OutputClusters: []zcl.ClusterID{zcl.ClusterOnOff},
	}
	bulb := ai.DeviceSnapshot{
		IEEE:          "0x00158D0002",
		FriendlyName:  "Bulb",
		InputClusters: []zcl.ClusterID{zcl.ClusterOnOff},
	}
	topology := ai.NetworkTopology{Devices: []ai.DeviceSnapshot{sw, bulb}}

	// Mock custom LLM client
	hook.SetCustomClient(func(ctx context.Context, prompt string) (string, error) {
		return `[
			{
				"id": "rec-llm-test",
				"type": "direct_binding",
				"title": "LLM Direct Binding",
				"description": "Direct link test",
				"confidence": 0.96,
				"source_ieee": "0x00158D0001",
				"source_endpoint": 1,
				"target_ieee": "0x00158D0002",
				"target_endpoint": 1,
				"cluster_id": 6,
				"cluster_name": "OnOff"
			}
		]`, nil
	})

	recs, err := hook.Analyze(ctx, topology, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(recs) != 1 || recs[0].ID != "rec-llm-test" {
		t.Fatalf("expected 1 LLM recommendation, got: %+v", recs)
	}
}
