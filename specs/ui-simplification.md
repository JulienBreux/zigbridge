# Spec: Embedded Dashboard UI Simplification & Advanced Mode

## Objective

Redesign the embedded web dashboard in [`internal/web/static/`](file:///Users/julienbreux/Projects/julienbreux/zigbridge/internal/web/static/) to provide a friendly, intuitive interface for **non-technical users** by default, while introducing an **Advanced Mode** toggle for **technical users** who require coordinator network diagnostics, raw Zigbee Cluster Library (ZCL) telemetry, and low-level addressing.

Specifically:
1. **Simplified Header**: By default, display only essential status indicators: **Coordinator Status** (Online/Connecting/Offline) and **MQTT Status** (Connected/Disabled/Offline), plus the **Permit Join** pairing action and an **Advanced Mode** toggle. Hide raw PAN ID, Channel, and transport technical details.
2. **Devices as Default Landing**: Hide the technical "Overview" tab in Simple Mode. Directly load and display the **Devices** tab upon opening the dashboard.
3. **Activity Feed**: Rename "Live Stream" to **Activity**. In Simple Mode, display friendly, human-readable event descriptions (e.g., *"Living Room Light turned ON"*, *"Motion detected"*, *"New device paired"*). In Advanced Mode, offer raw JSON/hex frame logs.
4. **Non-Technical vs. Technical Profile Views**:
   - **Simple Mode (Default)**: Clean device cards or simplified table showing Friendly Name, Device Type/Icon, State/Power, Battery %, friendly Link Quality (Good/Fair/Poor), and Rename action. Direct Bindings presented as intuitive "Device-to-Device Links".
   - **Advanced Mode**: Unhides the Overview tab (full coordinator diagnostics, PAN ID, Ext PAN ID, Channel, NVRAM, Transport info). Restores technical table columns in Devices (IEEE 64-bit address, 16-bit NWK, Endpoints, raw In/Out Cluster IDs, numeric LQI 0–255, raw state payloads). Provides raw WebSocket frame console in Activity.
5. **Persistence**: Store the user's selected mode (`simple` vs `advanced`) in browser `localStorage`.
6. **Zero-CDN Offline Design**: Keep 100% offline capability with no external font or script CDNs.

---

## Assumptions

1. **Client-Side Toggle**: The mode switch is entirely client-side in vanilla JavaScript and CSS, requiring no backend Go API changes (all REST and WebSocket endpoints continue serving complete data).
2. **Backward Compatibility**: Existing backend REST endpoints (`/api/status`, `/api/devices`, `/api/bindings`, `/api/ai/recommendations`) and the `/api/events` WebSocket stream remain unchanged.
3. **Single Embedded Binary**: All changes remain embedded in the single Go binary via `embed.FS` in [`internal/web/server.go`](file:///Users/julienbreux/Projects/julienbreux/zigbridge/internal/web/server.go).
4. **Default Mode**: New visitors land in **Simple Mode** with the **Devices** tab active.

---

## Tech Stack & Architecture

- **Backend**: Go 1.22+ standard library + `embed.FS` (unchanged).
- **Frontend**:
  - `index.html`: Semantic HTML5 single-page application structure.
  - `style.css`: Vanilla CSS with CSS custom properties (variables), modern card layouts, toggle switches, and responsive breakpoints.
  - `app.js`: Vanilla ES6+ handling REST requests, WebSocket stream, local state, tab navigation, friendly event translation, and `localStorage` persistence.

---

## Commands

```bash
# Build the binary with updated embedded static assets
make build

# Run unit tests to ensure web server and API tests pass
make test

# Run static analysis
make lint

# Run the app locally for manual verification
./bin/zigbridge -config config.yaml
```

---

## Project Structure

```
internal/web/
├── server.go              # Embedded filesystem & HTTP/WebSocket server
├── server_test.go         # Web server unit & integration tests
└── static/
    ├── index.html         # Simplified HTML structure with advanced mode containers
    ├── style.css          # CSS styles for simple/advanced views and toggle switch
    └── app.js             # UI controller, event formatter, and mode state management
```

---

## Code Style & UX Specification

### 1. Header Layout
- **Left**: ZigBridge logo and title.
- **Center / Status Indicators**:
  - **Coordinator Status Badge**: Simple pill with colored pulse dot (`● Online` / `○ Connecting` / `✕ Offline`).
  - **MQTT Status Badge**: Simple pill with colored pulse dot (`● MQTT Connected` / `○ MQTT Disabled`).
  - *(In Advanced Mode only)*: Additional badge showing `CH 20 | PAN 0x1A62 | TCP`.
- **Right**:
  - **Permit Join Button**: Prominent action button with countdown timer.
  - **Advanced Mode Switch**: Modern accessible pill toggle `[ ⚙️ Advanced ]` that toggles `.mode-advanced` class on `document.body` and saves to `localStorage.getItem('zigbridge_mode')`.

### 2. Navigation Tabs & Diagnostics Drawer
- **Tabs (Clean & Consistent)**:
  1. **Devices** *(Default active tab on load)*
  2. **Direct Bindings** *(Subtitled "Device Links")*
  3. **Smart Suggestions** *(AI Recommendations)*
  4. **Activity** *(Renamed from Live Stream)*
- **Diagnostics Drawer (Overview)**:
  - Rather than cluttering tabs with an "Overview" tab, the complete Coordinator & Network architecture diagnostics (Channel, PAN ID, Ext PAN ID, Firmware, Transport link, NVRAM state) are housed in an expandable **Diagnostics Drawer**.
  - In Simple Mode, the drawer is tucked away.
  - In Advanced Mode, a **"Diagnostics"** pill button appears in the header (or can be toggled open/closed) providing immediate access to the full network and coordinator telemetry.

### 3. Devices View
- **Simple Mode**:
  - Clean table or card grid focusing on what the user cares about:
    - Device Icon + Friendly Name (click to rename)
    - Device Type (Light, Switch, Sensor, Plug, etc., inferred from clusters/model)
    - Power / State (e.g. `ON` / `OFF`, temperature reading, occupancy)
    - Battery level badge (e.g., `95%` or `Mains-powered`)
    - Signal rating (`Excellent`, `Good`, `Fair`, `Poor` based on LQI)
    - Action: `Rename` button
  - Hides technical columns: IEEE address, NWK address, endpoints, raw cluster IDs.
- **Advanced Mode**:
  - Restores all technical columns: Full 64-bit IEEE, 16-bit NWK, Model/Vendor strings, Endpoint numbers, Cluster IDs (e.g., `0x0006`, `0x0008`), numerical LQI (0–255), and JSON state payload.

### 4. Activity View (Formerly "Live Stream")
- **Simple Mode**:
  - Displays a clean, human-readable timeline:
    - *"🛋️ Living Room Lamp turned ON"*
    - *"🚶 Hallway Motion Sensor detected movement"*
    - *"✨ New device connected: Smart Bulb (0x00124b...)"*
    - *"🔓 Device pairing opened for 60 seconds"*
    - *"🔗 Direct binding linked Switch to Lamp"*
  - Friendly relative timestamps (*"Just now"*, *"2 min ago"*).
- **Advanced Mode**:
  - Shows full technical message: Event Type, Source IEEE, Cluster ID, Command ID, Raw JSON/Hex payload.
  - Filter by event type, "Clear Console" and "Pause Stream" buttons.

### 5. Direct Bindings View
- **Simple Mode**:
  - Friendly explanation: *"Direct bindings let switches control lights directly with zero delay, even if the hub is offline."*
  - Create modal with simple dropdowns: "Which switch?" $\to$ "Which light?" $\to$ "What action? (Turn On/Off, Dim, Color)".
- **Advanced Mode**:
  - Exposes manual endpoint selection (EP 1, EP 2) and cluster selection (0x0006, 0x0008, 0x0300) with optimistic binding warning details.

---

## Testing Strategy

1. **Unit & API Testing** ([`internal/web/server_test.go`](file:///Users/julienbreux/Projects/julienbreux/zigbridge/internal/web/server_test.go)):
   - Verify `TestWebStaticAssets` passes and embedded files are loaded cleanly.
   - Verify all API endpoints (`/api/status`, `/api/devices`, `/api/bindings`, `/api/ai/recommendations`) remain unaffected.
   - Verify WebSocket live stream (`/api/events`) continues broadcasting events that the new frontend consumes.
2. **Browser & UI Verification**:
   - Verify initial page load defaults to the **Devices** tab without showing the Overview tab.
   - Verify toggling **Advanced Mode** shows/hides technical elements instantly and persists after page refresh.
   - Verify **Coordinator** and **MQTT** status pills update correctly from `/api/status` and WebSocket updates.
   - Verify **Activity** tab displays friendly translated event cards and updates in real-time.

---

## Boundaries

- **Always**:
  - Keep 100% offline capability (no external CDN links for fonts, icons, or JS libraries).
  - Preserve all existing REST and WebSocket API schemas.
  - Store and organize all specification documents in the `specs/` directory.
  - Test static binary build and race detector with `make test`.
- **Ask First**:
  - Changing any backend Go API route or data structure.
- **Never**:
  - Break existing direct binding or permit-join workflows.
  - Remove technical data from Advanced Mode.

---

## Success Criteria

1. On initial visit, the interface opens directly to the **Devices** tab.
2. In Simple Mode, the header displays only the simplified Coordinator status, MQTT status, Permit Join button, and the Advanced Mode toggle.
3. The Overview page is removed from tabs and integrated into an expandable Diagnostics Drawer accessible in Advanced Mode.
4. "Live Stream" is renamed to "Activity", with human-friendly descriptions for common Zigbee events (state changes, device joins, permit-join countdowns).
5. Toggling Advanced Mode displays full technical details (IEEE, NWK, endpoints, clusters, LQI, raw logs) and persists across page reloads via `localStorage`.
6. `make test`, `make lint`, and `make build` pass with 0 errors and 0 warnings.

---

## User Approvals (Confirmed)

- [x] 1. One-click toggle in header to activate Advanced Mode.
- [x] 2. Coordinator & Network Architecture Overview moved to expandable Diagnostics Drawer.
- [x] 3. Plain-language event translations for the Activity tab with raw logs available in Advanced Mode.
- [x] 4. All specifications stored in `specs/` directory.
