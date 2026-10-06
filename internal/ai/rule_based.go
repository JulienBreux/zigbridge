package ai

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/julienbreux/zigbridge/internal/zcl"
)

// RuleBasedAnalyzer implements fast, deterministic heuristic pattern recognition.
type RuleBasedAnalyzer struct {
	MinConfidence float64
	WindowDelta   time.Duration
}

// NewRuleBasedAnalyzer constructs a rule-based analyzer with default thresholds.
func NewRuleBasedAnalyzer(minConfidence float64) *RuleBasedAnalyzer {
	if minConfidence <= 0 {
		minConfidence = 0.70
	}
	return &RuleBasedAnalyzer{
		MinConfidence: minConfidence,
		WindowDelta:   2 * time.Second,
	}
}

// Analyze inspects devices and event logs to generate smart recommendations.
func (r *RuleBasedAnalyzer) Analyze(ctx context.Context, topology NetworkTopology, events []DeviceEvent) ([]Recommendation, error) {
	var recommendations []Recommendation

	// Index existing bindings to avoid duplicate suggestions
	boundMap := make(map[string]bool)
	for _, b := range topology.ActiveBindings {
		key := fmt.Sprintf("%s:%d->%s:%d", b.SrcIEEE, b.SrcEndpoint, b.DstIEEE, b.DstEndpoint)
		boundMap[key] = true
	}

	// 1. Cluster Capability Analysis (Source with Output Cluster + Target with Input Cluster)
	for _, src := range topology.Devices {
		for _, cluster := range src.OutputClusters {
			if cluster != zcl.ClusterOnOff && cluster != zcl.ClusterLevelControl {
				continue
			}

			for _, dst := range topology.Devices {
				if src.IEEE == dst.IEEE {
					continue
				}

				// Check if destination supports the cluster as input (server)
				hasInput := slices.Contains(dst.InputClusters, cluster)

				if !hasInput {
					continue
				}

				key := fmt.Sprintf("%s:1->%s:1", src.IEEE, dst.IEEE)
				if boundMap[key] {
					continue
				}

				// Check event history for correlation
				correlationScore := r.evaluateCorrelation(src.IEEE, dst.IEEE, events)
				confidence := min(0.75+(correlationScore*0.20), 0.98)

				if confidence >= r.MinConfidence {
					recID := fmt.Sprintf("rec-bind-%s-%s-%04X", src.IEEE, dst.IEEE, uint16(cluster))
					rec := Recommendation{
						ID:                   recID,
						Type:                 TypeDirectBinding,
						Title:                fmt.Sprintf("Direct Binding: %s to %s (%s)", friendly(src), friendly(dst), cluster.String()),
						Description:          fmt.Sprintf("Bind %s directly to %s on %s cluster. Enables instant local response without coordinator hop.", friendly(src), friendly(dst), cluster.String()),
						Confidence:           confidence,
						SourceIEEE:           src.IEEE,
						SourceEndpoint:       1,
						TargetIEEE:           dst.IEEE,
						TargetEndpoint:       1,
						ClusterID:            cluster,
						ClusterName:          cluster.String(),
						Status:               StatusPending,
						EstimatedLatencyDrop: 120, // ~120ms typical latency saved
						ReliabilityBenefit:   "Functions completely offline even if bridge or home automation server is rebooting.",
						CreatedAt:            time.Now().UTC(),
					}
					recommendations = append(recommendations, rec)
				}
			}
		}
	}

	// 2. Automated Scene Discovery (detecting groups of lights turned on together)
	sceneRec := r.detectGroupedScene(topology, events)
	if sceneRec != nil && sceneRec.Confidence >= r.MinConfidence {
		recommendations = append(recommendations, *sceneRec)
	}

	return recommendations, nil
}

func (r *RuleBasedAnalyzer) evaluateCorrelation(srcIEEE, dstIEEE string, events []DeviceEvent) float64 {
	if len(events) < 2 {
		return 0.0
	}

	correlations := 0
	for i := range len(events) - 1 {
		e1 := events[i]
		if e1.IEEE != srcIEEE {
			continue
		}

		for j := i + 1; j < len(events); j++ {
			e2 := events[j]
			diff := e2.Timestamp.Sub(e1.Timestamp)
			if diff > r.WindowDelta {
				break
			}
			if e2.IEEE == dstIEEE {
				correlations++
				break
			}
		}
	}

	return min(float64(correlations)/5.0, 1.0)
}

func (r *RuleBasedAnalyzer) detectGroupedScene(topology NetworkTopology, events []DeviceEvent) *Recommendation {
	if len(topology.Devices) < 2 {
		return nil
	}

	// Suggest a Good Night / All Off automated scene if multiple lights exist
	var lightIEEEs []string
	for _, dev := range topology.Devices {
		if slices.Contains(dev.InputClusters, zcl.ClusterOnOff) {
			lightIEEEs = append(lightIEEEs, dev.IEEE)
		}
	}

	if len(lightIEEEs) >= 2 {
		actions := make([]SceneAction, 0, len(lightIEEEs))
		for _, ieee := range lightIEEEs {
			actions = append(actions, SceneAction{
				TargetIEEE:     ieee,
				TargetEndpoint: 1,
				Command:        "turn_off",
				Parameters:     map[string]any{"state": "OFF"},
			})
		}

		return &Recommendation{
			ID:          "rec-scene-all-off",
			Type:        TypeAutomatedScene,
			Title:       "Automated Scene: All Off / Goodnight",
			Description: fmt.Sprintf("Synchronize %d lights to turn off in a single command.", len(lightIEEEs)),
			Confidence:  0.88,
			SuggestedScene: &SceneSuggestion{
				Name:       "All Off",
				Trigger:    "manual_or_schedule",
				Conditions: []string{"time >= 23:00"},
				Actions:    actions,
			},
			Status:             StatusPending,
			ReliabilityBenefit: "Atomic execution reduces radio traffic congestion.",
			CreatedAt:          time.Now().UTC(),
		}
	}

	return nil
}

func friendly(d DeviceSnapshot) string {
	if d.FriendlyName != "" {
		return d.FriendlyName
	}
	if len(d.IEEE) > 8 {
		return d.IEEE[len(d.IEEE)-6:]
	}
	return d.IEEE
}
