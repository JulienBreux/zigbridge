# Coordinator Setup Guide

Zigbridge supports both **network-attached coordinators** (operating over TCP/RFC2217) and **local USB serial dongles**.

---

## Supported Radio Adapters

| Adapter Type | Driver | Recommended Hardware |
|---|---|---|
| `zstack` | Texas Instruments Z-Stack 3.x (MT Protocol) | SMLIGHT SLZB-06, Sonoff ZBDongle-P, CC2652P, CC1352P |
| `ember` | Silicon Labs EmberZNet (EZSP v8+) | SMLIGHT SLZB-06M, Sonoff ZBDongle-E, Home Assistant SkyConnect, EFR32MG21 |
| `mock` | Virtual in-memory adapter | Software simulation, CI/CD testing, offline UI testing |

---

## 1. SMLIGHT SLZB-06 (Network / TCP Coordinator)

The **SMLIGHT SLZB-06** is an Ethernet/Wi-Fi connected Zigbee coordinator powered by a TI CC2652P chip. Because it communicates over the local network via TCP socket streaming, you can position the coordinator in the physical center of your home or near a PoE Ethernet switch without needing to place your server nearby.

### Why Zigbridge is Built for SLZB-06
Traditional Zigbee bridges often treat network coordinators as generic serial ports wrapped with `socat`. When a Wi-Fi drop, DHCP renewal, or switch reboot occurs, connection sockets can hang indefinitely.

Zigbridge provides **native TCP / RFC2217 transport handling**:
- **TCP Keepalive Probes**: Active TCP-level keepalive pulses (default `10s`) detect silent connection drops immediately.
- **Exponential Backoff Reconnect**: When a disconnect occurs, Zigbridge automatically retries with smooth exponential backoff without crashing the application.
- **RFC2217 Filter**: Strips out Telnet/RFC2217 baud rate negotiation sequences so they do not corrupt Zigbee frames.

### Configuration (`config.yaml`)

```yaml
transport:
  type: tcp
  url: "tcp://192.168.1.50:6638"    # Replace with your SLZB-06 IP and port (default 6638)
  reconnect_interval: 2s            # Initial reconnection delay
  max_reconnect_interval: 30s       # Max delay between reconnection attempts
  tcp_keepalive: 10s                # Active TCP keepalive interval
  rfc2217: true                     # Filter Telnet RFC2217 negotiation bytes

adapter:
  type: zstack                      # SLZB-06 uses TI Z-Stack
  pan_id: 0x1A62
  ext_pan_id: "0xDDDDDDDDDDDDDDDD"
  channel: 20
  network_key: "01030507090B0D0F00020406080A0C0D"
```

> [!TIP]
> Assign a static DHCP lease or fixed IP address to your SLZB-06 in your local router settings to prevent IP changes after a power cycle.

---

## 2. Local USB Serial Dongles

For coordinators plugged directly into a USB port on your server (e.g. Raspberry Pi, Mini PC):

### Linux Device Paths
Always prefer `/dev/serial/by-id/...` over `/dev/ttyUSB0` to avoid device path reordering when plugging in additional USB peripherals:

```yaml
transport:
  type: serial
  port: "/dev/serial/by-id/usb-Silicon_Labs_CP2102N_USB_to_UART_Bridge_Controller-if00-port0"
  baudrate: 115200                  # 115200 for TI CC2652, 115200 or 230400 for EZSP
  read_timeout: 10s
  write_timeout: 5s

adapter:
  type: zstack                      # or "ember" for Silicon Labs
  pan_id: 0x1A62
  channel: 20
```

### Platform Port Examples
- **Linux**: `/dev/ttyUSB0` or `/dev/serial/by-id/...`
- **macOS**: `/dev/cu.usbserial-0001` or `/dev/cu.SLAB_USBtoUART`
- **Windows**: `COM3`

---

## 3. Radio Channel & Wi-Fi Coexistence

Zigbee operates in the 2.4 GHz ISM band alongside Wi-Fi 802.11 b/g/n. Choosing the right channel prevents interference and packet loss.

| Zigbee Channel | Frequency | Wi-Fi Overlap | Recommendation |
|---|---|---|---|
| **11** | 2405 MHz | Overlaps Wi-Fi Ch 1 | OK if Wi-Fi Ch 1 is unused |
| **15** | 2425 MHz | In between Wi-Fi Ch 1 & 6 | **Recommended** |
| **20** | 2450 MHz | In between Wi-Fi Ch 6 & 11 | **Recommended** (Zigbridge Default) |
| **25** | 2475 MHz | Above Wi-Fi Ch 11 | **Strongly Recommended** (Minimal Wi-Fi interference) |
| **26** | 2480 MHz | Edge of band | Caution: Some low-power devices have reduced transmit power on Ch 26 |

---

## 4. Virtual Mock Adapter (Development & CI)

For software development, testing the web dashboard, or running automated unit tests without physical radio hardware:

```yaml
transport:
  type: mock

adapter:
  type: mock
```

In mock mode, Zigbridge boots an in-memory virtual Zigbee radio with simulated devices and responds to all REST and WebSocket requests immediately.
