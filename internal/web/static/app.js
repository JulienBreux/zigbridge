// ==============================================================================
// Zigbridge Web Management Dashboard JavaScript
// ==============================================================================

let currentDevices = [];
let currentBindings = [];
let currentMode = localStorage.getItem('zigbridge_mode') || 'simple';
let permitJoinCountdownInterval = null;
let permitJoinRemainingSeconds = 0;
let ws = null;

// Cluster names mapping for human readability
const CLUSTER_NAMES = {
  0: 'Basic',
  1: 'Power Config',
  3: 'Identify',
  4: 'Groups',
  5: 'Scenes',
  6: 'On/Off',
  8: 'Level Control',
  25: 'OTA Upgrade',
  768: 'Color Control',
  1024: 'Illuminance',
  1026: 'Temperature',
  1029: 'Humidity',
  1030: 'Occupancy',
  2820: 'Electrical Measurement'
};

function clusterName(id) {
  return CLUSTER_NAMES[id] || `0x${Number(id).toString(16).padStart(4, '0')}`;
}

// ==============================================================================
// Mode Management (Simple Non-Tech vs. Advanced Tech)
// ==============================================================================

function initMode() {
  const body = document.body;
  const statusLabel = document.getElementById('mode-toggle-status');

  if (currentMode === 'advanced') {
    body.classList.remove('mode-simple');
    body.classList.add('mode-advanced');
    if (statusLabel) {
      statusLabel.textContent = 'ON';
    }
  } else {
    body.classList.remove('mode-advanced');
    body.classList.add('mode-simple');
    if (statusLabel) {
      statusLabel.textContent = 'OFF';
    }
    // Close diagnostics drawer when reverting to simple mode
    const drawer = document.getElementById('drawer-diagnostics');
    if (drawer) drawer.style.display = 'none';
  }
}

function toggleAdvancedMode() {
  currentMode = (currentMode === 'simple') ? 'advanced' : 'simple';
  localStorage.setItem('zigbridge_mode', currentMode);
  initMode();
}

// Diagnostics Drawer Toggle
function toggleDiagnosticsDrawer() {
  const drawer = document.getElementById('drawer-diagnostics');
  if (!drawer) return;
  const isHidden = (drawer.style.display === 'none' || drawer.style.display === '');
  drawer.style.display = isHidden ? 'block' : 'none';
}

// ==============================================================================
// Tab Navigation
// ==============================================================================

function switchTab(tabId) {
  document.querySelectorAll('.tab-btn').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.tab === tabId);
  });
  document.querySelectorAll('.tab-pane').forEach(pane => {
    pane.classList.toggle('active', pane.id === `tab-${tabId}`);
  });

  if (tabId === 'devices') loadDevices();
  if (tabId === 'bindings') loadBindings();
}

// Modal handling
function openModal(id) {
  const m = document.getElementById(id);
  if (m) m.classList.add('open');
}

function closeModal(id) {
  const m = document.getElementById(id);
  if (m) m.classList.remove('open');
}

// ==============================================================================
// Coordinator & Bridge Status
// ==============================================================================

async function loadStatus() {
  try {
    const res = await fetch('/api/status');
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = await res.json();

    // 1. Coordinator Status Pill
    const coordPill = document.getElementById('coordinator-status-pill');
    const coordText = document.getElementById('coordinator-status-text');
    if (data.connected && data.coordinator?.status === 'running') {
      coordPill.className = 'badge badge-connected';
      coordText.textContent = 'Coordinator: Online';
    } else if (data.transport_status === 'reconnecting') {
      coordPill.className = 'badge badge-disconnected';
      coordText.textContent = 'Coordinator: Connecting...';
    } else {
      coordPill.className = 'badge badge-disconnected';
      coordText.textContent = 'Coordinator: Offline';
    }

    // 2. MQTT Status Pill
    const mqttPill = document.getElementById('mqtt-status-pill');
    const mqttText = document.getElementById('mqtt-status-text');
    if (data.mqtt_connected) {
      mqttPill.className = 'badge badge-connected';
      mqttText.textContent = 'MQTT: Connected';
    } else {
      mqttPill.className = 'badge badge-disconnected';
      mqttText.textContent = 'MQTT: Offline / Disabled';
    }

    // 3. Diagnostics Drawer Telemetry
    if (document.getElementById('diag-coord-type')) {
      document.getElementById('diag-coord-type').textContent = data.coordinator?.type || 'Coordinator';
      document.getElementById('diag-coord-model').textContent = data.coordinator?.version || 'Zigbee 3.0';
      document.getElementById('diag-coord-ieee').textContent = data.coordinator?.ieee || '--:--:--:--:--:--:--:--';
      document.getElementById('diag-channel').textContent = data.coordinator?.channel || '--';
      document.getElementById('diag-panid').textContent = data.coordinator?.pan_id ? `0x${data.coordinator.pan_id.toString(16).toUpperCase()}` : '--';
      document.getElementById('diag-ext-panid').textContent = `Ext PAN: ${data.coordinator?.ext_pan_id || '--'}`;

      const transBadge = document.getElementById('diag-transport-badge');
      const transStatus = document.getElementById('diag-transport-status');
      if (data.connected) {
        transBadge.className = 'badge badge-connected';
        transStatus.textContent = 'Active Link';
      } else {
        transBadge.className = 'badge badge-disconnected';
        transStatus.textContent = data.transport_status || 'Disconnected';
      }

      // Formatting Uptime
      const sec = data.uptime_seconds || 0;
      const h = Math.floor(sec / 3600);
      const m = Math.floor((sec % 3600) / 60);
      const s = sec % 60;
      document.getElementById('diag-uptime').textContent = h > 0 ? `${h}h ${m}m ${s}s` : (m > 0 ? `${m}m ${s}s` : `${s}s`);
      document.getElementById('diag-mesh-counts').textContent = `${data.device_count || 0} Devices · ${data.binding_count || 0} Bindings`;
    }

    // Update Counts in Nav Tabs
    document.getElementById('nav-device-count').textContent = data.device_count || 0;
    document.getElementById('nav-binding-count').textContent = data.binding_count || 0;

    // Permit join countdown
    updatePermitJoinDisplay(data.permit_join_remaining || 0);

  } catch (err) {
    console.error('Failed to load status:', err);
  }
}

// ==============================================================================
// Permit Join (Network Pairing)
// ==============================================================================

function updatePermitJoinDisplay(seconds) {
  permitJoinRemainingSeconds = seconds;
  const btn = document.getElementById('permit-join-btn');
  const timer = document.getElementById('permit-join-timer');

  if (seconds > 0) {
    btn.classList.add('active');
    timer.style.display = 'inline';
    timer.textContent = `(${seconds}s)`;
  } else {
    btn.classList.remove('active');
    timer.style.display = 'none';
  }
}

function openPermitJoinModal() {
  openModal('modal-permit-join');
}

async function submitPermitJoin() {
  const durationInput = document.getElementById('permit-join-duration');
  const duration = parseInt(durationInput.value, 10) || 60;

  try {
    const res = await fetch('/api/network/permit-join', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ time: duration })
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    closeModal('modal-permit-join');
    loadStatus();
  } catch (err) {
    alert(`Failed to open network: ${err.message}`);
  }
}

async function stopPermitJoin() {
  try {
    await fetch('/api/network/permit-join', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ time: 0 })
    });
    closeModal('modal-permit-join');
    loadStatus();
  } catch (err) {
    alert(`Failed to close network: ${err.message}`);
  }
}

// ==============================================================================
// Devices Management
// ==============================================================================

async function loadDevices() {
  try {
    const res = await fetch('/api/devices');
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    currentDevices = await res.json() || [];
    renderDevicesTable(currentDevices);
    document.getElementById('nav-device-count').textContent = currentDevices.length;
  } catch (err) {
    console.error('Failed to load devices:', err);
  }
}

// Helper to deduce friendly device type and icon
function inferDeviceType(dev) {
  const inClusters = dev.input_clusters || [];
  const outClusters = dev.output_clusters || [];
  const model = (dev.model || '').toLowerCase();

  if (inClusters.includes(1030) || model.includes('motion') || model.includes('occupancy')) {
    return { name: 'Motion Sensor', icon: '🚶' };
  }
  if (inClusters.includes(1026) || inClusters.includes(1029) || model.includes('temp') || model.includes('weather')) {
    return { name: 'Climate Sensor', icon: '🌡️' };
  }
  if (outClusters.includes(6) || model.includes('switch') || model.includes('remote') || model.includes('button')) {
    return { name: 'Remote / Switch', icon: '🖲️' };
  }
  if (inClusters.includes(6) && inClusters.includes(8)) {
    return { name: 'Dimmable Light', icon: '💡' };
  }
  if (inClusters.includes(6) && inClusters.includes(768)) {
    return { name: 'Color Light', icon: '🎨' };
  }
  if (inClusters.includes(6)) {
    return { name: 'Smart Light / Switch', icon: '💡' };
  }
  if (inClusters.includes(2820) || model.includes('plug') || model.includes('outlet')) {
    return { name: 'Smart Plug', icon: '🔌' };
  }
  return { name: 'Zigbee Device', icon: '📦' };
}

function renderDevicesTable(devices) {
  const tbody = document.getElementById('devices-table-body');
  if (!tbody) return;

  if (devices.length === 0) {
    tbody.innerHTML = `
      <tr>
        <td colspan="11" style="text-align: center; color: var(--text-muted); padding: 32px;">
          No devices paired yet. Click <strong>+ Add Device (Pair)</strong> above to connect smart switches, lights, or sensors.
        </td>
      </tr>`;
    return;
  }

  tbody.innerHTML = devices.map(dev => {
    const typeInfo = inferDeviceType(dev);

    // Qualitative signal strength
    let signalBadge = '<span class="badge badge-signal-poor">● Poor</span>';
    if (dev.lqi >= 180) {
      signalBadge = '<span class="badge badge-signal-excellent">● Excellent</span>';
    } else if (dev.lqi >= 110) {
      signalBadge = '<span class="badge badge-signal-good">● Good</span>';
    } else if (dev.lqi >= 50) {
      signalBadge = '<span class="badge badge-signal-fair">● Fair</span>';
    }

    // Battery / Power representation
    const batteryDisplay = dev.battery ? 
      `<span class="badge badge-info" style="font-size: 11px;">🔋 ${dev.battery}%</span>` : 
      `<span style="color:var(--text-secondary); font-size:12px;">⚡ Mains</span>`;

    // Power / State representation
    let stateHtml = '<span style="color:var(--text-muted); font-size:12px;">Idle</span>';
    if (dev.state) {
      if (typeof dev.state.state === 'string') {
        const isON = dev.state.state.toUpperCase() === 'ON';
        stateHtml = isON ? 
          `<span class="badge badge-connected" style="font-size:11px;">ON</span>` : 
          `<span class="badge badge-disconnected" style="font-size:11px;">OFF</span>`;
      } else if (dev.state.occupancy !== undefined) {
        stateHtml = dev.state.occupancy ? 
          `<span class="badge badge-purple" style="font-size:11px;">Motion</span>` : 
          `<span style="color:var(--text-muted); font-size:12px;">Clear</span>`;
      } else if (dev.state.temperature !== undefined) {
        stateHtml = `<span class="badge badge-info" style="font-size:11px;">${dev.state.temperature}°C</span>`;
      }
    }

    // Advanced info
    const lastSeen = dev.last_seen ? new Date(dev.last_seen).toLocaleTimeString() : 'Never';
    const inClusters = (dev.input_clusters || []).map(c => `<span class="tag-cluster" title="Server">${clusterName(c)}</span>`).join('');
    const outClusters = (dev.output_clusters || []).map(c => `<span class="tag-cluster" style="color:var(--accent-blue);" title="Client">${clusterName(c)}</span>`).join('');
    const lqiPercent = Math.min(100, Math.round((dev.lqi / 255) * 100));

    return `
      <tr>
        <!-- Device Friendly Name & Icon -->
        <td>
          <div class="device-row-main">
            <div class="device-avatar">${typeInfo.icon}</div>
            <div>
              <div class="device-name-title">${escapeHtml(dev.friendly_name || dev.ieee)}</div>
              <div class="device-name-model">${escapeHtml(dev.model || 'Generic Device')}</div>
            </div>
          </div>
        </td>

        <!-- Device Inferred Type -->
        <td>
          <span style="font-size: 13px; color: var(--text-secondary);">${typeInfo.name}</span>
        </td>

        <!-- Status / Power -->
        <td>${stateHtml}</td>

        <!-- Battery -->
        <td>${batteryDisplay}</td>

        <!-- Signal Strength -->
        <td>${signalBadge}</td>

        <!-- Advanced: IEEE Address -->
        <td class="col-advanced mono" style="font-size: 12px;">${escapeHtml(dev.ieee)}</td>

        <!-- Advanced: NWK Address -->
        <td class="col-advanced mono" style="font-size: 12px; color: var(--text-secondary);">
          0x${(dev.nwk || 0).toString(16).toUpperCase()}
        </td>

        <!-- Advanced: Endpoints & Clusters -->
        <td class="col-advanced">
          <div style="font-size:11px; margin-bottom:2px;">EP: [${(dev.endpoints || []).join(', ')}]</div>
          <div>${inClusters} ${outClusters}</div>
        </td>

        <!-- Advanced: Numerical LQI -->
        <td class="col-advanced">
          <div class="mono" style="font-size: 11px;">${dev.lqi} / 255</div>
          <div class="lqi-bar-container" style="height: 4px; margin-top: 3px;">
            <div class="lqi-bar" style="width: ${lqiPercent}%;"></div>
          </div>
        </td>

        <!-- Advanced: Last Seen -->
        <td class="col-advanced" style="font-size: 11px; color: var(--text-muted);">${lastSeen}</td>

        <!-- Actions -->
        <td>
          <button class="btn btn-sm" onclick="openRenameModal('${escapeHtml(dev.ieee)}', '${escapeHtml(dev.friendly_name || '')}')">
            Rename
          </button>
        </td>
      </tr>
    `;
  }).join('');
}

function filterDevices() {
  const query = (document.getElementById('device-search').value || '').toLowerCase();
  const filtered = currentDevices.filter(d => 
    (d.friendly_name || '').toLowerCase().includes(query) ||
    (d.ieee || '').toLowerCase().includes(query) ||
    (d.model || '').toLowerCase().includes(query) ||
    (d.manufacturer || '').toLowerCase().includes(query)
  );
  renderDevicesTable(filtered);
}

// Rename device
function openRenameModal(ieee, currentName) {
  document.getElementById('rename-device-ieee').value = ieee;
  document.getElementById('rename-device-name').value = currentName;
  openModal('modal-rename-device');
}

async function submitDeviceRename() {
  const ieee = document.getElementById('rename-device-ieee').value;
  const name = document.getElementById('rename-device-name').value.trim();

  try {
    const res = await fetch(`/api/devices/${encodeURIComponent(ieee)}/rename`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ friendly_name: name })
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    closeModal('modal-rename-device');
    loadDevices();
  } catch (err) {
    alert(`Failed to rename device: ${err.message}`);
  }
}

// ==============================================================================
// Direct Bindings
// ==============================================================================

async function loadBindings() {
  try {
    const res = await fetch('/api/bindings');
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    currentBindings = await res.json() || [];
    renderBindingsTable(currentBindings);
    document.getElementById('nav-binding-count').textContent = currentBindings.length;
  } catch (err) {
    console.error('Failed to load bindings:', err);
  }
}

function renderBindingsTable(bindings) {
  const tbody = document.getElementById('bindings-table-body');
  if (!tbody) return;

  if (bindings.length === 0) {
    tbody.innerHTML = `
      <tr>
        <td colspan="6" style="text-align: center; color: var(--text-muted); padding: 24px;">
          No direct bindings configured. Click <strong>+ Create Device Link</strong> to pair a switch directly to a bulb.
        </td>
      </tr>`;
    return;
  }

  tbody.innerHTML = bindings.map(b => {
    return `
      <tr>
        <td>
          <div style="font-weight: 600;">${escapeHtml(getDeviceName(b.src_ieee))}</div>
          <div class="mono" style="font-size:11px; color:var(--text-muted);">${escapeHtml(b.src_ieee)}</div>
        </td>
        <td class="col-advanced mono">${b.src_endpoint}</td>
        <td>
          <span class="badge badge-purple">${clusterName(b.cluster_id)}</span>
          <span class="col-advanced mono" style="font-size:11px; color:var(--text-muted); margin-left:4px;">
            0x${Number(b.cluster_id).toString(16).padStart(4, '0')}
          </span>
        </td>
        <td>
          <div style="font-weight: 600;">${escapeHtml(getDeviceName(b.dst_ieee))}</div>
          <div class="mono" style="font-size:11px; color:var(--text-muted);">${escapeHtml(b.dst_ieee)}</div>
        </td>
        <td class="col-advanced mono">${b.dst_endpoint}</td>
        <td>
          <button class="btn btn-danger btn-sm" onclick="removeBinding('${escapeHtml(b.src_ieee)}', ${b.src_endpoint}, '${escapeHtml(b.dst_ieee)}', ${b.dst_endpoint}, ${b.cluster_id})">
            Unlink
          </button>
        </td>
      </tr>
    `;
  }).join('');
}

function getDeviceName(ieee) {
  const dev = currentDevices.find(d => d.ieee === ieee);
  return (dev && dev.friendly_name) ? dev.friendly_name : (ieee || 'Unknown');
}

function openCreateBindingModal() {
  populateDeviceSelect('bind-src-device');
  populateDeviceSelect('bind-target-device');
  updateBindingEndpoints('src');
  updateBindingEndpoints('target');
  openModal('modal-create-binding');
}

function populateDeviceSelect(selectId) {
  const sel = document.getElementById(selectId);
  if (!sel) return;
  sel.innerHTML = currentDevices.map(d => {
    const label = d.friendly_name ? `${d.friendly_name} (${d.ieee})` : d.ieee;
    return `<option value="${escapeHtml(d.ieee)}">${escapeHtml(label)}</option>`;
  }).join('');
}

function updateBindingEndpoints(role) {
  const selectDevId = role === 'src' ? 'bind-src-device' : 'bind-target-device';
  const selectEpId = role === 'src' ? 'bind-src-ep' : 'bind-target-ep';
  const devIeee = document.getElementById(selectDevId)?.value;
  const epSelect = document.getElementById(selectEpId);
  if (!epSelect) return;

  const dev = currentDevices.find(d => d.ieee === devIeee);
  const eps = (dev && dev.endpoints && dev.endpoints.length > 0) ? dev.endpoints : [1];

  epSelect.innerHTML = eps.map(ep => `<option value="${ep}">${ep}</option>`).join('');

  if (role === 'target') {
    const note = document.getElementById('bind-optimistic-note');
    if (note) {
      note.style.display = (!dev || !dev.endpoints || dev.endpoints.length === 0) ? 'block' : 'none';
    }
  }
}

async function submitCreateBinding() {
  const srcIEEE = document.getElementById('bind-src-device').value;
  const srcEp = parseInt(document.getElementById('bind-src-ep').value, 10) || 1;
  const targetIEEE = document.getElementById('bind-target-device').value;
  const targetEp = parseInt(document.getElementById('bind-target-ep').value, 10) || 1;
  const clusterID = parseInt(document.getElementById('bind-cluster').value, 10);

  if (!srcIEEE || !targetIEEE) {
    alert('Please select both source and target devices.');
    return;
  }

  try {
    const res = await fetch('/api/bindings', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        source_ieee: srcIEEE,
        source_ep: srcEp,
        target_ieee: targetIEEE,
        target_ep: targetEp,
        cluster_id: clusterID
      })
    });

    const data = await res.json();
    if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);

    if (data.warning) {
      alert(`Link established with note:\n${data.warning}`);
    }

    closeModal('modal-create-binding');
    loadBindings();
  } catch (err) {
    alert(`Failed to create link: ${err.message}`);
  }
}

async function removeBinding(srcIEEE, srcEp, dstIEEE, dstEp, clusterID) {
  if (!confirm(`Are you sure you want to remove the direct link between ${getDeviceName(srcIEEE)} and ${getDeviceName(dstIEEE)}?`)) {
    return;
  }

  try {
    const res = await fetch('/api/bindings', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        source_ieee: srcIEEE,
        source_ep: srcEp,
        target_ieee: dstIEEE,
        target_ep: dstEp,
        cluster_id: clusterID
      })
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    loadBindings();
  } catch (err) {
    alert(`Failed to remove link: ${err.message}`);
  }
}

// ==============================================================================
// Smart Suggestions (AI Recommendations)
// ==============================================================================

async function runAIAnalysis() {
  const btn = document.getElementById('btn-trigger-ai');
  const container = document.getElementById('ai-recommendations-list');
  btn.disabled = true;
  btn.innerHTML = '<span>⚡ Finding Suggestions...</span>';

  try {
    const res = await fetch('/api/ai/recommendations');
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const recs = await res.json() || [];
    renderRecommendations(recs);
    document.getElementById('nav-rec-count').textContent = recs.length;
  } catch (err) {
    container.innerHTML = `<div class="card" style="color:var(--accent-red); padding:16px;">Failed to inspect suggestions: ${escapeHtml(err.message)}</div>`;
  } finally {
    btn.disabled = false;
    btn.innerHTML = '<span>⚡ Find Suggestions</span>';
  }
}

function renderRecommendations(recs) {
  const container = document.getElementById('ai-recommendations-list');
  if (!container) return;

  if (recs.length === 0) {
    container.innerHTML = `<div class="card" style="text-align: center; padding: 32px; color: var(--text-muted);">No automatic suggestions detected yet. Connect a switch and a bulb to see autonomous link proposals.</div>`;
    return;
  }

  container.innerHTML = recs.map(rec => {
    const confPercent = Math.round(rec.confidence * 100);
    const confClass = confPercent >= 80 ? 'confidence-high' : 'confidence-med';
    const isApplied = rec.status === 'applied';

    return `
      <div class="card rec-card">
        <div style="flex: 1; min-width: 300px;">
          <div style="display: flex; align-items: center; gap: 10px; margin-bottom: 6px;">
            <span class="badge badge-purple">${escapeHtml(rec.type)}</span>
            <span class="confidence-meter ${confClass}">Score: ${confPercent}%</span>
            <span class="badge badge-info">${clusterName(rec.cluster_id)}</span>
          </div>
          <div style="font-weight: 600; margin-bottom: 4px;">
            ${escapeHtml(getDeviceName(rec.source_ieee))} ➔ ${escapeHtml(getDeviceName(rec.target_ieee))}
          </div>
          <div style="font-size: 13px; color: var(--text-secondary);">
            ${escapeHtml(rec.reasoning)}
          </div>
        </div>
        <div>
          ${isApplied ? 
            `<span class="badge badge-connected">Applied</span>` : 
            `<button class="btn btn-success btn-sm" onclick="applyRecommendation('${escapeHtml(rec.id)}')">Enable Direct Link</button>`
          }
        </div>
      </div>
    `;
  }).join('');
}

async function applyRecommendation(recId) {
  try {
    const res = await fetch(`/api/ai/recommendations/${encodeURIComponent(recId)}/apply`, {
      method: 'POST'
    });
    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error || `HTTP ${res.status}`);
    }
    alert('Direct link enabled successfully!');
    runAIAnalysis();
    loadBindings();
  } catch (err) {
    alert(`Failed to apply suggestion: ${err.message}`);
  }
}

// ==============================================================================
// Activity Feed & WebSocket Live Stream
// ==============================================================================

function connectWebSocket() {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = `${protocol}//${window.location.host}/api/events`;
  const wsBadge = document.getElementById('ws-status-badge');

  try {
    ws = new WebSocket(wsUrl);

    ws.onopen = () => {
      if (wsBadge) {
        wsBadge.textContent = '● Live';
        wsBadge.style.color = 'var(--accent-green)';
      }
      appendActivityItem('system', 'System Online', 'Connected to Zigbridge live activity stream.');
    };

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        handleLiveEvent(msg);
      } catch (e) {
        console.error('Failed to parse WS message:', e);
      }
    };

    ws.onclose = () => {
      if (wsBadge) {
        wsBadge.textContent = '○ Reconnecting';
        wsBadge.style.color = 'var(--accent-orange)';
      }
      setTimeout(connectWebSocket, 3000);
    };

    ws.onerror = (err) => {
      console.warn('WS error:', err);
      ws.close();
    };
  } catch (err) {
    console.error('WebSocket connection error:', err);
  }
}

function handleLiveEvent(evt) {
  const eventType = evt.type || 'event';
  const payload = evt.data || evt.payload || evt;

  // 1. Convert into a friendly activity item
  dispatchFriendlyActivity(eventType, payload);

  // 2. Append to raw log console (for Advanced Mode)
  let typeClass = 'frame';
  if (eventType === 'device_join') typeClass = 'join';
  else if (eventType === 'binding_change') typeClass = 'binding';
  else if (eventType === 'system_warning') typeClass = 'warn';
  appendRawLog(eventType, JSON.stringify(payload), typeClass);

  // 3. Dynamic UI updates
  if (eventType === 'permit_join') {
    if (typeof payload.remaining === 'number') {
      updatePermitJoinDisplay(payload.remaining);
    }
  } else if (eventType === 'device_join' || eventType === 'device_state_change') {
    loadDevices();
    loadStatus();
  } else if (eventType === 'binding_change') {
    loadBindings();
    loadStatus();
  }
}

function dispatchFriendlyActivity(type, payload) {
  const now = new Date().toLocaleTimeString();

  if (type === 'device_join') {
    const devName = payload.friendly_name || payload.ieee || 'New Device';
    appendActivityItem('join', '✨ New Device Paired', `${devName} joined the Zigbee mesh network.`, 'join');
  } else if (type === 'device_state_change') {
    const devName = getDeviceName(payload.ieee);
    let stateDesc = 'Reported attribute update.';
    if (payload.state && typeof payload.state.state === 'string') {
      stateDesc = `Turned ${payload.state.state.toUpperCase()}.`;
    } else if (payload.state && payload.state.occupancy !== undefined) {
      stateDesc = payload.state.occupancy ? 'Detected motion.' : 'Motion clear.';
    } else if (payload.state && payload.state.temperature !== undefined) {
      stateDesc = `Temperature measured: ${payload.state.temperature}°C.`;
    }
    appendActivityItem('light', `💡 ${devName}`, stateDesc, 'light');
  } else if (type === 'permit_join') {
    if (payload.remaining > 0) {
      appendActivityItem('system', '🔓 Pairing Mode Active', `Network is open to pair new devices (${payload.remaining}s remaining).`, 'system');
    } else {
      appendActivityItem('system', '🔒 Pairing Window Closed', 'Network pairing window has closed.', 'system');
    }
  } else if (type === 'binding_change') {
    appendActivityItem('switch', '🔗 Direct Link Synchronized', `Direct binding updated for ${getDeviceName(payload.source_ieee)}.`, 'switch');
  } else if (type === 'system_warning') {
    appendActivityItem('system', '⚠️ Notice', String(payload), 'system');
  }
}

function appendActivityItem(iconType, title, meta, iconClass = 'system') {
  const timeline = document.getElementById('activity-timeline');
  if (!timeline) return;

  // Clear empty state placeholder
  const emptyPlaceholder = timeline.querySelector('.timeline-empty');
  if (emptyPlaceholder) emptyPlaceholder.remove();

  let iconEmoji = '📡';
  if (iconType === 'light') iconEmoji = '💡';
  else if (iconType === 'motion') iconEmoji = '🏃';
  else if (iconType === 'switch') iconEmoji = '🖲️';
  else if (iconType === 'join') iconEmoji = '✨';
  else if (iconType === 'system') iconEmoji = '⚙️';

  const now = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });

  const card = document.createElement('div');
  card.className = 'activity-card';
  card.innerHTML = `
    <div class="activity-icon ${iconClass}">${iconEmoji}</div>
    <div class="activity-body">
      <div class="activity-title">${escapeHtml(title)}</div>
      <div class="activity-meta">${escapeHtml(meta)}</div>
    </div>
    <div class="activity-time">${now}</div>
  `;

  timeline.insertBefore(card, timeline.firstChild);

  // Keep timeline bounded to 50 items
  while (timeline.children.length > 50) {
    timeline.removeChild(timeline.lastChild);
  }
}

function clearActivityFeed() {
  const timeline = document.getElementById('activity-timeline');
  if (timeline) {
    timeline.innerHTML = `<div class="timeline-empty" style="text-align: center; color: var(--text-muted); padding: 36px;">Activity feed cleared.</div>`;
  }
  const consoleEl = document.getElementById('event-log-console');
  if (consoleEl) consoleEl.innerHTML = '';
}

function appendRawLog(type, message, typeClass = 'frame') {
  const consoleEl = document.getElementById('event-log-console');
  if (!consoleEl) return;

  const now = new Date().toLocaleTimeString();
  const line = document.createElement('div');
  line.className = 'log-line';
  line.innerHTML = `<span class="log-time">[${now}]</span><span class="log-type ${typeClass}">[${escapeHtml(type.toUpperCase())}]</span><span>${escapeHtml(message)}</span>`;

  consoleEl.insertBefore(line, consoleEl.firstChild);

  while (consoleEl.children.length > 200) {
    consoleEl.removeChild(consoleEl.lastChild);
  }
}

function escapeHtml(str) {
  if (str === null || str === undefined) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

// ==============================================================================
// Bootstrap
// ==============================================================================

window.addEventListener('DOMContentLoaded', () => {
  // 1. Initialize user mode (Simple vs Advanced) from localStorage
  initMode();

  // 2. Default landing tab is Devices
  switchTab('devices');

  // 3. Load initial network state
  loadStatus();
  loadDevices();
  loadBindings();
  connectWebSocket();

  // 4. Polling timer every 3s
  setInterval(loadStatus, 3000);
});
