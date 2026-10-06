package controller

import (
	"bytes"
	"cmp"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/julienbreux/zigbridge/internal/adapter"
	"github.com/julienbreux/zigbridge/internal/ai"
	"github.com/julienbreux/zigbridge/internal/binding"
	"github.com/julienbreux/zigbridge/internal/config"
	"github.com/julienbreux/zigbridge/internal/fixture"
	"github.com/julienbreux/zigbridge/internal/mqtt"
	"github.com/julienbreux/zigbridge/internal/transport"
	"github.com/julienbreux/zigbridge/internal/zcl"
)

// VirtualDeviceManager is an optional interface implemented by adapters supporting simulated devices.
type VirtualDeviceManager interface {
	SpawnVirtualDevice(def *fixture.DeviceDefinition, ieee string, nwk uint16) (*fixture.VirtualDevice, error)
	GetVirtualDevices() []*fixture.VirtualDevice
	GetVirtualDevice(ieee string) (*fixture.VirtualDevice, bool)
}

// ErrAIDisabled indicates the AI recommendation engine is disabled in configuration.
var ErrAIDisabled = errors.New("ai recommendation engine is disabled in configuration")

// ErrCoordinatorNotConnected indicates the radio coordinator is disconnected or offline.
var ErrCoordinatorNotConnected = errors.New("coordinator is not connected")

// BridgeStatus reports high-level metrics and coordinator state.
type BridgeStatus struct {
	Connected           bool                `json:"connected"`
	TransportStatus     string              `json:"transport_status"`
	Coordinator         adapter.AdapterInfo `json:"coordinator"`
	DeviceCount         int                 `json:"device_count"`
	BindingCount        int                 `json:"binding_count"`
	PermitJoinRemaining uint8               `json:"permit_join_remaining"`
	MQTTConnected       bool                `json:"mqtt_connected"`
	UptimeSeconds       int64               `json:"uptime_seconds"`
	AIEnabled           bool                `json:"ai_enabled"`
}

// Controller coordinates the transport, radio adapter, device registry,
// binding engine, AI telemetry collection, MQTT client, and real-time events.
type Controller struct {
	cfg       *config.Config
	transport transport.Transport
	adapter   adapter.Adapter
	devices   *DeviceRegistry
	bindings  *binding.Engine
	store     *DeviceStore
	collector *ai.EventCollector
	analyzer  ai.Analyzer
	mqtt      mqtt.Client
	eventBus  *EventBus
	fixtures  *fixture.Registry

	mu                  sync.RWMutex
	startTime           time.Time
	running             bool
	permitJoinRemaining atomic.Uint32
	permitJoinCancel    context.CancelFunc

	// Cache recommendations generated on-demand
	cachedRecs []ai.Recommendation
}

// New creates a new Controller with all subsystem dependencies.
func New(
	cfg *config.Config,
	t transport.Transport,
	adp adapter.Adapter,
	mq mqtt.Client,
) *Controller {
	if mq == nil {
		mq = mqtt.NewMockClient()
	}

	bindingEngine := binding.NewEngine(adp)
	collector := ai.NewEventCollector(cfg.AI.MaxEventHistory)
	analyzer := ai.NewRuleBasedAnalyzer(cfg.AI.MinConfidence)
	bus := NewEventBus()
	devices := NewDeviceRegistry()
	store := NewDeviceStore(cfg.Storage.DevicesPath, cfg.Storage.Debounce, devices, bindingEngine)

	fixtures := fixture.NewRegistry()
	_ = fixtures.LoadEmbedded()
	_ = fixtures.LoadFromDir("fixtures/devices")

	c := &Controller{
		cfg:        cfg,
		transport:  t,
		adapter:    adp,
		devices:    devices,
		bindings:   bindingEngine,
		store:      store,
		collector:  collector,
		analyzer:   analyzer,
		mqtt:       mq,
		eventBus:   bus,
		fixtures:   fixtures,
		cachedRecs: make([]ai.Recommendation, 0),
	}

	// Wire binding engine events to EventBus and persistent storage
	bindingEngine.SetChangeListener(func(b *binding.Binding, action string) {
		c.eventBus.Publish("binding_change", map[string]any{
			"action":  action,
			"binding": b,
		})
		c.store.ScheduleSave()
	})

	// Wire adapter incoming frames and device join indications
	adp.RegisterFrameHandler(c.HandleIncomingFrame)
	adp.RegisterDeviceJoinHandler(c.HandleDeviceJoin)

	return c
}

// Start boots up the transport, connects to the coordinator, and initializes MQTT.
func (c *Controller) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return nil
	}
	c.startTime = time.Now().UTC()
	c.running = true
	c.mu.Unlock()

	// 1. Establish transport connection (TCP or Serial)
	if err := c.transport.Open(ctx); err != nil {
		// Log warning: for TCP transport, auto-reconnect will keep attempting in background
		c.eventBus.Publish("system_warning", fmt.Sprintf("Transport open warning: %v", err))
	}

	// 2. Initialize and start radio adapter (preserves existing NVRAM by default)
	if err := c.adapter.Init(ctx, c.transport); err != nil {
		return fmt.Errorf("failed to initialize adapter: %w", err)
	}

	if err := c.adapter.Start(ctx); err != nil {
		return fmt.Errorf("failed to start radio adapter: %w", err)
	}

	// 3. Connect MQTT client if enabled
	if c.cfg.MQTT.Enabled && c.mqtt != nil {
		if err := c.mqtt.Connect(ctx); err != nil {
			c.eventBus.Publish("system_warning", fmt.Sprintf("MQTT connect warning: %v", err))
		}
	}

	// 4. Load persisted devices and direct bindings from disk
	if c.store != nil {
		if err := c.store.Load(); err != nil {
			c.eventBus.Publish("system_warning", fmt.Sprintf("Failed to load devices storage: %v", err))
		} else {
			for _, dev := range c.devices.GetAll() {
				c.publishDeviceDiscovery(dev)
			}
		}
	}

	// 5. Open network joining on start if configured
	if c.cfg.Network.PermitJoinOnStart && c.cfg.Network.PermitJoinDuration > 0 {
		_ = c.PermitJoin(ctx, c.cfg.Network.PermitJoinDuration)
	}

	return nil
}

// Stop cleanly terminates all connections and subsystems.
func (c *Controller) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	c.running = false
	if c.permitJoinCancel != nil {
		c.permitJoinCancel()
	}

	_ = c.adapter.Stop()
	_ = c.transport.Close()
	if c.mqtt != nil {
		c.mqtt.Disconnect(250)
	}
	if c.store != nil {
		_ = c.store.Flush()
	}
	c.eventBus.Close()

	return nil
}

// IsCoordinatorConnected reports whether the transport is active and the radio adapter is operational.
func (c *Controller) IsCoordinatorConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.transport == nil || !c.transport.IsConnected() {
		return false
	}
	if c.adapter == nil {
		return false
	}
	status := c.adapter.Info().Status
	return status == "ready" || status == "running"
}

// PermitJoin opens network joining for the given duration (in seconds, max 254).
func (c *Controller) PermitJoin(ctx context.Context, duration uint8) error {
	if !c.IsCoordinatorConnected() {
		return ErrCoordinatorNotConnected
	}

	c.mu.Lock()
	if c.permitJoinCancel != nil {
		c.permitJoinCancel()
		c.permitJoinCancel = nil
	}

	if err := c.adapter.PermitJoin(ctx, duration); err != nil {
		c.mu.Unlock()
		return fmt.Errorf("adapter permit join failed: %w", err)
	}

	c.permitJoinRemaining.Store(uint32(duration))
	c.eventBus.Publish("permit_join", map[string]any{
		"duration":  duration,
		"remaining": duration,
	})

	if duration > 0 {
		timerCtx, cancel := context.WithCancel(context.Background())
		c.permitJoinCancel = cancel
		go c.permitJoinCountdown(timerCtx, duration)
	}
	c.mu.Unlock()

	return nil
}

func (c *Controller) permitJoinCountdown(ctx context.Context, duration uint8) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	rem := int(duration)
	for rem > 0 {
		select {
		case <-ctx.Done():
			c.permitJoinRemaining.Store(0)
			return
		case <-ticker.C:
			rem--
			c.permitJoinRemaining.Store(uint32(rem))
			c.eventBus.Publish("permit_join", map[string]any{
				"remaining": rem,
			})
		}
	}
}

// HandleIncomingFrame processes an incoming ZCL frame with zero-copy decoding.
func (c *Controller) HandleIncomingFrame(frame *zcl.Frame) {
	if frame == nil {
		return
	}

	stateUpdates := make(map[string]any)
	existingDev, _ := c.devices.Get(frame.SourceAddress)

	// Parse attribute reports if present (Global profile commands)
	isAttributeReport := frame.Header.Type == zcl.FrameTypeGlobal &&
		(frame.CommandID == zcl.CmdReportAttributes || frame.CommandID == zcl.CmdReadAttributesResponse)

	if isAttributeReport {
		var records []zcl.AttributeRecord
		var err error
		if frame.CommandID == zcl.CmdReadAttributesResponse {
			records, err = zcl.ParseReadAttributesResponse(frame.Payload)
		} else {
			records, err = zcl.ParseAttributeReport(frame.Payload)
		}
		if err == nil {
			for _, rec := range records {
				switch frame.ClusterID {
				case zcl.ClusterBasic:
					switch rec.AttributeID {
					case 0x0004: // Manufacturer Name
						if str, ok := rec.Value.(string); ok {
							model := ""
							if existingDev != nil {
								model = existingDev.Model
							}
							c.devices.UpdateModelInfo(frame.SourceAddress, str, model)
							if c.store != nil {
								c.store.ScheduleSave()
							}
						}
					case 0x0005: // Model Identifier
						if str, ok := rec.Value.(string); ok {
							mfg := ""
							if existingDev != nil {
								mfg = existingDev.Manufacturer
							}
							c.devices.UpdateModelInfo(frame.SourceAddress, mfg, str)
							if c.store != nil {
								c.store.ScheduleSave()
							}
							// Match against fixture registry to enrich metadata and HA discovery
							if c.fixtures != nil {
								if def, ok := c.fixtures.Get(str); ok {
									for _, ep := range def.Device.Endpoints {
										var inClusters, outClusters []zcl.ClusterID
										for _, inC := range ep.InputClusters {
											inClusters = append(inClusters, zcl.ClusterID(inC))
										}
										for _, outC := range ep.OutputClusters {
											outClusters = append(outClusters, zcl.ClusterID(outC))
										}
										c.devices.UpdateMetadata(frame.SourceAddress, []uint16{uint16(ep.Endpoint)}, inClusters, outClusters)
									}
									if enrichedDev, ok := c.devices.Get(frame.SourceAddress); ok {
										c.publishDeviceDiscovery(enrichedDev)
									}
								}
							}
						}
					}
				case zcl.ClusterOnOff:
					if b, ok := rec.Value.(bool); ok {
						stateUpdates["state"] = "OFF"
						if b {
							stateUpdates["state"] = "ON"
						}
					}
				case zcl.ClusterTemperatureMeasurement:
					if val, ok := rec.Value.(int16); ok {
						stateUpdates["temperature"] = float64(val) / 100.0
					}
				case zcl.ClusterRelativeHumidity:
					if val, ok := rec.Value.(uint16); ok {
						stateUpdates["humidity"] = float64(val) / 100.0
					}
				case zcl.ClusterOccupancySensing:
					if b, ok := rec.Value.(bool); ok {
						stateUpdates["occupancy"] = b
					}
				case zcl.ClusterElectricalMeasurement:
					isDualPhase := false
					if existingDev != nil && c.fixtures != nil && existingDev.Model != "" {
						if def, ok := c.fixtures.Get(existingDev.Model); ok {
							for _, exp := range def.Device.Exposes {
								if exp.Property == "power_a" {
									isDualPhase = true
									break
								}
							}
						}
					}
					switch rec.AttributeID {
					case 0x0505: // RMSVoltage (V)
						if v, ok := toFloat64(rec.Value); ok {
							stateUpdates["voltage"] = v
						}
					case 0x0508: // RMSCurrent (A)
						if v, ok := toFloat64(rec.Value); ok {
							if isDualPhase && frame.SourceEndpoint == 2 {
								stateUpdates["current_b"] = v
							} else if isDualPhase {
								stateUpdates["current_a"] = v
							} else {
								stateUpdates["current"] = v
							}
						}
					case 0x050B: // ActivePower (W)
						if v, ok := toFloat64(rec.Value); ok {
							if isDualPhase && frame.SourceEndpoint == 2 {
								stateUpdates["power_b"] = v
							} else if isDualPhase {
								stateUpdates["power_a"] = v
							} else {
								stateUpdates["power"] = v
							}
						}
					default:
						if v, ok := toFloat64(rec.Value); ok {
							if isDualPhase && frame.SourceEndpoint == 2 {
								stateUpdates["power_b"] = v
							} else if isDualPhase {
								stateUpdates["power_a"] = v
							} else {
								stateUpdates["power"] = v
							}
						}
					}
				case zcl.ClusterMetering:
					isDualPhase := false
					if existingDev != nil && c.fixtures != nil && existingDev.Model != "" {
						if def, ok := c.fixtures.Get(existingDev.Model); ok {
							for _, exp := range def.Device.Exposes {
								if exp.Property == "energy_a" {
									isDualPhase = true
									break
								}
							}
						}
					}
					switch rec.AttributeID {
					case 0x0000: // CurrentSummationDelivered (kWh)
						if v, ok := toFloat64(rec.Value); ok {
							if isDualPhase && frame.SourceEndpoint == 2 {
								stateUpdates["energy_b"] = v
							} else if isDualPhase {
								stateUpdates["energy_a"] = v
							} else {
								stateUpdates["energy"] = v
							}
						}
					}
				case zcl.ClusterIASZone:
					if rec.AttributeID == 0x0002 { // ZoneStatus
						if val, ok := toUint16(rec.Value); ok {
							c.applyIASZoneStatus(existingDev, val, stateUpdates)
						}
					}
				case zcl.ClusterPowerConfiguration:
					if val, ok := rec.Value.(uint8); ok {
						switch rec.AttributeID {
						case 0x0021: // BatteryPercentageRemaining (0-200)
							stateUpdates["battery"] = val / 2
						case 0x0020: // BatteryVoltage (in 100mV units)
							stateUpdates["voltage"] = uint16(val) * 100
						default:
							stateUpdates["battery"] = val
						}
					}
				}
			}
		}
	}

	// Check if this is an action command (non-report, cluster-specific or button command)
	if !isAttributeReport {
		matchedAction := false
		if c.fixtures != nil && existingDev != nil && existingDev.Model != "" {
			if def, ok := c.fixtures.Get(existingDev.Model); ok {
				var fallbackAct *fixture.ActionSim
				for _, act := range def.Device.Simulations.Actions {
					if act.Cluster != uint16(frame.ClusterID) || act.Command != frame.CommandID {
						continue
					}
					if len(act.Payload) > 0 {
						if bytes.HasPrefix(frame.Payload, act.Payload) {
							actCopy := act
							fallbackAct = &actCopy
							break
						}
					} else if fallbackAct == nil {
						actCopy := act
						fallbackAct = &actCopy
					}
				}
				if fallbackAct != nil {
					maps.Copy(stateUpdates, fallbackAct.MQTTPayload)
					matchedAction = true
				}
			}
		}

		if !matchedAction {
			switch frame.ClusterID {
			case zcl.ClusterOnOff:
				switch frame.CommandID {
				case 0x02: // Toggle
					stateUpdates["action"] = "single"
				case 0x01: // On
					if existingDev != nil && (existingDev.Model == "SNZB-01P" || existingDev.Model == "WB01") {
						stateUpdates["action"] = "double"
					} else {
						stateUpdates["action"] = "on"
					}
				case 0x00: // Off
					if existingDev != nil && (existingDev.Model == "SNZB-01P" || existingDev.Model == "WB01") {
						stateUpdates["action"] = "long"
					} else {
						stateUpdates["action"] = "off"
					}
				}
			case zcl.ClusterIASZone:
				if frame.CommandID == 0x00 && len(frame.Payload) >= 2 { // CmdIASZoneStatusChangeNotification
					status := binary.LittleEndian.Uint16(frame.Payload[:2])
					c.applyIASZoneStatus(existingDev, status, stateUpdates)
				}
			case zcl.ClusterIASACE:
				switch frame.CommandID {
				case zcl.CmdIASACEArm: // 0x00
					if len(frame.Payload) > 0 {
						switch frame.Payload[0] {
						case 0:
							stateUpdates["action"] = "disarm"
						case 1:
							stateUpdates["action"] = "arm_day_zones"
						case 2:
							stateUpdates["action"] = "arm_night_zones"
						case 3:
							stateUpdates["action"] = "arm_all_zones"
						default:
							stateUpdates["action"] = "arm"
						}
					} else {
						stateUpdates["action"] = "arm"
					}
				case zcl.CmdIASACEBypass: // 0x01
					stateUpdates["action"] = "bypass"
				case zcl.CmdIASACEEmergency: // 0x02
					stateUpdates["action"] = "emergency"
				case zcl.CmdIASACEFire: // 0x03
					stateUpdates["action"] = "fire"
				case zcl.CmdIASACEPanic: // 0x04
					stateUpdates["action"] = "panic"
				}
			case zcl.ClusterLevelControl:
				switch frame.CommandID {
				case 0x01, 0x05: // Move / MoveWithOnOff
					if len(frame.Payload) > 0 && frame.Payload[0] == 1 {
						stateUpdates["action"] = "brightness_move_down"
					} else {
						stateUpdates["action"] = "brightness_move_up"
					}
				case 0x02: // Step
					if len(frame.Payload) > 0 && frame.Payload[0] == 1 {
						stateUpdates["action"] = "brightness_step_down"
					} else {
						stateUpdates["action"] = "brightness_step_up"
					}
				case 0x03, 0x06: // Stop / StopWithOnOff
					stateUpdates["action"] = "brightness_stop"
				}
			case zcl.ClusterScenes:
				switch frame.CommandID {
				case 0x07: // RecallScene
					if len(frame.Payload) > 0 && frame.Payload[0] == 1 {
						stateUpdates["action"] = "arrow_left_click"
					} else {
						stateUpdates["action"] = "arrow_right_click"
					}
				case 0x08:
					if len(frame.Payload) > 0 && frame.Payload[0] == 1 {
						stateUpdates["action"] = "arrow_left_hold"
					} else {
						stateUpdates["action"] = "arrow_right_hold"
					}
				case 0x09:
					if len(frame.Payload) > 0 && frame.Payload[0] == 1 {
						stateUpdates["action"] = "arrow_left_release"
					} else {
						stateUpdates["action"] = "arrow_right_release"
					}
				}
			}
		}
	}

	// Update device state in registry
	dev, _ := c.devices.UpdateState(frame.SourceAddress, stateUpdates, frame.LQI)

	friendlyName := frame.SourceAddress
	if dev != nil && dev.FriendlyName != "" {
		friendlyName = dev.FriendlyName
	}

	// Record telemetry event in AI bounded ring buffer if enabled
	if c.cfg.AI.Enabled && c.collector != nil {
		c.collector.Record(ai.DeviceEvent{
			ID:           fmt.Sprintf("evt-%d", time.Now().UnixNano()),
			Timestamp:    time.Now().UTC(),
			IEEE:         frame.SourceAddress,
			FriendlyName: friendlyName,
			Endpoint:     frame.SourceEndpoint,
			ClusterID:    frame.ClusterID,
			ClusterName:  frame.ClusterID.String(),
			CommandID:    frame.CommandID,
			Value:        stateUpdates,
			EventType:    "attribute_report",
		})
	}

	// Publish state update to MQTT
	if c.cfg.MQTT.Enabled && c.mqtt != nil && c.mqtt.IsConnected() && len(stateUpdates) > 0 {
		_ = c.mqtt.PublishDeviceState(frame.SourceAddress, stateUpdates)
		if friendlyName != frame.SourceAddress {
			_ = c.mqtt.PublishDeviceState(friendlyName, stateUpdates)
		}
	}

	// Broadcast frame & state update on EventBus
	c.eventBus.Publish("frame", frame)
	if len(stateUpdates) > 0 {
		c.eventBus.Publish("device_state", map[string]any{
			"ieee":          frame.SourceAddress,
			"friendly_name": friendlyName,
			"state":         stateUpdates,
			"lqi":           frame.LQI,
		})
	}

	// Persist battery level updates if reported
	if _, hasBattery := stateUpdates["battery"]; hasBattery && c.store != nil {
		c.store.ScheduleSave()
	}
}

// HandleDeviceJoin registers newly paired devices and publishes HA discovery.
func (c *Controller) HandleDeviceJoin(info adapter.DeviceJoinInfo) {
	dev := &Device{
		IEEE:           info.IEEE,
		NWK:            info.NWK,
		FriendlyName:   info.IEEE,
		Endpoints:      []uint16{1},
		InputClusters:  []zcl.ClusterID{zcl.ClusterBasic, zcl.ClusterIdentify, zcl.ClusterOnOff},
		OutputClusters: []zcl.ClusterID{zcl.ClusterOnOff, zcl.ClusterLevelControl},
		State:          make(map[string]any),
		Available:      true,
		LastSeen:       time.Now().UTC(),
	}

	c.devices.Upsert(dev)

	if c.store != nil {
		c.store.ScheduleSave()
	}

	// Publish Home Assistant auto-discovery entities
	c.publishDeviceDiscovery(dev)

	c.eventBus.Publish("system_info", fmt.Sprintf("New device joined: IEEE=%s NWK=0x%04X", info.IEEE, info.NWK))
	c.eventBus.Publish("device_join", dev.Clone())

	// Actively interview device for Basic Cluster (0x0000) attributes (Manufacturer & Model)
	go c.interviewDevice(info.IEEE, info.NWK)
}

func (c *Controller) interviewDevice(ieee string, nwk uint16) {
	time.Sleep(1500 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	readPayload := []byte{
		0x04, 0x00, // 0x0004: Manufacturer Name
		0x05, 0x00, // 0x0005: Model Identifier
	}

	frame := &zcl.Frame{
		Header: zcl.FrameControl{
			Type:                   zcl.FrameTypeGlobal,
			ManufacturerSpecific:   false,
			Direction:              zcl.DirectionClientToServer,
			DisableDefaultResponse: false,
		},
		TransactionSequenceNum: 1,
		CommandID:              zcl.CmdReadAttributes,
		ClusterID:              zcl.ClusterBasic,
		DestAddress:            ieee,
		DestEndpoint:           1,
		SourceEndpoint:         1,
		Payload:                readPayload,
	}

	if err := c.adapter.SendZCL(ctx, frame); err != nil {
		c.eventBus.Publish("system_warning", fmt.Sprintf("Failed to query device %s model info: %v", ieee, err))
	}
}

// CreateDirectBinding executes a direct Zigbee binding with optimistic support.
// Returns the binding, any warnings (e.g. if the target was not fully discovered), and an error if failed.
func (c *Controller) CreateDirectBinding(ctx context.Context, req adapter.BindRequest) (*binding.Binding, []string, error) {
	if !c.IsCoordinatorConnected() {
		return nil, nil, ErrCoordinatorNotConnected
	}

	var warnings []string

	// Check if source device is known
	if _, found := c.devices.Get(req.SrcIEEE); !found {
		warnings = append(warnings, fmt.Sprintf("Source device %s is not yet recognized in local registry (optimistic binding)", req.SrcIEEE))
	}

	// Check if target device is known
	if _, found := c.devices.Get(req.DstIEEE); !found {
		warnings = append(warnings, fmt.Sprintf("Target device %s is not yet recognized in local registry (optimistic binding)", req.DstIEEE))
	}

	b, err := c.bindings.CreateBinding(ctx, req)
	if err != nil {
		return nil, warnings, err
	}

	return b, warnings, nil
}

// RemoveDirectBinding deletes an active direct binding.
func (c *Controller) RemoveDirectBinding(ctx context.Context, req adapter.BindRequest) error {
	if !c.IsCoordinatorConnected() {
		return ErrCoordinatorNotConnected
	}
	return c.bindings.RemoveBinding(ctx, req)
}

// GetRecommendations inspects topology and historical events on-demand to propose bindings and scenes.
func (c *Controller) GetRecommendations(ctx context.Context) ([]ai.Recommendation, error) {
	if !c.cfg.AI.Enabled {
		return nil, ErrAIDisabled
	}

	devices := c.devices.GetAll()
	snapshots := make([]ai.DeviceSnapshot, len(devices))
	for i, d := range devices {
		snapshots[i] = d.ToSnapshot()
	}

	bindingsList := c.bindings.ListBindings()
	bindingSnaps := make([]ai.BindingSnapshot, len(bindingsList))
	for i, b := range bindingsList {
		bindingSnaps[i] = ai.BindingSnapshot{
			SrcIEEE:     b.SrcIEEE,
			SrcEndpoint: b.SrcEndpoint,
			ClusterID:   b.ClusterID,
			DstIEEE:     b.DstIEEE,
			DstEndpoint: b.DstEndpoint,
		}
	}

	info := c.adapter.Info()
	topology := ai.NetworkTopology{
		CoordinatorIEEE: info.IEEE,
		Channel:         info.Channel,
		Devices:         snapshots,
		ActiveBindings:  bindingSnaps,
		TotalEvents:     c.collector.Count(),
	}

	events := c.collector.RecentEvents(100)
	recs, err := c.analyzer.Analyze(ctx, topology, events)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.cachedRecs = recs
	c.mu.Unlock()

	return recs, nil
}

// ApplyRecommendation executes an actionable recommendation (e.g. creating the direct binding).
func (c *Controller) ApplyRecommendation(ctx context.Context, recID string) error {
	if !c.cfg.AI.Enabled {
		return ErrAIDisabled
	}
	if !c.IsCoordinatorConnected() {
		return ErrCoordinatorNotConnected
	}

	c.mu.Lock()
	var targetRec *ai.Recommendation
	for i := range c.cachedRecs {
		if c.cachedRecs[i].ID == recID {
			targetRec = &c.cachedRecs[i]
			break
		}
	}
	c.mu.Unlock()

	if targetRec == nil {
		return fmt.Errorf("recommendation not found: %s", recID)
	}

	if targetRec.Type == ai.TypeDirectBinding {
		req := adapter.BindRequest{
			SrcIEEE:     targetRec.SourceIEEE,
			SrcEndpoint: targetRec.SourceEndpoint,
			ClusterID:   targetRec.ClusterID,
			DstIEEE:     targetRec.TargetIEEE,
			DstEndpoint: targetRec.TargetEndpoint,
		}

		_, _, err := c.CreateDirectBinding(ctx, req)
		if err != nil {
			return err
		}

		c.mu.Lock()
		targetRec.Status = ai.StatusApplied
		c.mu.Unlock()

		c.eventBus.Publish("ai_recommendation_applied", targetRec)
		return nil
	}

	return fmt.Errorf("unsupported recommendation type for automated apply: %s", targetRec.Type)
}

// Status returns a point-in-time snapshot of bridge operation.
func (c *Controller) Status() BridgeStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var uptime int64
	if !c.startTime.IsZero() {
		uptime = int64(time.Since(c.startTime).Seconds())
	}

	mqConnected := false
	if c.mqtt != nil {
		mqConnected = c.mqtt.IsConnected()
	}

	connected := false
	transStatus := "disconnected"
	if c.transport != nil {
		connected = c.transport.IsConnected()
		transStatus = string(c.transport.Status())
	}

	var coordInfo adapter.AdapterInfo
	if c.adapter != nil {
		coordInfo = c.adapter.Info()
	}

	return BridgeStatus{
		Connected:           connected,
		TransportStatus:     transStatus,
		Coordinator:         coordInfo,
		DeviceCount:         len(c.devices.GetAll()),
		BindingCount:        len(c.bindings.ListBindings()),
		PermitJoinRemaining: uint8(c.permitJoinRemaining.Load()),
		MQTTConnected:       mqConnected,
		UptimeSeconds:       uptime,
		AIEnabled:           c.cfg.AI.Enabled,
	}
}

// GetDevices returns all registered devices.
func (c *Controller) GetDevices() []*Device {
	return c.devices.GetAll()
}

// GetDevice retrieves a device by its IEEE address.
func (c *Controller) GetDevice(ieee string) (*Device, bool) {
	return c.devices.Get(ieee)
}

// SetDeviceFriendlyName updates the user-assigned alias for a device.
func (c *Controller) SetDeviceFriendlyName(ieee, name string) bool {
	ok := c.devices.SetFriendlyName(ieee, name)
	if ok {
		if c.store != nil {
			c.store.ScheduleSave()
		}
		c.eventBus.Publish("device_updated", map[string]string{
			"ieee":          ieee,
			"friendly_name": name,
		})
	}
	return ok
}

// UpdateDeviceMetadata safely updates endpoints and clusters for a registered device.
func (c *Controller) UpdateDeviceMetadata(ieee string, endpoints []uint16, inClusters, outClusters []zcl.ClusterID) bool {
	ok := c.devices.UpdateMetadata(ieee, endpoints, inClusters, outClusters)
	if ok && c.store != nil {
		c.store.ScheduleSave()
	}
	return ok
}

// UpdateDeviceModel sets the manufacturer and model for a registered device and schedules persistence.
func (c *Controller) UpdateDeviceModel(ieee, manufacturer, model string) bool {
	ok := c.devices.UpdateModelInfo(ieee, manufacturer, model)
	if ok && c.store != nil {
		c.store.ScheduleSave()
	}
	return ok
}

// SetDeviceState updates the state of a device, dispatches commands if applicable, and broadcasts updates.
func (c *Controller) SetDeviceState(ctx context.Context, ieee string, updates map[string]any) (*Device, error) {
	if !c.IsCoordinatorConnected() {
		return nil, ErrCoordinatorNotConnected
	}

	dev, ok := c.devices.Get(ieee)
	if !ok {
		return nil, fmt.Errorf("device %s not found", ieee)
	}

	// Update virtual device if running in mock simulation mode
	if c.IsSimulationSupported() {
		if vdev, ok := c.GetVirtualDevice(ieee); ok {
			vdev.SetState(updates)
		}
	}

	// Update device registry
	updatedDev, ok := c.devices.UpdateState(ieee, updates, dev.LQI)
	if !ok {
		return nil, fmt.Errorf("failed to update state for device %s", ieee)
	}

	// Schedule store persistence
	if c.store != nil {
		c.store.ScheduleSave()
	}

	// Broadcast via EventBus
	c.eventBus.Publish("device_state", map[string]any{
		"ieee":          updatedDev.IEEE,
		"friendly_name": updatedDev.FriendlyName,
		"state":         updatedDev.State,
	})

	// Publish to MQTT
	if c.mqtt != nil && c.mqtt.IsConnected() {
		topic := c.cfg.MQTT.BaseTopic + "/" + cmp.Or(updatedDev.FriendlyName, updatedDev.IEEE)
		payload, err := json.Marshal(updatedDev.State)
		if err == nil {
			_ = c.mqtt.Publish(topic, 0, true, payload)
		}
	}

	// Dispatch outbound ZCL frames to real adapter if physical
	if c.adapter != nil && !c.IsSimulationSupported() {
		c.dispatchStateZCL(ctx, updatedDev, updates)
	}

	return updatedDev, nil
}

// TriggerDeviceAction triggers an action command on a device (e.g. "identify" or virtual device button).
func (c *Controller) TriggerDeviceAction(ctx context.Context, ieee string, action string) error {
	if !c.IsCoordinatorConnected() {
		return ErrCoordinatorNotConnected
	}

	dev, ok := c.devices.Get(ieee)
	if !ok {
		return fmt.Errorf("device %s not found", ieee)
	}

	if c.IsSimulationSupported() {
		if vdev, ok := c.GetVirtualDevice(ieee); ok {
			return vdev.TriggerAction(action)
		}
	}

	if strings.EqualFold(action, "identify") && c.adapter != nil {
		var ep uint8 = 1
		if len(dev.Endpoints) > 0 {
			ep = uint8(dev.Endpoints[0])
		}
		frame := &zcl.Frame{
			Header: zcl.FrameControl{
				Type:                   zcl.FrameTypeClusterSpecific,
				Direction:              zcl.DirectionClientToServer,
				DisableDefaultResponse: true,
			},
			TransactionSequenceNum: 1,
			CommandID:              0, // Identify command
			ClusterID:              zcl.ClusterIdentify,
			DestAddress:            dev.IEEE,
			DestEndpoint:           ep,
			SourceEndpoint:         1,
			Payload:                []byte{0x05, 0x00}, // 5 seconds identify
		}
		return c.adapter.SendZCL(ctx, frame)
	}

	return nil
}

func (c *Controller) dispatchStateZCL(ctx context.Context, dev *Device, updates map[string]any) {
	if c.adapter == nil || dev == nil {
		return
	}
	var ep uint8 = 1
	if len(dev.Endpoints) > 0 {
		ep = uint8(dev.Endpoints[0])
	}

	for k, v := range updates {
		switch strings.ToLower(k) {
		case "state":
			strVal := strings.ToUpper(fmt.Sprint(v))
			var cmdID uint8
			switch strVal {
			case "ON":
				cmdID = 1 // CmdOn
			case "OFF":
				cmdID = 0 // CmdOff
			case "TOGGLE":
				cmdID = 2 // CmdToggle
			default:
				continue
			}
			frame := &zcl.Frame{
				Header: zcl.FrameControl{
					Type:                   zcl.FrameTypeClusterSpecific,
					Direction:              zcl.DirectionClientToServer,
					DisableDefaultResponse: true,
				},
				TransactionSequenceNum: 1,
				CommandID:              cmdID,
				ClusterID:              zcl.ClusterOnOff,
				DestAddress:            dev.IEEE,
				DestEndpoint:           ep,
				SourceEndpoint:         1,
			}
			_ = c.adapter.SendZCL(ctx, frame)
		case "brightness":
			if bNum, ok := toUint8(v); ok {
				frame := &zcl.Frame{
					Header: zcl.FrameControl{
						Type:                   zcl.FrameTypeClusterSpecific,
						Direction:              zcl.DirectionClientToServer,
						DisableDefaultResponse: true,
					},
					TransactionSequenceNum: 1,
					CommandID:              0x04, // Move to Level with On/Off
					ClusterID:              zcl.ClusterLevelControl,
					DestAddress:            dev.IEEE,
					DestEndpoint:           ep,
					SourceEndpoint:         1,
					Payload:                []byte{bNum, 0x0A, 0x00},
				}
				_ = c.adapter.SendZCL(ctx, frame)
			}
		}
	}
}

func toUint8(v any) (uint8, bool) {
	switch val := v.(type) {
	case uint8:
		return val, true
	case int:
		return uint8(val), true
	case float64:
		return uint8(val), true
	default:
		return 0, false
	}
}

// Store returns the persistent DeviceStore manager.
func (c *Controller) Store() *DeviceStore {
	return c.store
}

// publishDeviceDiscovery registers a device entity in Home Assistant via MQTT discovery.
func (c *Controller) publishDeviceDiscovery(dev *Device) {
	if !c.cfg.MQTT.Enabled || !c.cfg.MQTT.HADiscovery || c.mqtt == nil || !c.mqtt.IsConnected() || dev == nil {
		return
	}
	mfr := cmp.Or(dev.Manufacturer, "Zigbee Device")
	model := cmp.Or(dev.Model, "Standard Endpoint")
	haDev := mqtt.HADevice{
		Identifiers:  []string{dev.IEEE},
		Name:         dev.FriendlyName,
		Manufacturer: mfr,
		Model:        model,
	}

	baseTopic := c.cfg.MQTT.BaseTopic

	hasInCluster := func(cid zcl.ClusterID) bool {
		return slices.Contains(dev.InputClusters, cid)
	}
	hasOutCluster := func(cid zcl.ClusterID) bool {
		return slices.Contains(dev.OutputClusters, cid)
	}

	var def *fixture.DeviceDefinition
	if c.fixtures != nil && dev.Model != "" {
		def, _ = c.fixtures.Get(dev.Model)
	}

	hasExpose := func(prop string) bool {
		if def != nil {
			for _, exp := range def.Device.Exposes {
				if exp.Property == prop {
					return true
				}
			}
		}
		return false
	}

	// 1. OnOff Switch: If device has InCluster OnOff, or exposes "state", or has no input clusters and no def
	if hasInCluster(zcl.ClusterOnOff) || hasExpose("state") || (len(dev.InputClusters) == 0 && def == nil) {
		_ = c.mqtt.PublishDiscovery(mqtt.NewOnOffDiscovery(haDev, dev.IEEE, baseTopic))
	}

	// 2. Action & Device Triggers: If button/remote controller (outCluster OnOff/IASACE/Level/Scenes, or exposes action)
	if hasOutCluster(zcl.ClusterOnOff) || hasOutCluster(zcl.ClusterIASACE) || hasOutCluster(zcl.ClusterLevelControl) || hasOutCluster(zcl.ClusterScenes) || hasExpose("action") {
		_ = c.mqtt.PublishDiscovery(mqtt.NewActionDiscovery(haDev, dev.IEEE, baseTopic))

		var actions []string
		if def != nil {
			for _, exp := range def.Device.Exposes {
				if exp.Property == "action" && len(exp.Values) > 0 {
					actions = slices.Clone(exp.Values)
					break
				}
			}
			if len(actions) == 0 && len(def.Device.Simulations.Actions) > 0 {
				for a := range def.Device.Simulations.Actions {
					actions = append(actions, a)
				}
			}
		}
		if len(actions) == 0 {
			actions = []string{"single", "double", "long"}
		}
		slices.Sort(actions)
		actions = slices.Compact(actions)

		for _, action := range actions {
			_ = c.mqtt.PublishDiscovery(mqtt.NewDeviceTriggerDiscovery(haDev, dev.IEEE, baseTopic, action))
		}
	}

	// 3. Electrical Measurement & Metering
	if hasInCluster(zcl.ClusterElectricalMeasurement) || hasExpose("power") || hasExpose("current") || hasExpose("voltage") {
		if hasInCluster(zcl.ClusterElectricalMeasurement) || hasExpose("power") {
			_ = c.mqtt.PublishDiscovery(mqtt.NewPowerDiscovery(haDev, dev.IEEE, baseTopic))
		}
		if hasInCluster(zcl.ClusterElectricalMeasurement) || hasExpose("current") {
			_ = c.mqtt.PublishDiscovery(mqtt.NewCurrentDiscovery(haDev, dev.IEEE, baseTopic))
		}
		if hasInCluster(zcl.ClusterElectricalMeasurement) || hasExpose("voltage") {
			_ = c.mqtt.PublishDiscovery(mqtt.NewMainsVoltageDiscovery(haDev, dev.IEEE, baseTopic))
		}
	}
	if hasInCluster(zcl.ClusterMetering) || hasExpose("energy") {
		_ = c.mqtt.PublishDiscovery(mqtt.NewEnergyDiscovery(haDev, dev.IEEE, baseTopic))
	}

	// Multi-phase or clamped electrical exposes (e.g. PJ-1203A power_a, power_b, current_a, current_b, energy_a, energy_b)
	if def != nil {
		for _, exp := range def.Device.Exposes {
			switch exp.Property {
			case "power_a", "power_b", "power_ab":
				name := haDev.Name + " " + strings.ToUpper(strings.ReplaceAll(exp.Property, "_", " "))
				_ = c.mqtt.PublishDiscovery(mqtt.NewCustomSensorDiscovery(haDev, dev.IEEE, baseTopic, exp.Property, name, "power", "W"))
			case "current_a", "current_b":
				name := haDev.Name + " " + strings.ToUpper(strings.ReplaceAll(exp.Property, "_", " "))
				_ = c.mqtt.PublishDiscovery(mqtt.NewCustomSensorDiscovery(haDev, dev.IEEE, baseTopic, exp.Property, name, "current", "A"))
			case "energy_a", "energy_b":
				name := haDev.Name + " " + strings.ToUpper(strings.ReplaceAll(exp.Property, "_", " "))
				_ = c.mqtt.PublishDiscovery(mqtt.NewCustomSensorDiscovery(haDev, dev.IEEE, baseTopic, exp.Property, name, "energy", "kWh"))
			}
		}
	}

	// 4. Power & Battery
	if hasInCluster(zcl.ClusterPowerConfiguration) || hasExpose("battery") || dev.Battery > 0 {
		_ = c.mqtt.PublishDiscovery(mqtt.NewBatteryDiscovery(haDev, dev.IEEE, baseTopic))
		if !hasInCluster(zcl.ClusterElectricalMeasurement) && !hasExpose("power") {
			_ = c.mqtt.PublishDiscovery(mqtt.NewVoltageDiscovery(haDev, dev.IEEE, baseTopic))
		}
	}

	// 5. Temperature & Humidity
	if hasInCluster(zcl.ClusterTemperatureMeasurement) || hasExpose("temperature") {
		_ = c.mqtt.PublishDiscovery(mqtt.NewTemperatureDiscovery(haDev, dev.IEEE, baseTopic))
	}
	if hasInCluster(zcl.ClusterRelativeHumidity) || hasExpose("humidity") {
		_ = c.mqtt.PublishDiscovery(mqtt.NewHumidityDiscovery(haDev, dev.IEEE, baseTopic))
	}

	// 6. Occupancy
	if hasInCluster(zcl.ClusterOccupancySensing) || hasExpose("occupancy") {
		_ = c.mqtt.PublishDiscovery(mqtt.NewOccupancyDiscovery(haDev, dev.IEEE, baseTopic))
	}

	// 7. IAS Zone (Moisture, Contact, Smoke, Tamper, BatteryLow)
	lowerModel := strings.ToLower(model + " " + dev.FriendlyName)
	if hasInCluster(zcl.ClusterIASZone) || hasExpose("water_leak") || hasExpose("contact") || hasExpose("smoke") {
		if hasExpose("water_leak") || (!hasExpose("contact") && !hasExpose("smoke") && strings.Contains(lowerModel, "leak")) {
			_ = c.mqtt.PublishDiscovery(mqtt.NewMoistureDiscovery(haDev, dev.IEEE, baseTopic))
		}
		if hasExpose("contact") || (!hasExpose("water_leak") && !hasExpose("smoke") && (strings.Contains(lowerModel, "contact") || strings.Contains(lowerModel, "door") || strings.Contains(lowerModel, "window") || strings.Contains(lowerModel, "mccgq"))) {
			_ = c.mqtt.PublishDiscovery(mqtt.NewContactDiscovery(haDev, dev.IEEE, baseTopic))
		}
		if hasExpose("smoke") || strings.Contains(lowerModel, "smoke") {
			_ = c.mqtt.PublishDiscovery(mqtt.NewSmokeDiscovery(haDev, dev.IEEE, baseTopic))
		}
		if hasExpose("tamper") || hasInCluster(zcl.ClusterIASZone) {
			_ = c.mqtt.PublishDiscovery(mqtt.NewTamperDiscovery(haDev, dev.IEEE, baseTopic))
		}
		if hasExpose("battery_low") || hasInCluster(zcl.ClusterIASZone) {
			_ = c.mqtt.PublishDiscovery(mqtt.NewBatteryLowDiscovery(haDev, dev.IEEE, baseTopic))
		}
	}

	// 8. IAS WD / Siren
	if hasInCluster(zcl.ClusterIASWD) || hasExpose("warning") || hasExpose("squawk") || strings.Contains(lowerModel, "siren") {
		_ = c.mqtt.PublishDiscovery(mqtt.NewSirenDiscovery(haDev, dev.IEEE, baseTopic))
	}
}

// Fixtures returns the fixture registry containing loaded device definitions.
func (c *Controller) Fixtures() *fixture.Registry {
	return c.fixtures
}

// IsSimulationSupported returns whether the active adapter supports virtual device simulations.
func (c *Controller) IsSimulationSupported() bool {
	_, ok := c.adapter.(VirtualDeviceManager)
	return ok
}

// SpawnVirtualDevice creates and registers a virtual device if the adapter supports it.
func (c *Controller) SpawnVirtualDevice(def *fixture.DeviceDefinition, ieee string, nwk uint16) (*fixture.VirtualDevice, error) {
	if vdm, ok := c.adapter.(VirtualDeviceManager); ok {
		return vdm.SpawnVirtualDevice(def, ieee, nwk)
	}
	return nil, fmt.Errorf("virtual devices not supported by adapter %s", c.adapter.Info().Type)
}

// GetVirtualDevices returns all currently active virtual devices if running on a mock adapter.
func (c *Controller) GetVirtualDevices() []*fixture.VirtualDevice {
	if vdm, ok := c.adapter.(VirtualDeviceManager); ok {
		return vdm.GetVirtualDevices()
	}
	return nil
}

// GetVirtualDevice retrieves a virtual device by its IEEE address if running on a mock adapter.
func (c *Controller) GetVirtualDevice(ieee string) (*fixture.VirtualDevice, bool) {
	if vdm, ok := c.adapter.(VirtualDeviceManager); ok {
		return vdm.GetVirtualDevice(ieee)
	}
	return nil, false
}

// GetBindings returns all active direct bindings.
func (c *Controller) GetBindings() []*binding.Binding {
	return c.bindings.ListBindings()
}

// EventBus returns the active real-time event bus.
func (c *Controller) EventBus() *EventBus {
	return c.eventBus
}

// EventCollector returns the telemetry event collector.
func (c *Controller) EventCollector() *ai.EventCollector {
	return c.collector
}

// SetAnalyzer configures a custom or external AI analyzer for on-demand recommendations.
func (c *Controller) SetAnalyzer(a ai.Analyzer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.analyzer = a
}

func toFloat64(val any) (float64, bool) {
	switch v := val.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case int:
		return float64(v), true
	default:
		return 0, false
	}
}

func (c *Controller) applyIASZoneStatus(dev *Device, status uint16, stateUpdates map[string]any) {
	alarm1 := (status & 0x0001) != 0
	alarm2 := (status & 0x0002) != 0
	tamper := (status & 0x0004) != 0
	batteryLow := (status & 0x0008) != 0
	deviceType := "leak"
	if dev != nil {
		if c.fixtures != nil && dev.Model != "" {
			if def, ok := c.fixtures.Get(dev.Model); ok {
				for _, exp := range def.Device.Exposes {
					if exp.Property == "contact" {
						deviceType = "contact"
						break
					} else if exp.Property == "water_leak" {
						deviceType = "leak"
						break
					} else if exp.Property == "smoke" {
						deviceType = "smoke"
						break
					} else if exp.Property == "occupancy" {
						deviceType = "occupancy"
						break
					}
				}
			}
		}
		if deviceType == "leak" {
			lowerModel := strings.ToLower(dev.Model + " " + dev.FriendlyName)
			if strings.Contains(lowerModel, "contact") || strings.Contains(lowerModel, "door") || strings.Contains(lowerModel, "window") || strings.Contains(lowerModel, "mccgq") {
				deviceType = "contact"
			} else if strings.Contains(lowerModel, "smoke") {
				deviceType = "smoke"
			} else if strings.Contains(lowerModel, "motion") || strings.Contains(lowerModel, "occupancy") {
				deviceType = "occupancy"
			}
		}
	}
	switch deviceType {
	case "contact":
		stateUpdates["contact"] = !alarm1
	case "smoke":
		stateUpdates["smoke"] = alarm1
	case "occupancy":
		stateUpdates["occupancy"] = alarm1
	default:
		stateUpdates["water_leak"] = alarm1
	}
	stateUpdates["tamper"] = tamper
	if batteryLow {
		stateUpdates["battery_low"] = true
	}
	if alarm2 {
		stateUpdates["alarm2"] = true
	}
}

func toUint16(val any) (uint16, bool) {
	switch v := val.(type) {
	case uint16:
		return v, true
	case uint8:
		return uint16(v), true
	case uint32:
		return uint16(v), true
	case int:
		return uint16(v), true
	case int16:
		return uint16(v), true
	case int32:
		return uint16(v), true
	default:
		return 0, false
	}
}
