package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/julienbreux/zigbridge/internal/adapter"
	"github.com/julienbreux/zigbridge/internal/adapter/mock"
	"github.com/julienbreux/zigbridge/internal/config"
	"github.com/julienbreux/zigbridge/internal/controller"
	"github.com/julienbreux/zigbridge/internal/mqtt"
	"github.com/julienbreux/zigbridge/internal/transport"
	"github.com/julienbreux/zigbridge/internal/web"
	"github.com/julienbreux/zigbridge/internal/zcl"
)

func setupTestServer(t *testing.T) (*web.Server, *controller.Controller, string, func()) {
	cfg := config.Default()
	cfg.Storage.DevicesPath = filepath.Join(t.TempDir(), "devices.yaml")
	cfg.Web.ListenAddr = "127.0.0.1:0" // Random available port

	mockTrans, _ := transport.NewMockTransport()
	mockAdp := mock.New(15, 0x1A62)
	mockMQ := mqtt.NewMockClient()

	ctrl := controller.New(cfg, mockTrans, mockAdp, mockMQ)
	ctx := context.Background()
	if err := ctrl.Start(ctx); err != nil {
		t.Fatalf("failed to start controller: %v", err)
	}

	srv := web.NewServer(&cfg.Web, ctrl)
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("failed to start web server: %v", err)
	}

	addr := srv.Addr().String()
	baseURL := fmt.Sprintf("http://%s", addr)

	teardown := func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Stop(stopCtx)
		_ = ctrl.Stop()
	}

	return srv, ctrl, baseURL, teardown
}

func TestWebStaticAssets(t *testing.T) {
	_, _, baseURL, teardown := setupTestServer(t)
	defer teardown()

	resp, err := http.Get(baseURL + "/")
	if err != nil {
		t.Fatalf("failed to get root: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Zigbridge") {
		t.Errorf("expected index.html to contain 'Zigbridge'")
	}
}

func TestWebAPIStatus(t *testing.T) {
	_, _, baseURL, teardown := setupTestServer(t)
	defer teardown()

	resp, err := http.Get(baseURL + "/api/status")
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var status controller.BridgeStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatalf("failed to decode status: %v", err)
	}

	if status.Coordinator.Channel != 15 {
		t.Errorf("expected channel 15, got %d", status.Coordinator.Channel)
	}
}

func TestWebAPIPermitJoin(t *testing.T) {
	_, ctrl, baseURL, teardown := setupTestServer(t)
	defer teardown()

	payload := `{"time": 45}`
	resp, err := http.Post(baseURL+"/api/network/permit-join", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("failed to post permit join: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	st := ctrl.Status()
	if st.PermitJoinRemaining == 0 {
		t.Errorf("expected non-zero permit join remaining, got %d", st.PermitJoinRemaining)
	}
}

func TestWebAPIDevicesAndRename(t *testing.T) {
	_, ctrl, baseURL, teardown := setupTestServer(t)
	defer teardown()

	// Register device via join handler
	ctrl.HandleDeviceJoin(adapter.DeviceJoinInfo{
		IEEE: "0x00158D0001",
		NWK:  0x1234,
	})

	// GET devices
	resp, err := http.Get(baseURL + "/api/devices")
	if err != nil {
		t.Fatalf("failed to get devices: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var devs []*controller.Device
	if err := json.NewDecoder(resp.Body).Decode(&devs); err != nil {
		t.Fatalf("failed to decode devices: %v", err)
	}

	if len(devs) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devs))
	}

	// Rename device
	renamePayload := `{"friendly_name": "Kitchen Light"}`
	renameResp, err := http.Post(baseURL+"/api/devices/0x00158D0001/rename", "application/json", strings.NewReader(renamePayload))
	if err != nil {
		t.Fatalf("failed to post rename: %v", err)
	}
	defer func() { _ = renameResp.Body.Close() }()

	if renameResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for rename, got %d", renameResp.StatusCode)
	}

	// Verify rename
	all := ctrl.GetDevices()
	if len(all) == 0 || all[0].FriendlyName != "Kitchen Light" {
		t.Errorf("expected friendly name Kitchen Light, got %s", all[0].FriendlyName)
	}
}

func TestWebAPIBindings(t *testing.T) {
	_, ctrl, baseURL, teardown := setupTestServer(t)
	defer teardown()

	// Populate devices
	ctrl.HandleDeviceJoin(adapter.DeviceJoinInfo{IEEE: "0xSW01", NWK: 0x2001})
	ctrl.HandleDeviceJoin(adapter.DeviceJoinInfo{IEEE: "0xLT01", NWK: 0x2002})

	// POST /api/bindings
	bindJSON := `{
		"source_ieee": "0xSW01",
		"source_ep": 1,
		"target_ieee": "0xLT01",
		"target_ep": 1,
		"cluster_id": 6
	}`

	postResp, err := http.Post(baseURL+"/api/bindings", "application/json", strings.NewReader(bindJSON))
	if err != nil {
		t.Fatalf("failed to post binding: %v", err)
	}
	defer func() { _ = postResp.Body.Close() }()

	if postResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", postResp.StatusCode)
	}

	// GET /api/bindings
	getResp, err := http.Get(baseURL + "/api/bindings")
	if err != nil {
		t.Fatalf("failed to get bindings: %v", err)
	}
	defer func() { _ = getResp.Body.Close() }()

	var bindings []map[string]interface{}
	if err := json.NewDecoder(getResp.Body).Decode(&bindings); err != nil {
		t.Fatalf("failed to decode bindings: %v", err)
	}

	if len(bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(bindings))
	}

	// DELETE /api/bindings
	req, err := http.NewRequest(http.MethodDelete, baseURL+"/api/bindings", strings.NewReader(bindJSON))
	if err != nil {
		t.Fatalf("failed to create delete request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to delete binding: %v", err)
	}
	defer func() { _ = delResp.Body.Close() }()

	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on delete, got %d", delResp.StatusCode)
	}

	if len(ctrl.GetBindings()) != 0 {
		t.Errorf("expected 0 bindings remaining, got %d", len(ctrl.GetBindings()))
	}
}

func TestWebAPIRecommendationsAndApply(t *testing.T) {
	_, ctrl, baseURL, teardown := setupTestServer(t)
	defer teardown()

	// Register compatible switch and bulb
	ctrl.HandleDeviceJoin(adapter.DeviceJoinInfo{IEEE: "0x00158D0001", NWK: 0x1001})
	ctrl.HandleDeviceJoin(adapter.DeviceJoinInfo{IEEE: "0x00158D0002", NWK: 0x1002})

	ctrl.UpdateDeviceMetadata("0x00158D0001", []uint16{1}, nil, []zcl.ClusterID{zcl.ClusterOnOff})
	ctrl.UpdateDeviceMetadata("0x00158D0002", []uint16{1}, []zcl.ClusterID{zcl.ClusterOnOff}, nil)

	// GET /api/ai/recommendations
	recResp, err := http.Get(baseURL + "/api/ai/recommendations")
	if err != nil {
		t.Fatalf("failed to get recommendations: %v", err)
	}
	defer func() { _ = recResp.Body.Close() }()

	var recs []struct {
		ID         string  `json:"id"`
		SourceIEEE string  `json:"source_ieee"`
		TargetIEEE string  `json:"target_ieee"`
		Confidence float64 `json:"confidence"`
	}
	if err := json.NewDecoder(recResp.Body).Decode(&recs); err != nil {
		t.Fatalf("failed to decode recommendations: %v", err)
	}

	if len(recs) == 0 {
		t.Fatalf("expected at least 1 recommendation")
	}

	// POST /api/ai/recommendations/{id}/apply
	applyURL := fmt.Sprintf("%s/api/ai/recommendations/%s/apply", baseURL, recs[0].ID)
	applyResp, err := http.Post(applyURL, "application/json", bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("failed to apply recommendation: %v", err)
	}
	defer func() { _ = applyResp.Body.Close() }()

	if applyResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", applyResp.StatusCode)
	}

	if len(ctrl.GetBindings()) != 1 {
		t.Errorf("expected 1 binding created from recommendation, got %d", len(ctrl.GetBindings()))
	}
}

func TestWebWebSocketLiveStream(t *testing.T) {
	srv, ctrl, _, teardown := setupTestServer(t)
	defer teardown()

	wsURL := fmt.Sprintf("ws://%s/api/events", srv.Addr().String())
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Wait briefly for connection setup
	time.Sleep(50 * time.Millisecond)

	// Trigger event on bus
	testMsg := "telemetry_test_payload"
	ctrl.EventBus().Publish("test_event", testMsg)

	// Read message from WS
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read WS message: %v", err)
	}

	if !strings.Contains(string(msg), "test_event") || !strings.Contains(string(msg), testMsg) {
		t.Errorf("expected WS message to contain test_event and payload, got: %s", string(msg))
	}
}
