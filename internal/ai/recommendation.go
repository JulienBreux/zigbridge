package ai

import (
	"context"
	"time"

	"github.com/julienbreux/zigbridge/internal/zcl"
)

// RecommendationType distinguishes direct Zigbee bindings from smart automated scenes.
type RecommendationType string

const (
	TypeDirectBinding RecommendationType = "direct_binding"
	TypeAutomatedScene RecommendationType = "automated_scene"
)

// RecommendationStatus tracks user or system actions on proposals.
type RecommendationStatus string

const (
	StatusPending   RecommendationStatus = "pending"
	StatusApplied   RecommendationStatus = "applied"
	StatusDismissed RecommendationStatus = "dismissed"
)

// SceneAction defines a targeted operation within an automated scene suggestion.
type SceneAction struct {
	TargetIEEE     string                 `json:"target_ieee"`
	TargetEndpoint uint8                  `json:"target_endpoint"`
	Command        string                 `json:"command"` // e.g. "turn_on", "set_level"
	Parameters     map[string]interface{} `json:"parameters"`
}

// SceneSuggestion describes an automated multi-device routine.
type SceneSuggestion struct {
	Name        string        `json:"name"`
	Trigger     string        `json:"trigger"`
	Conditions  []string      `json:"conditions"`
	Actions     []SceneAction `json:"actions"`
}

// Recommendation represents an actionable suggestion derived from network telemetry.
type Recommendation struct {
	ID                   string               `json:"id"`
	Type                 RecommendationType   `json:"type"`
	Title                string               `json:"title"`
	Description          string               `json:"description"`
	Confidence           float64              `json:"confidence"` // 0.0 to 1.0
	SourceIEEE           string               `json:"source_ieee,omitempty"`
	SourceEndpoint       uint8                `json:"source_endpoint,omitempty"`
	TargetIEEE           string               `json:"target_ieee,omitempty"`
	TargetEndpoint       uint8                `json:"target_endpoint,omitempty"`
	ClusterID            zcl.ClusterID        `json:"cluster_id,omitempty"`
	ClusterName          string               `json:"cluster_name,omitempty"`
	SuggestedScene       *SceneSuggestion     `json:"suggested_scene,omitempty"`
	Status               RecommendationStatus `json:"status"`
	EstimatedLatencyDrop int                  `json:"estimated_latency_drop_ms"` // ms saved by direct binding
	ReliabilityBenefit   string               `json:"reliability_benefit"`
	CreatedAt            time.Time            `json:"created_at"`
}

// Analyzer inspects network topology and device events to propose direct bindings and scenes.
type Analyzer interface {
	// Analyze inspects current topology and event logs to generate recommendations.
	Analyze(ctx context.Context, topology NetworkTopology, events []DeviceEvent) ([]Recommendation, error)
}
