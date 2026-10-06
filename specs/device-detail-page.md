# Spec: Device Detail Page with Hardware About & Interactive Exposes Control

## 1. Objective

Provide a dedicated, full-featured **Device Detail Page** within the embedded Zigbridge dashboard ([`internal/web/static/`](file:///Users/julienbreux/Projects/julienbreux/zigbridge/internal/web/static/)). This view enables users to inspect comprehensive hardware & network metadata ("About Section") and interact with physical/virtual device capabilities ("Exposes & Controls Section") matching the Zigbee2MQTT device control design (as seen in the reference screenshot).

### User Stories
- **As a smart home user**, when I click on a device in the Devices list, I want to navigate to a dedicated detail page with a clean back button (`← Back to Devices`) so I can focus on that specific device.
- **As a user**, I want to view an **About Section** displaying hardware information (friendly name, model, vendor, IEEE/NWK addresses, power source, battery level, signal quality LQI, endpoints, and input/output clusters).
- **As a user**, I want an **Exposes & Controls Section** that dynamically renders appropriate interactive UI controls based on the device's capabilities:
  - **Binary / Switch**: Dual-labeled toggle switch (e.g., `OFF` [toggle] `ON`, `UNLOCK` [toggle] `LOCK`).
  - **Numeric Slider + Input**: Range slider with min/max bounds alongside a numeric input box and unit suffix (e.g., Countdown `0`–`43200 s`).
  - **Enum / Mode Selector**: Segmented chip/button group where the active state is visually highlighted (e.g., Power outage memory `[on]` `[off]` `[restore]`, Indicator mode `[off]` `[off/on]` `[on/off]` `[on]`).
  - **Numeric Sensor / Telemetry Readout**: Prominent bold value with measurement unit (e.g., `0 W`, `0 A`, `238.3 V`, `0 kWh`, `131 lqi`, `21.5 °C`).
  - **Action Trigger**: Trigger button (e.g., `[identify]`).
- **As a user**, I want state changes and telemetry updates arriving via WebSocket to immediately update the controls and sensor readouts on this page in real time without refreshing.

---

## 2. Tech Stack

- **Backend**: Go 1.22+ (`net/http`, `embed.FS`, `sync.RWMutex`, `gorilla/websocket`).
- **Frontend**: Vanilla ES6+ JavaScript, Semantic HTML5, Vanilla CSS3 with CSS custom properties.
- **Packaging**: Embedded static assets via Go `embed.FS` in standalone static binary (`CGO_ENABLED=0`).
- **Dependencies**: Zero external CSS/JS CDNs or remote web fonts (100% offline-first).

---

## 3. Commands

```bash
# Build standalone binary with embedded web dashboard
make build

# Run full test suite with race detector
make test

# Run code linter and verify zero issues
make lint

# Run modernize check
make modernize

# Run local server with test configuration
./bin/zigbridge -config config.yaml
```

---

## 4. Project Structure

```
specs/
└── device-detail-page.md         # This specification document

internal/
├── controller/
│   ├── controller.go             # SetDeviceState(ctx, ieee, state) orchestration
│   └── device.go                 # Device struct and registry lookups
├── fixture/
│   └── definition.go             # ExposeDef, DeviceDefinition, and cluster definitions
└── web/
    ├── server.go                 # REST endpoints: GET /api/devices/{ieee}, POST /api/devices/{ieee}/set
    ├── server_test.go            # HTTP unit and integration tests for device endpoints
    └── static/
        ├── index.html            # Device detail page container (#view-device-detail)
        ├── style.css             # Layout styles for About cards, expose rows, toggles, sliders, chips
        └── app.js                # Hash routing (#/devices/{ieee}), exposes dynamic rendering, live WebSocket updates
```

---

## 5. Architectural & UX Specification

### 5.1 Routing & Navigation
- **Navigation Trigger**: Clicking any device row, avatar, or friendly name in the main Devices table navigates to `#/devices/{ieee}`.
- **Hash-based Client Routing**:
  - `#/devices` (or default): Shows the primary devices table and navigation tabs.
  - `#/devices/{ieee}`: Automatically hides the table view and reveals the `#view-device-detail` view.
  - Supports browser forward/back buttons (`window.addEventListener('hashchange', ...)`).
- **Header Breadcrumb**: A prominent top bar provides `← Back to Devices`, the device friendly name, model badge, and online/offline status.

### 5.2 Section 1: About Device (Hardware & Mesh Identity)
Rendered as a responsive card grid at the top of the detail page:
1. **Identity & Identification**:
   - Friendly Name (with inline rename modal/input)
   - Model identifier & Manufacturer / Vendor
   - Device description (from fixture catalog)
2. **Network & Addressing**:
   - IEEE 64-bit address (`0x...`) with copy-to-clipboard button
   - NWK 16-bit address (hex format: `0x1A2B`)
   - Endpoints list (e.g. `[1, 2]`)
   - Controllable Input Clusters (Server) & Sensor Output Clusters (Client) with friendly cluster names
3. **Power & Health**:
   - Power source (`Mains-powered` or `Battery`)
   - Battery percentage with color-coded badge (`🔋 95%`) and voltage if reported
   - Signal Quality (LQI 0–255 with graphical bar, qualitative rating `Excellent`/`Good`/`Fair`/`Poor`, and estimated dBm)
   - Availability status (`Online` / `Offline`) and Last Seen timestamp

### 5.3 Section 2: Exposes & Device Controls
Directly replicating the layout and visual hierarchy shown in the reference screenshot:
- **Card Container**: A clean list of capability rows separated by subtle dividers (`var(--border-subtle)`).
- **Row Anatomy**:
  - **Icon (Left)**: Visual category glyph (Star for State, Clock for Timers, Lightning for Power/Voltage/Current/Energy, Lock for Child Lock, Hand for Identify, Signal Bars for Link Quality, Thermometer for Climate, etc.).
  - **Info (Center)**:
    - Title: Human-readable name (e.g., `State`, `Countdown`, `Power outage memory`, `Voltage`).
    - Subtitle / Description: Descriptive text (e.g., `On/off state of the switch`, `Toggle the device after a set duration`, `Measured electrical potential value`).
    - Property Identifier: Subtle hover tooltip or tag showing raw key (e.g., `power_outage_memory`, `child_lock`).
  - **Interactive Element (Right)**:
    - **Type `binary`**:
      - Dual-state toggle switch with left and right state labels (e.g., `OFF` [switch] `ON`, `UNLOCK` [switch] `LOCK`).
      - Clicking immediately updates local state and issues `POST /api/devices/{ieee}/set` with `{"<property>": "<value>"}`.
    - **Type `numeric` (Controllable)**:
      - Slider range input bounded by `min` and `max` values with numeric labels underneath.
      - Numeric text input on the right with formatted unit suffix (e.g. `[ 0 ] s`).
      - Value changes sync bidirectionally between slider and text input, dispatched on change/debounce.
    - **Type `numeric` (Read-Only Sensor)**:
      - Large, bold telemetry readout with unit (e.g., **0** W, **0** A, **238.3** V, **0** kWh, **131** lqi).
    - **Type `enum`**:
      - Segmented button / chip selector.
      - Unselected options have clean outline borders; active option has solid accent background (`var(--accent-blue)`).
      - Clicking an unselected option dispatches `POST /api/devices/{ieee}/set` with the selected string value.
    - **Type `action` / Command**:
      - Styled action button (e.g. `[identify]`).
      - Clicking executes `POST /api/devices/{ieee}/action` with `{"action": "identify"}`.

### 5.4 Backend REST API Endpoints
1. `GET /api/devices/{ieee}`:
   - Returns device registry details combined with its matched fixture definition (exposes list, vendor, description, endpoints).
   - Response status: `200 OK` or `404 Not Found`.
2. `POST /api/devices/{ieee}/set`:
   - Payload: JSON object containing properties to set, e.g.:
     ```json
     { "state": "ON" }
     ```
     or
     ```json
     { "power_outage_memory": "restore" }
     ```
   - Controller updates device state, translates command to ZCL / adapter dispatch, updates store, and broadcasts WebSocket update.
   - Response status: `200 OK` with updated state or `400 Bad Request` / `404 Not Found`.
3. `POST /api/devices/{ieee}/action`:
   - Payload: JSON object specifying action name, e.g.: `{"action": "identify"}`.
   - Executes command simulation or ZCL cluster command.

---

## 6. Code Style & Example Implementation

### JavaScript Expose Dynamic Renderer Pattern
```javascript
function renderExposeRow(dev, expose) {
  const prop = expose.property;
  const currentValue = (dev.state && dev.state[prop] !== undefined) ? dev.state[prop] : null;
  const icon = getExposeIcon(expose.property, expose.type);

  let controlHtml = '';

  switch (expose.type) {
    case 'binary': {
      const offVal = (expose.values && expose.values[0]) || 'OFF';
      const onVal = (expose.values && expose.values[1]) || 'ON';
      const isChecked = String(currentValue).toUpperCase() === String(onVal).toUpperCase();
      controlHtml = `
        <div class="expose-toggle-group">
          <span class="toggle-label ${!isChecked ? 'active' : ''}">${escapeHtml(offVal)}</span>
          <label class="switch">
            <input type="checkbox" ${isChecked ? 'checked' : ''} onchange="setDeviceProperty('${escapeHtml(dev.ieee)}', '${escapeHtml(prop)}', this.checked ? '${escapeHtml(onVal)}' : '${escapeHtml(offVal)}')">
            <span class="slider round"></span>
          </label>
          <span class="toggle-label ${isChecked ? 'active' : ''}">${escapeHtml(onVal)}</span>
        </div>`;
      break;
    }
    case 'enum': {
      const values = expose.values || [];
      const chips = values.map(val => {
        const isSelected = String(currentValue).toLowerCase() === String(val).toLowerCase();
        return `<button class="chip-btn ${isSelected ? 'active' : ''}" onclick="setDeviceProperty('${escapeHtml(dev.ieee)}', '${escapeHtml(prop)}', '${escapeHtml(val)}')">${escapeHtml(val)}</button>`;
      }).join('');
      controlHtml = `<div class="chip-group">${chips}</div>`;
      break;
    }
    case 'numeric': {
      const isSettable = (expose.access & 2) !== 0;
      if (isSettable && expose.min !== undefined && expose.max !== undefined) {
        controlHtml = `
          <div class="expose-slider-group">
            <div class="slider-wrapper">
              <input type="range" min="${expose.min}" max="${expose.max}" value="${currentValue ?? expose.min}" onchange="setDeviceProperty('${escapeHtml(dev.ieee)}', '${escapeHtml(prop)}', Number(this.value))">
              <div class="slider-bounds"><span>${expose.min}</span><span>${expose.max}</span></div>
            </div>
            <div class="slider-val-box"><span>${currentValue ?? 0}</span> <span class="unit">${escapeHtml(expose.unit || '')}</span></div>
          </div>`;
      } else {
        controlHtml = `<div class="expose-metric-readout"><strong>${currentValue !== null ? currentValue : '--'}</strong> <span class="unit">${escapeHtml(expose.unit || '')}</span></div>`;
      }
      break;
    }
    case 'action': {
      controlHtml = `<button class="btn btn-primary btn-sm" onclick="triggerDeviceAction('${escapeHtml(dev.ieee)}', '${escapeHtml(prop)}')">${escapeHtml(expose.name || prop)}</button>`;
      break;
    }
  }

  return `
    <div class="expose-row" data-property="${escapeHtml(prop)}">
      <div class="expose-meta">
        <div class="expose-icon">${icon}</div>
        <div class="expose-info">
          <div class="expose-title" title="${escapeHtml(prop)}">${escapeHtml(expose.name || prop)}</div>
          <div class="expose-desc">${escapeHtml(expose.description || '')}</div>
        </div>
      </div>
      <div class="expose-control">${controlHtml}</div>
    </div>`;
}
```

---

## 7. Testing Strategy

1. **Go Unit & Integration Tests (`internal/web/server_test.go`)**:
   - `TestGetDeviceDetail`: Verifies `GET /api/devices/{ieee}` returns HTTP 200 with device data and associated fixture exposes; returns 404 for unknown devices.
   - `TestSetDeviceState`: Verifies `POST /api/devices/{ieee}/set` accepts state payload, updates in-memory registry, publishes WebSocket event, and returns HTTP 200.
   - `TestDeviceAction`: Verifies `POST /api/devices/{ieee}/action` invokes simulated or ZCL command and returns execution result.
2. **Controller Unit Tests (`internal/controller/controller_test.go`)**:
   - `TestController_SetDeviceState`: Verifies state mutation, mutex protection, and event emission.
3. **Frontend DOM Structure & Offline Verification**:
   - Inspect static assets (`index.html`, `app.js`, `style.css`) to ensure no external URLs (`http://`, `https://`, `cdn.`) are present.
   - Validate CSS responsive layout on desktop and mobile viewports.

---

## 8. Boundaries

- **Always**:
  - Maintain 100% offline compatibility with zero remote CDN scripts or styles.
  - Sanitize all device names, properties, and values before rendering to prevent XSS (`escapeHtml`).
  - Run `make test` and `make lint` before any Git commit.
  - Keep resident memory consumption below `< 25 MB`.
- **Ask First**:
  - Modifying existing public REST endpoints (`GET /api/devices`).
  - Adding new third-party Go dependencies.
- **Never**:
  - Block radio frame ingestion or WebSocket streams with slow synchronous HTTP calls.
  - Commit credentials, secrets, or test tokens.
  - Introduce external web font or script dependencies.

---

## 9. Success Criteria (Reframed Requirements)

- [ ] **SC-1 (Navigation & Routing)**: Clicking any device in the Devices table navigates to `#/devices/{ieee}` and displays the Device Detail Page without full page reload; clicking `← Back to Devices` returns to the devices list.
- [ ] **SC-2 (About Section)**: The detail page displays device Friendly Name, Model, Vendor, IEEE, NWK, Endpoints, Input/Output Clusters, Power source, Battery %, LQI with graphical bar, Availability, and Last Seen timestamp.
- [ ] **SC-3 (Binary Controls)**: Binary exposes render as toggle switches with dual labels (e.g. `OFF` / `ON`) matching the reference screenshot; toggling sends `POST /api/devices/{ieee}/set` and reflects immediately.
- [ ] **SC-4 (Numeric Sliders & Inputs)**: Controllable numeric exposes render a range slider with min/max labels and a numeric input box with unit; read-only numeric sensors display bold values with units.
- [ ] **SC-5 (Enum Mode Selectors)**: Enum exposes render segmented chip buttons with the active option highlighted in accent blue/purple; clicking an option dispatches the state change.
- [ ] **SC-6 (Action Triggers)**: Action exposes (e.g., `identify`) render as dedicated action buttons that execute `POST /api/devices/{ieee}/action`.
- [ ] **SC-7 (Live Real-Time Reactivity)**: WebSocket `device_state` events dynamically update the corresponding expose controls, sensor readouts, and About metrics without requiring a page refresh.
- [ ] **SC-8 (Zero CDN & Quality Gate)**: 100% offline operation, zero external CDN requests, clean `make test`, and zero `make lint` warnings.

---

## 10. Open Questions & Assumptions

### Assumptions Made
1. **Hash Routing**: Using browser URL hash (`#/devices/{ieee}`) for client-side routing within the single-page application so that deep links, bookmarking, and the browser's native Back button work naturally.
2. **Fixture Exposes Association**: If a device matches a definition in the fixture catalog (by `model` or `zigbee_models`), its `exposes` schema is used to dynamically construct the controls. If a device has no catalog definition, generic controls are generated based on its input/output cluster list (e.g., Cluster 6 = On/Off switch, Cluster 8 = Level slider, Cluster 1026 = Temperature readout).
3. **Mock & Real Device Uniformity**: In Mock coordinator mode, `POST /api/devices/{ieee}/set` and `/action` seamlessly interact with `VirtualDevice` and update state. In physical coordinator mode, it dispatches corresponding ZCL frames to the adapter.

### Open Questions for Human Review
1. **Control Placement**: Should the About Section be placed side-by-side with the Controls (two columns on wide screens) or stacked vertically (About cards on top, Exposes list underneath as shown in the spec)?
2. **Direct State Write Endpoint**: Do you approve adding `POST /api/devices/{ieee}/set` (accepting `{"property": value}`) and `GET /api/devices/{ieee}` (returning the combined device + fixture definition) to [`internal/web/server.go`](file:///Users/julienbreux/Projects/julienbreux/zigbridge/internal/web/server.go)?
