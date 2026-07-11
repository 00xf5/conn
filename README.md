# WorthyJoin

Self-hosted remote desktop (**worthyjoin.online**): **Go signaling server**, **Windows host agent**, **browser viewer**.

## Components

| Binary | Role |
|--------|------|
| `connectd` | Registry, WebSocket signaling, dashboard + viewer, optional TLS + embedded TURN (LAN) |
| `connect-agent` | Host: **DXGI** (default) or gdigrab fallback → ffmpeg H.264 → WebRTC, input injection |

## Quick start (developers)

```powershell
winget install Gyan.FFmpeg
winget install -e BrechtSanders.WinLibs.POSIX.UCRT   # gcc — required for DXGI default path

cd C:\Users\shiver\Desktop\connect
$env:CONNECT_ALLOW_SYSTEM_FFMPEG = "1"
.\deploy\start.ps1 -Build
.\deploy\start.ps1
```

Fallback capture (no CGO / DXGI unavailable):

```powershell
$env:CONNECT_ENCODER_GDIGRAB = "1"
.\deploy\start.ps1 -Restart
```

## End users (one download)

Ship a single folder or zip for ops — hosts only download **WorthyJoin-Setup.exe** from the install link (zip stays on the server):

```
Connect/
  connect-agent.exe   (~20 MB)
  bin/ffmpeg.exe      (~30 MB minimal, see below)
  config.json
```

The agent **only** uses `bin\ffmpeg.exe` beside itself in production. System PATH/winget is ignored unless `CONNECT_ALLOW_SYSTEM_FFMPEG=1` (dev only — `start.ps1` sets this when winget ffmpeg is found).

**ffmpeg size:** full Gyan install is ~650 MB — we ship **one** trimmed `ffmpeg.exe` (~25–40 MB) with `h264_nvenc`, `h264_amf`, `h264_qsv`, `libx264`.

**Total host download:** ~50–60 MB (agent + ffmpeg).

Dashboard (local dev): **http://YOUR_LAN_IP:8787/dashboard/**

Phone can't reach PC? Run once as Administrator:

```powershell
.\deploy\start.ps1 -Firewall
```

Stuck? `.\deploy\start.ps1 -Restart`

Debug logs: `.\deploy\start.ps1 -Console`

Stream settings: `%LOCALAPPDATA%\Connect\config.json` (see `deploy/config.example.json`).

## Default video path (code)

```
DXGI → NV12 → ffmpeg H.264 (live-probed) → WebRTC → browser
```

Fallback when DXGI fails (or `CONNECT_ENCODER_GDIGRAB=1`):

```
gdigrab → ffmpeg H.264 (live-probed) → WebRTC → browser
```

Defaults: **854×480 @ 20fps, 2 Mbps**. Tune via `config.json` — not by editing encoder internals.

See **`docs/STABLE.md`** for frozen rules, **`docs/RELEASE-GATE.md`** for must-pass gates, **`docs/PHASE-A.md`** for the local perf workflow, **`docs/DEPLOY-RENDER.md`** for cloud deploy.

After a viewer session: `.\deploy\check-phase-a.ps1`

## Deploy server to Render

**connectd only** — agent stays on Windows. See **`docs/DEPLOY-RENDER.md`**.

Quick pointer: production is **https://worthyjoin.online** (VPS). For a throwaway Render smoke test, set agent `serverUrl` to that service’s `wss://…/ws` — see [DEPLOY-RENDER.md](docs/DEPLOY-RENDER.md).

## Build (manual)

```powershell
go build -o connectd.exe ./cmd/connectd
$env:CGO_ENABLED="1"
go run ./cmd/connect-agent/genicon.go
go build -ldflags "-H=windowsgui" -o connect-agent.exe ./cmd/connect-agent
```

## Architecture

```
connect-agent ──WSS──► connectd ◄── session code ── browser
       └──── WebRTC (H.264 + input DataChannel) ────┘
```

## Project layout

```
cmd/connectd/           signaling server (VPS Docker or LAN)
cmd/connect-agent/      host agent (tray)
internal/agent/         WebRTC, encoders, stream profile
internal/captureenc/    DXGI + in-process HW encode
internal/server/        HTTP, signaling, embedded web
deploy/start.ps1        local LAN dev
deploy/setup-vps.sh     VPS: connectd + coturn + Caddy
deploy/start-vps-agent.ps1   point agent at VPS
docs/DEPLOY-VPS.md      production deploy (recommended)
docs/DEPLOY-RENDER.md   legacy signaling-only smoke test
Dockerfile              connectd container
```

## Internet / cellular (Phase C)

After Phase A + B pass locally, deploy on a VPS:

```bash
# VPS
cd deploy && cp .env.example .env && ./setup-vps.sh
```

```powershell
# Windows host
.\deploy\start-vps-agent.ps1
.\deploy\check-vps.ps1
```

See **[docs/DEPLOY-VPS.md](docs/DEPLOY-VPS.md)** — coturn TURN relay for cellular viewers.

## Experimental native encoder

In-process NVENC/QSV is **not** in the default agent build:

```powershell
go build -tags experimental -o captureenc-test.exe ./experimental/captureenc-test
```

## License

Private / self-hosted use.
