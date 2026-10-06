# Security Policy

The Zigbridge team takes security issues seriously and welcomes responsible vulnerability reporting. This document outlines our security policies, supported versions, and the process for reporting vulnerabilities.

---

## Supported Versions

Security updates and patches are provided for the following versions:

| Version | Supported          | Notes                              |
| ------- | ------------------ | ---------------------------------- |
| 1.x     | :white_check_mark: | Current active release branch      |
| < 1.0   | :x:                | Development / experimental release |

We strongly recommend always running the latest released version of Zigbridge to ensure you have the latest security and stability updates.

---

## Reporting a Vulnerability

**Please do not report security vulnerabilities via public GitHub issues, discussions, or pull requests.**

If you discover a security vulnerability in Zigbridge, please report it privately:

1. **GitHub Security Advisory (Preferred)**:
   - Go to the [Security Advisories](https://github.com/julienbreux/zigbridge/security/advisories) tab of the repository.
   - Click **"Report a vulnerability"** to submit a private draft advisory.

2. **Email**:
   - If you cannot use GitHub Security Advisories, send an encrypted or direct email to **julien.breux@gmail.com** with the subject line `[SECURITY] Zigbridge Vulnerability Report`.

### What to Include in Your Report

To help us triage and resolve the issue quickly, please provide:
- A clear description of the vulnerability and its potential impact.
- Affected component(s) (e.g. ZCL packet parser, Web API, WebSocket handler, MQTT client).
- Step-by-step instructions to reproduce the issue, including proof-of-concept scripts, payloads, or network capture (`.pcap`) files if applicable.
- The version or Git commit of Zigbridge you tested against.
- Coordinator hardware and transport mode used (e.g., SMLIGHT SLZB-06 over TCP, USB serial dongle, or Mock mode).
- Any proposed mitigations or fixes.

---

## Response & Disclosure Process

1. **Acknowledgment**: We will acknowledge receipt of your vulnerability report within **48 hours**.
2. **Assessment & Triage**: We will verify the vulnerability, determine its severity (CVSS), and keep you informed of our progress.
3. **Fix & Verification**: We will develop and test a patch privately. We may ask you to verify the fix before release.
4. **Coordinated Disclosure**: A security advisory and a patched release will be published simultaneously. We will credit you in the advisory release notes (unless you request anonymity).
5. **Timeline**: We aim to resolve critical vulnerabilities within **14 days** and moderate vulnerabilities within **30 days**.

---

## Security Best Practices for Zigbridge Deployments

When running Zigbridge in production, we recommend the following security measures:

1. **Protect Sensitive Configuration**:
   - The runtime `data/` directory and `config.yaml` contain sensitive credentials, including MQTT passwords and Zigbee network keys (`network_key`). Ensure file permissions restrict access to the Zigbridge process user only (`chmod 600 data/config.yaml`).
   - Never commit `data/` or `config.yaml` to version control.
2. **Network Coordinator Isolation**:
   - If using network-attached coordinators (e.g., SMLIGHT SLZB-06 or TubeZB via TCP RFC2217), isolate them on a dedicated IoT VLAN or firewall-restricted subnet accessible only by the Zigbridge host.
3. **MQTT Transport Encryption**:
   - Enable TLS (`mqtt.url: "ssl://..."` or `"tls://..."`) when communicating with external or public MQTT brokers.
4. **Local Network Boundary**:
   - Zigbridge's web management dashboard and REST APIs are intended for local network operation behind a reverse proxy (e.g., NGINX, Traefik, or Caddy) with TLS and authentication if exposed outside a trusted home network.
