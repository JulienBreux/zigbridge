package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// LLMClient defines the pluggable completion function for external LLMs.
type LLMClient func(ctx context.Context, prompt string) (string, error)

// LLMAnalyzerHook facilitates AI-powered network recommendations using local or remote LLMs.
type LLMAnalyzerHook struct {
	endpoint   string
	apiKey     string
	httpClient *http.Client
	fallback   *RuleBasedAnalyzer
	clientFunc LLMClient
}

// NewLLMAnalyzerHook creates an extensible LLM analyzer hook with rule-based fallback.
func NewLLMAnalyzerHook(endpoint, apiKey string) *LLMAnalyzerHook {
	return &LLMAnalyzerHook{
		endpoint: endpoint,
		apiKey:   apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		fallback: NewRuleBasedAnalyzer(0.75),
	}
}

// SetCustomClient permits injecting a custom completion function (for unit tests or SDKs).
func (h *LLMAnalyzerHook) SetCustomClient(fn LLMClient) {
	h.clientFunc = fn
}

// BuildPrompt constructs a structured JSON context prompt for the LLM.
func (h *LLMAnalyzerHook) BuildPrompt(topology NetworkTopology, events []DeviceEvent) (string, error) {
	type PromptContext struct {
		Topology NetworkTopology `json:"topology"`
		Events   []DeviceEvent   `json:"events"`
	}

	ctxData := PromptContext{
		Topology: topology,
		Events:   events,
	}

	jsonData, err := json.MarshalIndent(ctxData, "", "  ")
	if err != nil {
		return "", err
	}

	prompt := fmt.Sprintf(`You are an expert Zigbee IoT network optimization and home automation engine.
Analyze the following network topology (devices, endpoints, supported clusters) and historical device interaction events.
Propose high-impact direct bindings (e.g. binding battery switches to lights on OnOff or LevelControl) and automated scenes.

Network & Event Data:
%s

Output ONLY valid JSON matching this schema:
[
  {
    "id": "rec-llm-1",
    "type": "direct_binding",
    "title": "Direct Binding Title",
    "description": "Why this direct binding optimizes the mesh and eliminates latency",
    "confidence": 0.95,
    "source_ieee": "0x...",
    "source_endpoint": 1,
    "target_ieee": "0x...",
    "target_endpoint": 1,
    "cluster_id": 6,
    "cluster_name": "OnOff",
    "estimated_latency_drop_ms": 120,
    "reliability_benefit": "Works without bridge connectivity"
  }
]
`, string(jsonData))

	return prompt, nil
}

// Analyze sends the structured topology and event log to the LLM, parsing suggestions.
func (h *LLMAnalyzerHook) Analyze(ctx context.Context, topology NetworkTopology, events []DeviceEvent) ([]Recommendation, error) {
	// If custom client function or HTTP endpoint configured, invoke LLM
	if h.clientFunc != nil {
		prompt, err := h.BuildPrompt(topology, events)
		if err != nil {
			return h.fallback.Analyze(ctx, topology, events)
		}

		response, err := h.clientFunc(ctx, prompt)
		if err != nil {
			// Gracefully fallback to deterministic heuristics
			return h.fallback.Analyze(ctx, topology, events)
		}

		var recs []Recommendation
		if err := json.Unmarshal([]byte(response), &recs); err == nil && len(recs) > 0 {
			for i := range recs {
				recs[i].Status = StatusPending
				if recs[i].CreatedAt.IsZero() {
					recs[i].CreatedAt = time.Now().UTC()
				}
				if recs[i].ClusterID != 0 && recs[i].ClusterName == "" {
					recs[i].ClusterName = recs[i].ClusterID.String()
				}
			}
			return recs, nil
		}
	}

	if h.endpoint != "" {
		prompt, err := h.BuildPrompt(topology, events)
		if err == nil {
			recs, err := h.callEndpoint(ctx, prompt)
			if err == nil && len(recs) > 0 {
				return recs, nil
			}
		}
	}

	// Always fallback safely to rule-based engine
	return h.fallback.Analyze(ctx, topology, events)
}

func (h *LLMAnalyzerHook) callEndpoint(ctx context.Context, prompt string) ([]Recommendation, error) {
	reqBody, _ := json.Marshal(map[string]any{
		"prompt": prompt,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if h.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.apiKey)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm endpoint returned status %d", resp.StatusCode)
	}

	var recs []Recommendation
	if err := json.NewDecoder(resp.Body).Decode(&recs); err != nil {
		return nil, err
	}

	for i := range recs {
		recs[i].Status = StatusPending
		if recs[i].CreatedAt.IsZero() {
			recs[i].CreatedAt = time.Now().UTC()
		}
		if recs[i].ClusterID != 0 && recs[i].ClusterName == "" {
			recs[i].ClusterName = recs[i].ClusterID.String()
		}
	}

	return recs, nil
}
