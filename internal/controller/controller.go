package controller

import (
	"context"
	"fmt"
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
		c.eventBus.Publish("binding_change", map[string]interface{}{
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

// PermitJoin opens network joining for the given duration (in seconds, max 254).
func (c *Controller) PermitJoin(ctx context.Context, duration uint8) error {
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
	c.eventBus.Publish("permit_join", map[string]interface{}{
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
			c.eventBus.Publish("permit_join", map[string]interface{}{
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

	stateUpdates := make(map[string]interface{})

	// Parse attribute reports if present (Global profile commands)
	isAttributeReport := frame.Header.Type == zcl.FrameTypeGlobal &&
		(frame.CommandID == zcl.CmdReportAttributes || frame.CommandID == zcl.CmdReadAttributesResponse)

	if isAttributeReport {
		records, err := zcl.ParseAttributeReport(frame.Payload)
		if err == nil {
			for _, rec := range records {
				switch frame.ClusterID {
				case zcl.ClusterBasic:
					switch rec.AttributeID {
					case 0x0004: // Manufacturer Name
						if str, ok := rec.Value.(string); ok {
							existingDev, found := c.devices.Get(frame.SourceAddress)
							model := ""
							if found {
								model = existingDev.Model
							}
							c.devices.UpdateModelInfo(frame.SourceAddress, str, model)
							if c.store != nil {
								c.store.ScheduleSave()
							}
						}
					case 0x0005: // Model Identifier
						if str, ok := rec.Value.(string); ok {
							existingDev, found := c.devices.Get(frame.SourceAddress)
							mfg := ""
							if found {
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
					if val, ok := rec.Value.(uint16); ok {
						stateUpdates["power"] = val
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

	// Look up existing device
	existingDev, _ := c.devices.Get(frame.SourceAddress)

	// Check if this is an action command (non-report, cluster-specific or button command)
	if !isAttributeReport {
		matchedAction := false
		if c.fixtures != nil && existingDev != nil && existingDev.Model != "" {
			if def, ok := c.fixtures.Get(existingDev.Model); ok {
				for _, act := range def.Device.Simulations.Actions {
					if act.Cluster == uint16(frame.ClusterID) && act.Command == frame.CommandID {
						for k, v := range act.MQTTPayload {
							stateUpdates[k] = v
						}
						matchedAction = true
						break
					}
				}
			}
		}

		if !matchedAction && frame.ClusterID == zcl.ClusterOnOff {
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
		}
	}

	// Update device state in registry
	dev, _ := c.devices.UpdateState(frame.SourceAddress, stateUpdates, frame.LQI)

	friendlyName := frame.SourceAddress
	if dev != nil && dev.FriendlyName != "" {
		friendlyName = dev.FriendlyName
	}

	// Record telemetry event in AI bounded ring buffer
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
		c.eventBus.Publish("device_state", map[string]interface{}{
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
		State:          make(map[string]interface{}),
		Available:      true,
		LastSeen:       time.Now().UTC(),
	}

	c.devices.Upsert(dev)

	if c.store != nil {
		c.store.ScheduleSave()
	}

	// Publish Home Assistant auto-discovery entities
	c.publishDeviceDiscovery(dev)

	c.eventBus.Publish("device_join", dev.Clone())
}

// CreateDirectBinding executes a direct Zigbee binding with optimistic support.
// Returns the binding, any warnings (e.g. if the target was not fully discovered), and an error if failed.
func (c *Controller) CreateDirectBinding(ctx context.Context, req adapter.BindRequest) (*binding.Binding, []string, error) {
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
	return c.bindings.RemoveBinding(ctx, req)
}

// GetRecommendations inspects topology and historical events on-demand to propose bindings and scenes.
func (c *Controller) GetRecommendations(ctx context.Context) ([]ai.Recommendation, error) {
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

	return BridgeStatus{
		Connected:           c.transport.IsConnected(),
		TransportStatus:     string(c.transport.Status()),
		Coordinator:         c.adapter.Info(),
		DeviceCount:         len(c.devices.GetAll()),
		BindingCount:        len(c.bindings.ListBindings()),
		PermitJoinRemaining: uint8(c.permitJoinRemaining.Load()),
		MQTTConnected:       mqConnected,
		UptimeSeconds:       uptime,
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

// Store returns the persistent DeviceStore manager.
func (c *Controller) Store() *DeviceStore {
	return c.store
}

// publishDeviceDiscovery registers a device entity in Home Assistant via MQTT discovery.
func (c *Controller) publishDeviceDiscovery(dev *Device) {
	if !c.cfg.MQTT.Enabled || !c.cfg.MQTT.HADiscovery || c.mqtt == nil || !c.mqtt.IsConnected() || dev == nil {
		return
	}
	mfr := dev.Manufacturer
	if mfr == "" {
		mfr = "Zigbee Device"
	}
	model := dev.Model
	if model == "" {
		model = "Standard Endpoint"
	}
	haDev := mqtt.HADevice{
		Identifiers:  []string{dev.IEEE},
		Name:         dev.FriendlyName,
		Manufacturer: mfr,
		Model:        model,
	}

	baseTopic := c.cfg.MQTT.BaseTopic

	hasInCluster := func(cid zcl.ClusterID) bool {
		for _, cluster := range dev.InputClusters {
			if cluster == cid {
				return true
			}
		}
		return false
	}
	hasOutCluster := func(cid zcl.ClusterID) bool {
		for _, cluster := range dev.OutputClusters {
			if cluster == cid {
				return true
			}
		}
		return false
	}

	// Always publish default on/off if InputClusters has OnOff or by default for basic devices
	if hasInCluster(zcl.ClusterOnOff) || len(dev.InputClusters) == 0 {
		_ = c.mqtt.PublishDiscovery(mqtt.NewOnOffDiscovery(haDev, dev.IEEE, baseTopic))
	}

	// If device acts as a button controller (output cluster OnOff)
	if hasOutCluster(zcl.ClusterOnOff) {
		_ = c.mqtt.PublishDiscovery(mqtt.NewActionDiscovery(haDev, dev.IEEE, baseTopic))
		for _, action := range []string{"single", "double", "long"} {
			_ = c.mqtt.PublishDiscovery(mqtt.NewDeviceTriggerDiscovery(haDev, dev.IEEE, baseTopic, action))
		}
	}

	// Power & Battery
	if hasInCluster(zcl.ClusterPowerConfiguration) || dev.Battery > 0 {
		_ = c.mqtt.PublishDiscovery(mqtt.NewBatteryDiscovery(haDev, dev.IEEE, baseTopic))
		_ = c.mqtt.PublishDiscovery(mqtt.NewVoltageDiscovery(haDev, dev.IEEE, baseTopic))
	}

	// Temperature & Humidity
	if hasInCluster(zcl.ClusterTemperatureMeasurement) {
		_ = c.mqtt.PublishDiscovery(mqtt.NewTemperatureDiscovery(haDev, dev.IEEE, baseTopic))
	}
	if hasInCluster(zcl.ClusterRelativeHumidity) {
		_ = c.mqtt.PublishDiscovery(mqtt.NewHumidityDiscovery(haDev, dev.IEEE, baseTopic))
	}
}

// Fixtures returns the fixture registry containing loaded device definitions.
func (c *Controller) Fixtures() *fixture.Registry {
	return c.fixtures
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
