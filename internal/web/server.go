package web

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"net"
	"net/http"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/julienbreux/zigbridge/internal/adapter"
	"github.com/julienbreux/zigbridge/internal/config"
	"github.com/julienbreux/zigbridge/internal/controller"
	"github.com/julienbreux/zigbridge/internal/fixture"
	"github.com/julienbreux/zigbridge/internal/zcl"
	"github.com/julienbreux/zigbridge/webui"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow embedded and local LAN web dashboard origins
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// BindPayload represents incoming JSON for direct binding operations.
type BindPayload struct {
	SourceIEEE     string        `json:"source_ieee"`
	SourceEndpoint uint8         `json:"source_ep"`
	TargetIEEE     string        `json:"target_ieee"`
	TargetEndpoint uint8         `json:"target_ep"`
	ClusterID      zcl.ClusterID `json:"cluster_id"`
}

// Server provides the embedded HTTP dashboard and REST/WebSocket API.
type Server struct {
	cfg        *config.WebConfig
	controller *controller.Controller
	httpServer *http.Server
	listener net.Listener
	staticMu sync.RWMutex
	staticFS fs.FS

	clientsMu sync.RWMutex
	clients   map[*websocket.Conn]struct{}

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewServer creates a new web server bound to the controller.
func NewServer(cfg *config.WebConfig, ctrl *controller.Controller) *Server {
	dist, _ := webui.DistFS()
	return &Server{
		cfg:        cfg,
		controller: ctrl,
		staticFS:   dist,
		clients:    make(map[*websocket.Conn]struct{}),
		stopCh:     make(chan struct{}),
	}
}

// SetStaticFS overrides the static assets filesystem (useful for testing or custom distribution).
func (s *Server) SetStaticFS(staticFS fs.FS) {
	s.staticMu.Lock()
	s.staticFS = staticFS
	s.staticMu.Unlock()
}

func (s *Server) getStaticFS() fs.FS {
	s.staticMu.RLock()
	defer s.staticMu.RUnlock()
	return s.staticFS
}

// Start launches the HTTP server and event forwarding goroutine.
func (s *Server) Start(ctx context.Context) error {
	addr := cmp.Or(s.cfg.ListenAddr, "0.0.0.0:8080")

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	s.listener = ln

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	var handler http.Handler = mux
	if s.cfg.EnableCORS {
		handler = s.corsMiddleware(mux)
	}

	s.httpServer = &http.Server{
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start WebSocket event broadcaster loop
	s.wg.Add(1)
	go s.eventBroadcasterLoop()

	// Start HTTP server in background
	go func() {
		if err := s.httpServer.Serve(s.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// Log server termination
			s.controller.EventBus().Publish("system_warning", fmt.Sprintf("Web server terminated: %v", err))
		}
	}()

	return nil
}

// Stop cleanly terminates the HTTP server and active WebSocket connections.
func (s *Server) Stop(ctx context.Context) error {
	close(s.stopCh)

	// Close all active WebSocket connections
	s.clientsMu.Lock()
	for conn := range s.clients {
		_ = conn.Close()
	}
	s.clients = make(map[*websocket.Conn]struct{})
	s.clientsMu.Unlock()

	s.wg.Wait()

	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// Addr returns the actual listening address (useful for port 0 testing).
func (s *Server) Addr() net.Addr {
	if s.listener != nil {
		return s.listener.Addr()
	}
	return nil
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Embedded static assets and SPA fallback
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"endpoint not found"}`))
			return
		}

		staticFS := s.getStaticFS()
		if staticFS == nil {
			http.NotFound(w, r)
			return
		}

		fileServer := http.FileServer(http.FS(staticFS))
		cleanPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if cleanPath == "" || cleanPath == "." || cleanPath == "index.html" {
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}

		if f, err := staticFS.Open(cleanPath); err == nil {
			stat, statErr := f.Stat()
			_ = f.Close()
			if statErr == nil && !stat.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		if strings.HasPrefix(r.URL.Path, "/assets/") || strings.HasSuffix(r.URL.Path, ".ico") {
			http.NotFound(w, r)
			return
		}

		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})

	// REST APIs
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("POST /api/network/permit-join", s.handlePermitJoin)
	mux.HandleFunc("GET /api/devices", s.handleGetDevices)
	mux.HandleFunc("GET /api/devices/{ieee}", s.handleGetDeviceDetail)
	mux.HandleFunc("POST /api/devices/{ieee}/rename", s.handleDeviceRename)
	mux.HandleFunc("POST /api/devices/{ieee}/set", s.handleDeviceSetState)
	mux.HandleFunc("POST /api/devices/{ieee}/action", s.handleDeviceAction)
	mux.HandleFunc("GET /api/bindings", s.handleGetBindings)
	mux.HandleFunc("POST /api/bindings", s.handleCreateBinding)
	mux.HandleFunc("DELETE /api/bindings", s.handleDeleteBinding)
	mux.HandleFunc("GET /api/ai/recommendations", s.handleGetRecommendations)
	mux.HandleFunc("POST /api/ai/recommendations/{id}/apply", s.handleApplyRecommendation)

	// Simulation Lab APIs (Mock mode testing)
	mux.HandleFunc("GET /api/test/status", s.handleTestStatus)
	mux.HandleFunc("GET /api/test/definitions", s.handleGetDefinitions)
	mux.HandleFunc("GET /api/test/devices", s.handleGetVirtualDevices)
	mux.HandleFunc("POST /api/test/devices", s.handleSpawnVirtualDevice)
	mux.HandleFunc("POST /api/test/devices/{ieee}/action", s.handleVirtualDeviceAction)
	mux.HandleFunc("POST /api/test/devices/{ieee}/telemetry", s.handleVirtualDeviceTelemetry)

	// WebSocket live stream
	mux.HandleFunc("GET /api/events", s.handleWebSocket)
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// REST Handlers

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := s.controller.Status()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

func (s *Server) handlePermitJoin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Time     uint8 `json:"time"`
		Duration uint8 `json:"duration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	duration := cmp.Or(body.Time, body.Duration)

	if err := s.controller.PermitJoin(r.Context(), duration); err != nil {
		w.Header().Set("Content-Type", "application/json")
		if errors.Is(err, controller.ErrCoordinatorNotConnected) {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":  true,
		"duration": duration,
	})
}

func (s *Server) handleGetDevices(w http.ResponseWriter, r *http.Request) {
	devices := s.controller.GetDevices()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(devices)
}

func (s *Server) handleDeviceRename(w http.ResponseWriter, r *http.Request) {
	ieee := r.PathValue("ieee")
	if ieee == "" {
		http.Error(w, `{"error":"missing ieee path parameter"}`, http.StatusBadRequest)
		return
	}

	var body struct {
		FriendlyName string `json:"friendly_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	if !s.controller.SetDeviceFriendlyName(ieee, body.FriendlyName) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "device not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":       true,
		"ieee":          ieee,
		"friendly_name": body.FriendlyName,
	})
}

// DeviceDetailResponse bundles device state and its fixture definition if available.
type DeviceDetailResponse struct {
	Device     *controller.Device        `json:"device"`
	Definition *fixture.DeviceDefinition `json:"definition,omitempty"`
}

func (s *Server) handleGetDeviceDetail(w http.ResponseWriter, r *http.Request) {
	ieee := r.PathValue("ieee")
	if ieee == "" {
		http.Error(w, `{"error":"missing ieee path parameter"}`, http.StatusBadRequest)
		return
	}

	dev, ok := s.controller.GetDevice(ieee)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "device not found"})
		return
	}

	resp := DeviceDetailResponse{
		Device: dev,
	}

	if s.controller.Fixtures() != nil && dev.Model != "" {
		if def, found := s.controller.Fixtures().Get(dev.Model); found {
			resp.Definition = def
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleDeviceSetState(w http.ResponseWriter, r *http.Request) {
	ieee := r.PathValue("ieee")
	if ieee == "" {
		http.Error(w, `{"error":"missing ieee path parameter"}`, http.StatusBadRequest)
		return
	}

	var updates map[string]any
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	updatedDev, err := s.controller.SetDeviceState(r.Context(), ieee, updates)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if errors.Is(err, controller.ErrCoordinatorNotConnected) {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"device":  updatedDev,
	})
}

func (s *Server) handleDeviceAction(w http.ResponseWriter, r *http.Request) {
	ieee := r.PathValue("ieee")
	if ieee == "" {
		http.Error(w, `{"error":"missing ieee path parameter"}`, http.StatusBadRequest)
		return
	}

	var body struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	if body.Action == "" {
		http.Error(w, `{"error":"action cannot be empty"}`, http.StatusBadRequest)
		return
	}

	if err := s.controller.TriggerDeviceAction(r.Context(), ieee, body.Action); err != nil {
		w.Header().Set("Content-Type", "application/json")
		if errors.Is(err, controller.ErrCoordinatorNotConnected) {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"action":  body.Action,
	})
}

func (s *Server) handleGetBindings(w http.ResponseWriter, r *http.Request) {
	bindings := s.controller.GetBindings()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(bindings)
}

func (s *Server) handleCreateBinding(w http.ResponseWriter, r *http.Request) {
	var body BindPayload
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	req := adapter.BindRequest{
		SrcIEEE:     body.SourceIEEE,
		SrcEndpoint: body.SourceEndpoint,
		ClusterID:   body.ClusterID,
		DstIEEE:     body.TargetIEEE,
		DstEndpoint: body.TargetEndpoint,
	}

	b, warnings, err := s.controller.CreateDirectBinding(r.Context(), req)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if errors.Is(err, controller.ErrCoordinatorNotConnected) {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	resp := map[string]any{
		"success": true,
		"binding": b,
	}
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleDeleteBinding(w http.ResponseWriter, r *http.Request) {
	var body BindPayload
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	req := adapter.BindRequest{
		SrcIEEE:     body.SourceIEEE,
		SrcEndpoint: body.SourceEndpoint,
		ClusterID:   body.ClusterID,
		DstIEEE:     body.TargetIEEE,
		DstEndpoint: body.TargetEndpoint,
	}

	if err := s.controller.RemoveDirectBinding(r.Context(), req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		if errors.Is(err, controller.ErrCoordinatorNotConnected) {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (s *Server) handleGetRecommendations(w http.ResponseWriter, r *http.Request) {
	recs, err := s.controller.GetRecommendations(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if errors.Is(err, controller.ErrAIDisabled) {
			w.WriteHeader(http.StatusForbidden)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(recs)
}

func (s *Server) handleApplyRecommendation(w http.ResponseWriter, r *http.Request) {
	recID := r.PathValue("id")
	if recID == "" {
		http.Error(w, `{"error":"missing recommendation id"}`, http.StatusBadRequest)
		return
	}

	if err := s.controller.ApplyRecommendation(r.Context(), recID); err != nil {
		w.Header().Set("Content-Type", "application/json")
		if errors.Is(err, controller.ErrCoordinatorNotConnected) {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else if errors.Is(err, controller.ErrAIDisabled) {
			w.WriteHeader(http.StatusForbidden)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// WebSocket Live Stream

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	s.clientsMu.Lock()
	s.clients[conn] = struct{}{}
	s.clientsMu.Unlock()

	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, conn)
		s.clientsMu.Unlock()
		_ = conn.Close()
	}()

	// Read loop to detect disconnects and respond to keepalives
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (s *Server) eventBroadcasterLoop() {
	defer s.wg.Done()

	sub := s.controller.EventBus().Subscribe(128)
	defer s.controller.EventBus().Unsubscribe(sub)

	for {
		select {
		case <-s.stopCh:
			return
		case event, ok := <-sub:
			if !ok {
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}

			s.clientsMu.RLock()
			for conn := range s.clients {
				_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
				_ = conn.WriteMessage(websocket.TextMessage, data)
			}
			s.clientsMu.RUnlock()
		}
	}
}

// Simulation Lab Handlers

// DefinitionSummary provides device definition overview for simulation UI.
type DefinitionSummary struct {
	Model          string   `json:"model"`
	Vendor         string   `json:"vendor"`
	Description    string   `json:"description"`
	ZigbeeModels   []string `json:"zigbee_models"`
	Actions        []string `json:"actions"`
	HasBattery     bool     `json:"has_battery"`
	HasTemperature bool     `json:"has_temperature"`
	HasHumidity    bool     `json:"has_humidity"`
}

// VirtualDeviceSummary provides virtual device runtime overview for simulation UI.
type VirtualDeviceSummary struct {
	IEEE           string         `json:"ieee"`
	NWK            uint16         `json:"nwk"`
	Model          string         `json:"model"`
	Vendor         string         `json:"vendor"`
	Description    string         `json:"description"`
	State          map[string]any `json:"state"`
	Actions        []string       `json:"actions"`
	HasBattery     bool           `json:"has_battery"`
	HasTemperature bool           `json:"has_temperature"`
	HasHumidity    bool           `json:"has_humidity"`
}

// SpawnDevicePayload describes parameters to instantiate a virtual device.
type SpawnDevicePayload struct {
	Model string `json:"model"`
	IEEE  string `json:"ieee,omitempty"`
	NWK   uint16 `json:"nwk,omitempty"`
}

// VirtualTelemetryPayload describes telemetry simulation inputs.
type VirtualTelemetryPayload struct {
	Battery     *uint8   `json:"battery,omitempty"`
	Voltage     *uint16  `json:"voltage,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	Humidity    *float64 `json:"humidity,omitempty"`
}

func (s *Server) handleTestStatus(w http.ResponseWriter, r *http.Request) {
	vdevs := s.controller.GetVirtualDevices()
	count := 0
	if vdevs != nil {
		count = len(vdevs)
	}
	defCount := 0
	if s.controller.Fixtures() != nil {
		defCount = len(s.controller.Fixtures().List())
	}
	adapterType := "hardware"
	if s.controller.IsSimulationSupported() {
		adapterType = "mock"
	}
	resp := map[string]any{
		"supported":              s.controller.IsSimulationSupported(),
		"adapter":                adapterType,
		"devices":                count,
		"active_virtual_devices": count,
		"definitions":            defCount,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleGetDefinitions(w http.ResponseWriter, r *http.Request) {
	if s.controller.Fixtures() == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]DefinitionSummary{})
		return
	}

	defs := s.controller.Fixtures().List()
	res := make([]DefinitionSummary, len(defs))
	for i, d := range defs {
		actions := make([]string, 0, len(d.Device.Simulations.Actions))
		for a := range d.Device.Simulations.Actions {
			actions = append(actions, a)
		}
		slices.Sort(actions)

		_, hasBatt := d.Device.Simulations.Telemetry["battery"]
		_, hasTemp := d.Device.Simulations.Telemetry["temperature"]
		_, hasHum := d.Device.Simulations.Telemetry["humidity"]

		res[i] = DefinitionSummary{
			Model:          d.Device.Model,
			Vendor:         d.Device.Vendor,
			Description:    d.Device.Description,
			ZigbeeModels:   d.Device.ZigbeeModels,
			Actions:        actions,
			HasBattery:     hasBatt,
			HasTemperature: hasTemp,
			HasHumidity:    hasHum,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleGetVirtualDevices(w http.ResponseWriter, r *http.Request) {
	vdevs := s.controller.GetVirtualDevices()
	if vdevs == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]VirtualDeviceSummary{})
		return
	}

	res := make([]VirtualDeviceSummary, len(vdevs))
	for i, v := range vdevs {
		def := v.Def
		actions := make([]string, 0, len(def.Device.Simulations.Actions))
		for a := range def.Device.Simulations.Actions {
			actions = append(actions, a)
		}
		slices.Sort(actions)

		_, hasBatt := def.Device.Simulations.Telemetry["battery"]
		_, hasTemp := def.Device.Simulations.Telemetry["temperature"]
		_, hasHum := def.Device.Simulations.Telemetry["humidity"]

		res[i] = VirtualDeviceSummary{
			IEEE:           v.IEEE,
			NWK:            v.NWK,
			Model:          def.Device.Model,
			Vendor:         def.Device.Vendor,
			Description:    def.Device.Description,
			State:          v.GetState(),
			Actions:        actions,
			HasBattery:     hasBatt,
			HasTemperature: hasTemp,
			HasHumidity:    hasHum,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleSpawnVirtualDevice(w http.ResponseWriter, r *http.Request) {
	if !s.controller.IsSimulationSupported() {
		http.Error(w, `{"error":"simulation not supported on current adapter"}`, http.StatusBadRequest)
		return
	}

	var body SpawnDevicePayload
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	if body.Model == "" {
		http.Error(w, `{"error":"missing model in request payload"}`, http.StatusBadRequest)
		return
	}

	if s.controller.Fixtures() == nil {
		http.Error(w, `{"error":"no fixture registry loaded"}`, http.StatusInternalServerError)
		return
	}

	def, ok := s.controller.Fixtures().Get(body.Model)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("fixture definition for model '%s' not found", body.Model)})
		return
	}

	ieee := body.IEEE
	if ieee == "" {
		ieee = fmt.Sprintf("0x00124b%010x", rand.Uint64()&0xffffffffff)
	}
	nwk := body.NWK
	if nwk == 0 {
		nwk = uint16(rand.IntN(0xff00) + 0x0010)
	}

	vdev, err := s.controller.SpawnVirtualDevice(def, ieee, nwk)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if err := vdev.SimulateJoin(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to simulate join: %v", err)})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"ieee":    ieee,
		"nwk":     nwk,
		"model":   def.Device.Model,
		"vendor":  def.Device.Vendor,
	})
}

func (s *Server) handleVirtualDeviceAction(w http.ResponseWriter, r *http.Request) {
	ieee := r.PathValue("ieee")
	if ieee == "" {
		http.Error(w, `{"error":"missing ieee path parameter"}`, http.StatusBadRequest)
		return
	}

	vdev, ok := s.controller.GetVirtualDevice(ieee)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "virtual device not found"})
		return
	}

	var body struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	if body.Action == "" {
		http.Error(w, `{"error":"action cannot be empty"}`, http.StatusBadRequest)
		return
	}

	if err := vdev.TriggerAction(body.Action); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"action":  body.Action,
		"state":   vdev.GetState(),
	})
}

func (s *Server) handleVirtualDeviceTelemetry(w http.ResponseWriter, r *http.Request) {
	ieee := r.PathValue("ieee")
	if ieee == "" {
		http.Error(w, `{"error":"missing ieee path parameter"}`, http.StatusBadRequest)
		return
	}

	vdev, ok := s.controller.GetVirtualDevice(ieee)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "virtual device not found"})
		return
	}

	var body VirtualTelemetryPayload
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	if body.Battery != nil || body.Voltage != nil {
		batt := uint8(100)
		volt := uint16(3000)
		if body.Battery != nil {
			batt = *body.Battery
		}
		if body.Voltage != nil {
			volt = *body.Voltage
		}
		if err := vdev.ReportBattery(batt, volt); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
	}

	if body.Temperature != nil || body.Humidity != nil {
		temp := 20.0
		hum := 50.0
		if body.Temperature != nil {
			temp = *body.Temperature
		}
		if body.Humidity != nil {
			hum = *body.Humidity
		}
		if err := vdev.ReportTemperatureHumidity(temp, hum); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"state":   vdev.GetState(),
	})
}
