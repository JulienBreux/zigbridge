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

func setupTestServerWithConfig(t *testing.T, cfg *config.Config) (*web.Server, *controller.Controller, string) {
	if cfg.Storage.DevicesPath == "" || cfg.Storage.DevicesPath == "data/devices.yaml" {
		cfg.Storage.DevicesPath = filepath.Join(t.TempDir(), "devices.yaml")
	}
	cfg.Web.ListenAddr = "127.0.0.1:0" // Random available port

	mockTrans, _ := transport.NewMockTransport()
	mockAdp := mock.New(15, 0x1A62)
	mockMQ := mqtt.NewMockClient()

	ctrl := controller.New(cfg, mockTrans, mockAdp, mockMQ)
	ctx := t.Context()
	if err := ctrl.Start(ctx); err != nil {
		t.Fatalf("failed to start controller: %v", err)
	}

	srv := web.NewServer(&cfg.Web, ctrl)
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("failed to start web server: %v", err)
	}

	addr := srv.Addr().String()
	baseURL := "http://" + addr

	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 2*time.Second)
		defer cancel()
		_ = srv.Stop(stopCtx)
		_ = ctrl.Stop()
	})

	return srv, ctrl, baseURL
}

func setupTestServer(t *testing.T) (*web.Server, *controller.Controller, string) {
	cfg := config.Default()
	return setupTestServerWithConfig(t, cfg)
}

func TestWebStaticAssets(t *testing.T) {
	_, _, baseURL := setupTestServer(t)

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
	_, _, baseURL := setupTestServer(t)

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
	if status.AIEnabled {
		t.Error("expected AIEnabled to be false in API status")
	}
}

func TestWebAPIPermitJoin(t *testing.T) {
	_, ctrl, baseURL := setupTestServer(t)

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
	_, ctrl, baseURL := setupTestServer(t)

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
	_, ctrl, baseURL := setupTestServer(t)

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

	var bindings []map[string]any
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
	cfg := config.Default()
	cfg.AI.Enabled = true
	_, ctrl, baseURL := setupTestServerWithConfig(t, cfg)

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

func TestWebAPIRecommendationsDisabled(t *testing.T) {
	_, _, baseURL := setupTestServer(t) // Default has AI.Enabled = false

	// GET /api/ai/recommendations should return 403 Forbidden
	recResp, err := http.Get(baseURL + "/api/ai/recommendations")
	if err != nil {
		t.Fatalf("failed to get recommendations: %v", err)
	}
	defer func() { _ = recResp.Body.Close() }()

	if recResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", recResp.StatusCode)
	}

	var errBody map[string]string
	if err := json.NewDecoder(recResp.Body).Decode(&errBody); err != nil {
		t.Fatalf("failed to decode error body: %v", err)
	}
	if errBody["error"] == "" {
		t.Error("expected non-empty error message")
	}

	// POST /api/ai/recommendations/{id}/apply should return 403 Forbidden
	applyResp, err := http.Post(baseURL+"/api/ai/recommendations/rec-123/apply", "application/json", bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("failed to post apply: %v", err)
	}
	defer func() { _ = applyResp.Body.Close() }()

	if applyResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden on apply, got %d", applyResp.StatusCode)
	}
}

func TestWebWebSocketLiveStream(t *testing.T) {
	srv, ctrl, _ := setupTestServer(t)

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

func TestWebSimulationLab(t *testing.T) {
	_, ctrl, baseURL := setupTestServer(t)

	// 1. GET /api/test/status
	statusResp, err := http.Get(baseURL + "/api/test/status")
	if err != nil {
		t.Fatalf("failed to get simulation status: %v", err)
	}
	defer func() { _ = statusResp.Body.Close() }()

	if statusResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for test status, got %d", statusResp.StatusCode)
	}

	var simStatus struct {
		Supported   bool   `json:"supported"`
		Adapter     string `json:"adapter"`
		Devices     int    `json:"devices"`
		Definitions int    `json:"definitions"`
	}
	if err := json.NewDecoder(statusResp.Body).Decode(&simStatus); err != nil {
		t.Fatalf("failed to decode sim status: %v", err)
	}
	if !simStatus.Supported {
		t.Fatalf("expected simulation to be supported with mock adapter")
	}
	if simStatus.Definitions < 1 {
		t.Fatalf("expected at least 1 fixture definition, got %d", simStatus.Definitions)
	}

	// 2. GET /api/test/definitions
	defsResp, err := http.Get(baseURL + "/api/test/definitions")
	if err != nil {
		t.Fatalf("failed to get definitions: %v", err)
	}
	defer func() { _ = defsResp.Body.Close() }()

	if defsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for definitions, got %d", defsResp.StatusCode)
	}

	var defs []struct {
		Model       string   `json:"model"`
		Vendor      string   `json:"vendor"`
		Description string   `json:"description"`
		Actions     []string `json:"actions"`
	}
	if err := json.NewDecoder(defsResp.Body).Decode(&defs); err != nil {
		t.Fatalf("failed to decode definitions: %v", err)
	}
	hasSNZB01P := false
	for _, d := range defs {
		if d.Model == "SNZB-01P" {
			hasSNZB01P = true
			break
		}
	}
	if !hasSNZB01P {
		t.Fatalf("expected SNZB-01P in definitions")
	}

	// 3. POST /api/test/devices (Spawn SNZB-01P)
	testIEEE := "0x00124b0001020304"
	spawnPayload := map[string]any{
		"model": "SNZB-01P",
		"ieee":  testIEEE,
		"nwk":   0x1234,
	}
	spawnBody, _ := json.Marshal(spawnPayload)
	spawnResp, err := http.Post(baseURL+"/api/test/devices", "application/json", bytes.NewBuffer(spawnBody))
	if err != nil {
		t.Fatalf("failed to spawn device: %v", err)
	}
	defer func() { _ = spawnResp.Body.Close() }()

	if spawnResp.StatusCode != http.StatusCreated {
		respBytes, _ := io.ReadAll(spawnResp.Body)
		t.Fatalf("expected 201 Created, got %d: %s", spawnResp.StatusCode, string(respBytes))
	}

	// 4. GET /api/test/devices
	devsResp, err := http.Get(baseURL + "/api/test/devices")
	if err != nil {
		t.Fatalf("failed to get virtual devices: %v", err)
	}
	defer func() { _ = devsResp.Body.Close() }()

	if devsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", devsResp.StatusCode)
	}

	var virtualDevices []struct {
		IEEE   string         `json:"ieee"`
		NWK    uint16         `json:"nwk"`
		Model  string         `json:"model"`
		Vendor string         `json:"vendor"`
		State  map[string]any `json:"state"`
	}
	if err := json.NewDecoder(devsResp.Body).Decode(&virtualDevices); err != nil {
		t.Fatalf("failed to decode virtual devices: %v", err)
	}
	if len(virtualDevices) != 1 {
		t.Fatalf("expected 1 virtual device, got %d", len(virtualDevices))
	}
	if virtualDevices[0].IEEE != testIEEE {
		t.Errorf("expected IEEE %s, got %s", testIEEE, virtualDevices[0].IEEE)
	}

	// Verify device also appears in controller registry
	dev, ok := ctrl.GetDevice(testIEEE)
	if !ok {
		t.Fatalf("expected device %s to be registered in controller", testIEEE)
	}
	if dev.Model != "SNZB-01P" {
		t.Errorf("expected controller device model 'SNZB-01P', got '%s'", dev.Model)
	}

	// 5. POST /api/test/devices/{ieee}/action
	actionPayload := map[string]string{"action": "single"}
	actionBody, _ := json.Marshal(actionPayload)
	actionResp, err := http.Post(fmt.Sprintf("%s/api/test/devices/%s/action", baseURL, testIEEE), "application/json", bytes.NewBuffer(actionBody))
	if err != nil {
		t.Fatalf("failed to trigger action: %v", err)
	}
	defer func() { _ = actionResp.Body.Close() }()

	if actionResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for action, got %d", actionResp.StatusCode)
	}

	// 6. POST /api/test/devices/{ieee}/telemetry
	batt := uint8(85)
	volt := uint16(2950)
	telemetryPayload := map[string]any{
		"battery": batt,
		"voltage": volt,
	}
	telemetryBody, _ := json.Marshal(telemetryPayload)
	telemetryResp, err := http.Post(fmt.Sprintf("%s/api/test/devices/%s/telemetry", baseURL, testIEEE), "application/json", bytes.NewBuffer(telemetryBody))
	if err != nil {
		t.Fatalf("failed to report telemetry: %v", err)
	}
	defer func() { _ = telemetryResp.Body.Close() }()

	if telemetryResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for telemetry, got %d", telemetryResp.StatusCode)
	}

	// Check device telemetry was updated
	vdev, ok := ctrl.GetVirtualDevice(testIEEE)
	if !ok {
		t.Fatalf("expected virtual device to exist")
	}
	if vdev.GetState()["battery"] != batt {
		t.Errorf("expected battery %d, got %v", batt, vdev.GetState()["battery"])
	}
}

func TestWebAPIDeviceDetail(t *testing.T) {
	_, ctrl, baseURL := setupTestServer(t)

	const ieee = "0x00158D00018899AA"
	ctrl.HandleDeviceJoin(adapter.DeviceJoinInfo{
		IEEE: ieee,
		NWK:  0x3456,
	})
	if dev, ok := ctrl.GetDevice(ieee); ok {
		dev.Model = "TS0041"
		dev.Manufacturer = "TuYa"
	}

	// 1. GET /api/devices/{ieee} - Found
	resp, err := http.Get(baseURL + "/api/devices/" + ieee)
	if err != nil {
		t.Fatalf("failed to get device detail: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var detail web.DeviceDetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("failed to decode detail response: %v", err)
	}
	if detail.Device == nil || detail.Device.IEEE != ieee {
		t.Fatalf("expected device IEEE %s, got %v", ieee, detail.Device)
	}
	if detail.Definition != nil && detail.Definition.Device.Model != "TS0041" {
		t.Errorf("expected definition model TS0041, got %s", detail.Definition.Device.Model)
	}

	// 2. GET /api/devices/{ieee} - 404 Not Found
	resp404, err := http.Get(baseURL + "/api/devices/0xNONEXISTENT")
	if err != nil {
		t.Fatalf("failed to call GET for nonexistent device: %v", err)
	}
	defer func() { _ = resp404.Body.Close() }()
	if resp404.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 Not Found, got %d", resp404.StatusCode)
	}
}

func TestWebAPIDeviceSetStateAndAction(t *testing.T) {
	_, ctrl, baseURL := setupTestServer(t)

	const ieee = "0x00158D0001AABBCC"
	ctrl.HandleDeviceJoin(adapter.DeviceJoinInfo{
		IEEE: ieee,
		NWK:  0x5678,
	})

	// 1. POST /api/devices/{ieee}/set
	setPayload := `{"state": "ON", "brightness": 128}`
	respSet, err := http.Post(baseURL+"/api/devices/"+ieee+"/set", "application/json", strings.NewReader(setPayload))
	if err != nil {
		t.Fatalf("failed to post set state: %v", err)
	}
	defer func() { _ = respSet.Body.Close() }()

	if respSet.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", respSet.StatusCode)
	}

	dev, ok := ctrl.GetDevice(ieee)
	if !ok {
		t.Fatalf("device %s not found", ieee)
	}
	if dev.State["state"] != "ON" || dev.State["brightness"] != float64(128) {
		t.Errorf("expected state ON and brightness 128, got: %v", dev.State)
	}

	// 2. POST /api/devices/{ieee}/action
	actionPayload := `{"action": "identify"}`
	respAction, err := http.Post(baseURL+"/api/devices/"+ieee+"/action", "application/json", strings.NewReader(actionPayload))
	if err != nil {
		t.Fatalf("failed to post action: %v", err)
	}
	defer func() { _ = respAction.Body.Close() }()

	if respAction.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", respAction.StatusCode)
	}

	// 3. Error handling: non-existent device set
	respErr, err := http.Post(baseURL+"/api/devices/0xNONEXISTENT/set", "application/json", strings.NewReader(setPayload))
	if err != nil {
		t.Fatalf("failed to post set to nonexistent: %v", err)
	}
	defer func() { _ = respErr.Body.Close() }()
	if respErr.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for nonexistent device, got %d", respErr.StatusCode)
	}
}

