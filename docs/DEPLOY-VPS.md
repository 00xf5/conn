# Deploy Connect on a VPS (production path)

**Topology:** Windows PC runs `connect-agent` (capture + encode). VPS runs **connectd** (signaling) + **coturn** (TURN relay) behind **Caddy** (HTTPS/WSS).

No Render. One VPS, one domain, UDP relay for cellular viewers.

## Prerequisites

- Linux VPS (Ubuntu 22.04+ recommended), 1 GB RAM minimum
- Docker Engine + Compose plugin
- DNS `A` record: `remote.example.com` → VPS public IP
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
nano .env   # DOMAIN, VPS_PUBLIC_IP, TURN_SECRET
chmod +x setup-vps.sh
./setup-vps.sh
```

`.env` fields:

| Variable | Example | Notes |
|----------|---------|-------|
| `DOMAIN` | `remote.example.com` | Must match DNS + TLS cert |
| `VPS_PUBLIC_IP` | `203.0.113.10` | **Required for coturn** — relay fails without this |
| `TURN_SECRET` | long random string | Same value in connectd + coturn (auto-generated in `coturn.conf`) |

## 2. Verify VPS

```bash
curl -s "https://remote.example.com/api/health" | jq .
# turnExternal: true, iceServers: 2+
curl -s "https://remote.example.com/api/ice" | jq .
# should include turn:... with username + credential
```

## 3. Point Windows agent at VPS

On the host PC (after `.\deploy\start.ps1 -Build`):

```powershell
.\deploy\start-vps-agent.ps1 -Server "wss://remote.example.com/ws"
```

Or copy `deploy/config.vps-agent.example.json` → `%LOCALAPPDATA%\Connect\config.json` and edit `serverUrl`.

## 4. Viewer

| Network | URL |
|---------|-----|
| Any browser | `https://remote.example.com/dashboard/` |
| Phone on cellular | Same URL — TURN relay via coturn |

Create session code on dashboard → connect from phone with Wi‑Fi **off** to prove TURN.

## How ICE works

```
Viewer/Agent ←WSS→ connectd (VPS)
              ←UDP→ coturn (VPS) when direct P2P fails (cellular, symmetric NAT)
              ←P2P→  host PC when LAN/same network allows it
```

connectd advertises ICE servers via `/api/ice`:

- `stun:YOUR_DOMAIN:3478` (coturn)
- `turn:YOUR_DOMAIN:3478?transport=udp` with time-limited credentials (HMAC from `TURN_SECRET`)

Embedded TURN in connectd is **disabled** on VPS (`CONNECT_NO_TURN=1`). coturn runs as a separate container with `network_mode: host`.

## Phase C gate

After VPS is up and agent registered:

```powershell
# Agent log should show WebRTC connected + send_fps >= 15
.\deploy\check-phase-a.ps1

# Optional: verify TURN from Windows
.\deploy\check-vps.ps1 -Domain remote.example.com
```

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `turnExternal: false` in health | Check `CONNECT_TURN_URL` + `CONNECT_TURN_SECRET` in docker-compose / `.env`, restart connectd |
| ICE failed on cellular | Open UDP 3478 + relay range; verify `external-ip` in `coturn.conf` matches VPS public IP |
| Agent offline on dashboard | Agent `serverUrl` must be `wss://DOMAIN/ws`; check agent.log for WS errors |
| Video bad on VPS but LAN OK | Encoder issue on PC — re-run Phase A; do not debug encode on VPS |

## Related

- [RELEASE-GATE.md](RELEASE-GATE.md) — Gate 4 (VPS smoke), Gate 5 (cellular)
- [PHASE-A.md](PHASE-A.md) — Phase C replaces Render
- [ARCHITECTURE.md](ARCHITECTURE.md) — Phase 3 internet path
