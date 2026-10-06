package ai

import (
	"sync"
	"time"

	"github.com/julienbreux/zigbridge/internal/zcl"
)

// DeviceEvent captures an individual telemetry, state change, or command event on the Zigbee network.
type DeviceEvent struct {
	ID           string        `json:"id"`
	Timestamp    time.Time     `json:"timestamp"`
	IEEE         string        `json:"ieee"`
	FriendlyName string        `json:"friendly_name"`
	Endpoint     uint8         `json:"endpoint"`
	ClusterID    zcl.ClusterID `json:"cluster_id"`
	ClusterName  string        `json:"cluster_name"`
	CommandID    uint8         `json:"command_id"`
	AttributeID  uint16        `json:"attribute_id"`
	Value        interface{}   `json:"value"`
	EventType    string        `json:"event_type"` // "attribute_report", "command", "state_change"
}

// DeviceSnapshot summarizes a device's cluster capabilities and state for AI network analysis.
type DeviceSnapshot struct {
	IEEE           string                 `json:"ieee"`
	FriendlyName   string                 `json:"friendly_name"`
	Model          string                 `json:"model"`
	Manufacturer   string                 `json:"manufacturer"`
	Endpoints      []uint16               `json:"endpoints"`
	InputClusters  []zcl.ClusterID        `json:"input_clusters"`  // Server clusters (receives commands)
	OutputClusters []zcl.ClusterID        `json:"output_clusters"` // Client clusters (sends commands)
	State          map[string]interface{} `json:"state"`
	LQI            uint8                  `json:"lqi"`
	LastSeen       time.Time              `json:"last_seen"`
}

// BindingSnapshot represents a currently active binding in the network.
type BindingSnapshot struct {
	SrcIEEE     string        `json:"src_ieee"`
	SrcEndpoint uint8         `json:"src_endpoint"`
	ClusterID   zcl.ClusterID `json:"cluster_id"`
	DstIEEE     string        `json:"dst_ieee"`
	DstEndpoint uint8         `json:"dst_endpoint"`
}

// NetworkTopology supplies complete context for AI models to understand relationships and opportunities.
type NetworkTopology struct {
	CoordinatorIEEE string            `json:"coordinator_ieee"`
	Channel         uint8             `json:"channel"`
	Devices         []DeviceSnapshot  `json:"devices"`
	ActiveBindings  []BindingSnapshot `json:"active_bindings"`
	TotalEvents     int               `json:"total_events"`
}

// EventCollector is a concurrency-safe ring buffer storing recent device telemetry.
type EventCollector struct {
	mu        sync.RWMutex
	events    []DeviceEvent
	maxEvents int
	head      int
	count     int
}

// NewEventCollector creates an EventCollector with a bounded memory footprint.
func NewEventCollector(maxEvents int) *EventCollector {
	if maxEvents <= 0 {
		maxEvents = 1000
	}
	return &EventCollector{
		events:    make([]DeviceEvent, maxEvents),
		maxEvents: maxEvents,
	}
}

// Record appends a new device event to the ring buffer.
func (ec *EventCollector) Record(evt DeviceEvent) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now().UTC()
	}

	ec.events[ec.head] = evt
	ec.head = (ec.head + 1) % ec.maxEvents
	if ec.count < ec.maxEvents {
		ec.count++
	}
}

// RecentEvents returns the most recent N events in chronological order.
func (ec *EventCollector) RecentEvents(limit int) []DeviceEvent {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	if limit <= 0 || limit > ec.count {
		limit = ec.count
	}

	result := make([]DeviceEvent, limit)
	start := (ec.head - limit + ec.maxEvents) % ec.maxEvents

	for i := 0; i < limit; i++ {
		idx := (start + i) % ec.maxEvents
		result[i] = ec.events[idx]
	}

	return result
}

// Count returns the number of captured events currently held in memory.
func (ec *EventCollector) Count() int {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return ec.count
}
