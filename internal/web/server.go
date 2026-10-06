package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/julienbreux/zigbridge/internal/adapter"
	"github.com/julienbreux/zigbridge/internal/config"
	"github.com/julienbreux/zigbridge/internal/controller"
	"github.com/julienbreux/zigbridge/internal/zcl"
)

//go:embed static/*
var staticFS embed.FS

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
	listener   net.Listener

	clientsMu sync.RWMutex
	clients   map[*websocket.Conn]struct{}

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewServer creates a new web server bound to the controller.
func NewServer(cfg *config.WebConfig, ctrl *controller.Controller) *Server {
	return &Server{
		cfg:        cfg,
		controller: ctrl,
		clients:    make(map[*websocket.Conn]struct{}),
		stopCh:     make(chan struct{}),
	}
}

// Start launches the HTTP server and event forwarding goroutine.
func (s *Server) Start(ctx context.Context) error {
	addr := s.cfg.ListenAddr
	if addr == "" {
		addr = "0.0.0.0:8080"
	}

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
	// Embedded static assets
	sub, err := fs.Sub(staticFS, "static")
	if err == nil {
		fileServer := http.FileServer(http.FS(sub))
		mux.Handle("GET /static/", http.StripPrefix("/static/", fileServer))
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" || r.URL.Path == "/index.html" {
				r.URL.Path = "/"
				fileServer.ServeHTTP(w, r)
				return
			}
			// Attempt to serve asset; if missing fallback to index.html
			fileServer.ServeHTTP(w, r)
		})
	}

	// REST APIs
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("POST /api/network/permit-join", s.handlePermitJoin)
	mux.HandleFunc("GET /api/devices", s.handleGetDevices)
	mux.HandleFunc("POST /api/devices/{ieee}/rename", s.handleDeviceRename)
	mux.HandleFunc("GET /api/bindings", s.handleGetBindings)
	mux.HandleFunc("POST /api/bindings", s.handleCreateBinding)
	mux.HandleFunc("DELETE /api/bindings", s.handleDeleteBinding)
	mux.HandleFunc("GET /api/ai/recommendations", s.handleGetRecommendations)
	mux.HandleFunc("POST /api/ai/recommendations/{id}/apply", s.handleApplyRecommendation)

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
		Time uint8 `json:"time"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	if err := s.controller.PermitJoin(r.Context(), body.Time); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"duration": body.Time,
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
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"ieee":          ieee,
		"friendly_name": body.FriendlyName,
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
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	resp := map[string]interface{}{
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
		w.WriteHeader(http.StatusInternalServerError)
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
		w.WriteHeader(http.StatusInternalServerError)
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
		w.WriteHeader(http.StatusBadRequest)
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
