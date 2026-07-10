# Deploy BlueConnect on a VPS (production path)

**Topology:** Windows PC runs `connect-agent` (capture + encode). VPS runs **connectd** (signaling) + **coturn** (TURN relay) behind **Caddy** (HTTPS/WSS).

Official domain: **blueconnect.online**. One VPS, one domain, UDP relay for cellular viewers.

## Prerequisites

- Linux VPS (Ubuntu 22.04+ recommended), 1 GB RAM minimum
- Docker Engine + Compose plugin
- DNS `A` record: `blueconnect.online` → VPS public IP
- Phase A + Phase B passed on LAN ([PHASE-A.md](PHASE-A.md))

## Firewall (VPS)

| Port | Protocol | Purpose |
|------|----------|---------|
| 80 | TCP | HTTP → HTTPS redirect |
| 443 | TCP | HTTPS dashboard + WSS signaling |
| 3478 | UDP | STUN + TURN |
| 49152–65535 | UDP | TURN relay media |

## 1. Configure on VPS

```bash
git clone <your-repo> connect && cd connect/deploy
cp .env.example .env
nano .env   # DOMAIN=blueconnect.online, VPS_PUBLIC_IP, TURN_SECRET
chmod +x setup-vps.sh
./setup-vps.sh
```

`.env` fields:

| Variable | Example | Notes |
|----------|---------|-------|
| `DOMAIN` | `blueconnect.online` | Must match DNS + TLS cert |
| `VPS_PUBLIC_IP` | `203.0.113.10` | **Required for coturn** — relay fails without this |
| `TURN_SECRET` | long random string | Same value in connectd + coturn (auto-generated in `coturn.conf`) |

## 2. Verify VPS

```bash
curl -s "https://blueconnect.online/api/health" | jq .
# turnExternal: true, iceServers: 2+
curl -s "https://blueconnect.online/api/ice" | jq .
# should include turn:... with username + credential
```

## 3. Publish Windows agent package (required for Host install links)

On a Windows build PC:

```powershell
.\deploy\publish-agent.ps1 -OutZip .\agent.zip
```

Then open **Admin → Agent package** at `https://blueconnect.online/admin/` and upload `agent.zip` (no SSH/SCP).

Host install (from dashboard link) enrolls the machine and installs the **ConnectAgent** Windows Service (UAC). The service supervisor keeps the interactive capture agent alive across reboot, lock, and crash. If UAC is denied, Startup-folder watchdog is used as fallback.

## 4. Enroll hosts (recommended)

1. Tech signs into `https://blueconnect.online/dashboard/` with an Access code  
2. **Add machine** → copy **install link** → send to the host PC  
3. Host opens the link (or pastes the PowerShell one-liner) → agent downloads, enrolls, appears online  

Lab fallback (manual):

```powershell
.\deploy\start.ps1 -Build
.\deploy\start-vps-agent.ps1
```

Or copy `deploy/config.vps-agent.example.json` → `%LOCALAPPDATA%\Connect\config.json` and edit `serverUrl` if needed.

## 5. Viewer

| Network | URL |
|---------|-----|
| Any browser | `https://blueconnect.online/dashboard/` |
| Phone on cellular | Same URL — TURN relay via coturn |

Create session code on dashboard → connect from phone with Wi‑Fi **off** to prove TURN.

## How ICE works

```
Viewer/Agent ←WSS→ connectd (VPS)
              ←UDP→ coturn (VPS) when direct P2P fails (cellular, symmetric NAT)
              ←P2P→  host PC when LAN/same network allows it
```

connectd advertises ICE servers via `/api/ice`:

- `stun:blueconnect.online:3478` (coturn)
- `turn:blueconnect.online:3478?transport=udp` with time-limited credentials (HMAC from `TURN_SECRET`)

Embedded TURN in connectd is **disabled** on VPS (`CONNECT_NO_TURN=1`). coturn runs as a separate container with `network_mode: host`.

## Phase C gate

After VPS is up and agent registered:

```powershell
# Agent log should show WebRTC connected + send_fps >= 15
.\deploy\check-phase-a.ps1

# Optional: verify TURN from Windows
.\deploy\check-vps.ps1
```

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `turnExternal: false` in health | Check `CONNECT_TURN_URL` + `CONNECT_TURN_SECRET` in docker-compose / `.env`, restart connectd |
| ICE failed on cellular | Open UDP 3478 + relay range; verify `external-ip` in `coturn.conf` matches VPS public IP |
| Agent offline on dashboard | Agent `serverUrl` must be `wss://blueconnect.online/ws`; check agent.log for WS errors |
| Video bad on VPS but LAN OK | Encoder issue on PC — re-run Phase A; do not debug encode on VPS |

## Related

- [RELEASE-GATE.md](RELEASE-GATE.md) — Gate 4 (VPS smoke), Gate 5 (cellular)
- [PHASE-A.md](PHASE-A.md) — Phase C replaces Render
- [ARCHITECTURE.md](ARCHITECTURE.md) — Phase 3 internet path
