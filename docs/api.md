# REST & WebSocket API Reference

ZigBridge provides an embedded HTTP server exposing RESTful endpoints and a real-time WebSocket event stream for seamless integration with external scripts, home automation systems, and custom frontends.

---

## Base URL & General Conventions

- **Default Base URL**: `http://localhost:8080` (configured via `web.listen_addr`)
- **Content-Type**: `application/json`
- **CORS**: Supported by default (configurable via `web.enable_cors`)

---

## REST Endpoints

### 1. System Status (`GET /api/status`)
Returns bridge uptime, radio coordinator telemetry, network channel, device counts, and MQTT status.

#### Example Request
```bash
curl -s http://localhost:8080/api/status
```

#### Example Response (`200 OK`)
```json
{
  "connected": true,
  "transport_status": "connected",
  "coordinator": {
    "type": "TI Z-Stack 3.x",
    "version": "3.30.0 (TI MT)",
    "channel": 20,
    "pan_id": 6754,
    "ext_pan_id": "0xDDDDDDDDDDDDDDDD",
    "ieee": "0x00124B001F00ABCD",
    "status": "running"
  },
  "device_count": 5,
  "binding_count": 2,
  "permit_join_remaining": 0,
  "mqtt_connected": true,
  "uptime_seconds": 3600
}
```

---

### 2. Permit Join (`POST /api/network/permit-join`)
Opens or closes the Zigbee pairing window for new devices.

#### Request Payload
```json
{
  "duration_seconds": 120
}
```
*Pass `"duration_seconds": 0` to close the pairing window immediately.*

#### Example Request
```bash
curl -X POST http://localhost:8080/api/network/permit-join \
  -H "Content-Type: application/json" \
  -d '{"duration_seconds": 180}'
```

#### Example Response (`200 OK`)
```json
{
  "status": "ok",
  "permit_join_seconds": 180
}
```

---

### 3. List Devices (`GET /api/devices`)
Returns the list of all paired Zigbee devices, their discovered endpoints, clusters, friendly names, and signal metrics.

#### Example Request
```bash
curl -s http://localhost:8080/api/devices
```

#### Example Response (`200 OK`)
```json
[
  {
    "ieee_address": "0x00124B001CA20001",
    "network_address": 4660,
    "friendly_name": "Living Room Remote",
    "model": "IKEA TRADFRI remote",
    "manufacturer": "IKEA of Sweden",
    "power_source": "battery",
    "battery_percent": 90,
    "lqi": 195,
    "endpoints": [1],
    "in_clusters": [0, 1, 3],
    "out_clusters": [3, 4, 6, 8]
  },
  {
    "ieee_address": "0x00124B001CA20002",
    "network_address": 4661,
    "friendly_name": "Ceiling Light",
    "model": "Philips Hue White",
    "manufacturer": "Signify",
    "power_source": "mains",
    "battery_percent": 100,
    "lqi": 220,
    "endpoints": [1],
    "in_clusters": [0, 3, 4, 5, 6, 8],
    "out_clusters": []
  }
]
```

---

### 4. Rename Device (`POST /api/devices/:ieee/rename`)
Updates a device's human-friendly name.

#### Example Request
```bash
curl -X POST http://localhost:8080/api/devices/0x00124B001CA20001/rename \
  -H "Content-Type: application/json" \
  -d '{"friendly_name": "Kitchen Switch"}'
```

#### Example Response (`200 OK`)
```json
{
  "status": "ok",
  "ieee": "0x00124B001CA20001",
  "friendly_name": "Kitchen Switch"
}
```

---

### 5. Direct Bindings (`GET /api/bindings`, `POST /api/bindings`, `DELETE /api/bindings`)

#### List Bindings (`GET /api/bindings`)
```bash
curl -s http://localhost:8080/api/bindings
```

#### Create Binding (`POST /api/bindings`)
```bash
curl -X POST http://localhost:8080/api/bindings \
  -H "Content-Type: application/json" \
  -d '{
    "source_ieee": "0x00124B001CA20001",
    "source_endpoint": 1,
    "cluster_id": 6,
    "target_ieee": "0x00124B001CA20002",
    "target_endpoint": 1
  }'
```

#### Delete Binding (`DELETE /api/bindings`)
```bash
curl -X DELETE http://localhost:8080/api/bindings \
  -H "Content-Type: application/json" \
  -d '{
    "source_ieee": "0x00124B001CA20001",
    "source_endpoint": 1,
    "cluster_id": 6,
    "target_ieee": "0x00124B001CA20002",
    "target_endpoint": 1
  }'
```

---

### 6. AI & Smart Recommendations (`GET /api/ai/recommendations`, `POST /api/ai/recommendations/apply`)

#### Get Recommendations (`GET /api/ai/recommendations`)
```bash
curl -s http://localhost:8080/api/ai/recommendations
```

#### Apply a Recommended Binding (`POST /api/ai/recommendations/apply`)
```bash
curl -X POST http://localhost:8080/api/ai/recommendations/apply \
  -H "Content-Type: application/json" \
  -d '{"recommendation_id": "rec_00124b_001_to_002"}'
```

---

## WebSocket Event Stream (`GET /api/events`)

ZigBridge broadcasts real-time events over a single persistent WebSocket connection at `ws://localhost:8080/api/events`.

### Event Types & Payloads

#### `device_joined`
```json
{
  "type": "device_joined",
  "data": {
    "ieee": "0x00124B001CA20001",
    "nwk": 4660,
    "friendly_name": "New Device 4660"
  },
  "timestamp": "2026-10-06T12:00:00Z"
}
```

#### `permit_join_changed`
```json
{
  "type": "permit_join_changed",
  "data": {
    "remaining_seconds": 120
  },
  "timestamp": "2026-10-06T12:00:01Z"
}
```

#### `frame_received`
```json
{
  "type": "frame_received",
  "data": {
    "source_ieee": "0x00124B001CA20001",
    "cluster_id": 6,
    "command_id": 1,
    "payload": "AQ=="
  },
  "timestamp": "2026-10-06T12:00:05Z"
}
```

#### `binding_added` / `binding_removed`
```json
{
  "type": "binding_added",
  "data": {
    "source_ieee": "0x00124B001CA20001",
    "source_endpoint": 1,
    "cluster_id": 6,
    "target_ieee": "0x00124B001CA20002",
    "target_endpoint": 1
  },
  "timestamp": "2026-10-06T12:00:10Z"
}
```
