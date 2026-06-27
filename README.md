# Connect

Self-hosted remote desktop: **Go signaling server**, **Windows host agent**, **browser viewer**.

## Components

| Binary | Role |
|--------|------|
| `connectd` | Registry, WebSocket signaling, dashboard + viewer, optional TLS + embedded TURN (LAN) |
| `connect-agent` | Host: **gdigrab** (default) or DXGI capture → ffmpeg H.264 → WebRTC, input injection |

## Quick start (developers)

```powershell
winget install Gyan.FFmpeg
winget install -e BrechtSanders.WinLibs.POSIX.UCRT   # gcc — only if using DXGI path

cd C:\Users\shiver\Desktop\connect
$env:CONNECT_ALLOW_SYSTEM_FFMPEG = "1"
.\deploy\start.ps1 -Build
.\deploy\start.ps1
```

Optional faster capture (requires CGO build):

```powershell
$env:CONNECT_ENCODER_DXGI = "1"
.\deploy\start.ps1 -Restart
```

## End users (one download)

Ship a single folder or zip — no winget, no separate ffmpeg install:

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
gdigrab → ffmpeg H.264 (cache or h264_qsv) → WebRTC → browser
```

Optional (`CONNECT_ENCODER_DXGI=1`):

```
DXGI → NV12 → ffmpeg H.264 (live-probed) → WebRTC → browser
```

Defaults: **854×480 @ 20fps, 2 Mbps**. Tune via `config.json` — not by editing encoder internals.

See **`docs/STABLE.md`** for frozen rules, **`docs/PHASE-A.md`** for the local perf gate, **`docs/DEPLOY-RENDER.md`** for cloud deploy.

After a viewer session: `.\deploy\check-phase-a.ps1`

## Deploy server to Render

**connectd only** — agent stays on Windows. See **`docs/DEPLOY-RENDER.md`**.

Quick pointer: push repo → Render Blueprint (`render.yaml`) or Docker web service → set agent `serverUrl` to `wss://YOUR-SERVICE.onrender.com/ws`.

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
cmd/connectd/           server (Render deploy target)
cmd/connect-agent/      host agent (tray)
internal/agent/         WebRTC, encoders, stream profile
internal/captureenc/    DXGI capture (optional agent path)
internal/server/        HTTP, signaling, embedded web
deploy/start.ps1        local dev script
deploy/config.example.json
docs/STABLE.md          baseline + roadmap
docs/DEPLOY-RENDER.md   Render deploy
Dockerfile              connectd container
render.yaml             Render blueprint
```

## To-be-implemented

- Auth on dashboard / WebSocket (Ed25519 key generated but not verified)
- External TURN ops for cellular (env hooks exist: `CONNECT_TURN_URL`, `CONNECT_TURN_SECRET`)
- Runtime bitrate changes (`SetBitrate` is currently a no-op)

## Experimental native encoder

In-process NVENC/QSV is **not** in the default agent build:

```powershell
go build -tags experimental -o captureenc-test.exe ./experimental/captureenc-test
```

## License

Private / self-hosted use.
