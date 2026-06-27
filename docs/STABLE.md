# Stable baseline

This document defines what **must not change** without a perf checklist pass.
Tune stream settings via `config.json` or CLI flags — not by editing encoder internals.

## Stable video pipeline (default)

```
ffmpeg gdigrab (desktop BitBlt)
  → scale 854×480 (fast_bilinear)
  → ffmpeg H.264 (cached codec or default h264_qsv)
  → H.264 access units
  → WebRTC sample writer (latest-frame + pace)
  → browser viewer
```

**Default capture:** gdigrab (`internal/agent/encoder_ffmpeg_windows.go`). No CGO required.

**Optional capture (better perf, needs CGO + gcc):** set env `CONNECT_ENCODER_DXGI=1`:

```
DXGI desktop duplication (capture_only)
  → NV12 frames
  → ffmpeg H.264 (live-probed codec, pipe)
  → … same WebRTC path
```

## Codec selection

| Path | How codec is chosen |
|------|---------------------|
| **gdigrab (default)** | `CONNECT_ENCODER_CODEC` env → `%LOCALAPPDATA%\Connect\encoder.json` cache → fallback **`h264_qsv`** |
| **DXGI (`CONNECT_ENCODER_DXGI=1`)** | Live **3s DXGI → ffmpeg probe**; first codec ≥ 12 fps wins; cached to `encoder.json` |

Probe order (DXGI path only): `h264_nvenc` → `h264_amf` → `h264_qsv` → `libx264`.

To force re-probe (DXGI path): `CONNECT_ENCODER_REPROBE=1` or delete `encoder.json`.

**Bump `encoderCacheVersion` in `encoder_codec.go` if probe logic or codec list changes.**

## Not in default build

- In-process native QSV/NVENC (`internal/captureenc/encoder.go` with `-tags experimental`)

## Frozen defaults

| Setting | Value | Config key |
|---------|-------|------------|
| Resolution | 854×480 | `width`, `height` |
| FPS | 20 | `fps` |
| Bitrate | 2000 kbps | `bitrate` |
| GOP | 40 | `gop` |
| Keyint min | 20 | `keyIntMin` |
| Stall timeout | **15s** | (code constant — not in config) |
| Warm prime | 1.2s | (code constant) |

All tunable defaults live in `internal/agent/stream_profile.go`.

**Note:** `SetBitrate()` is currently a **no-op** in ffmpeg/DXGI encoders; changing `bitrate` in config affects the next encoder start only.

## Warm encoder

When the agent is **online with no active viewer**:

1. After server registration, `preloadEncoder()` starts a background warm encode (gdigrab by default).
2. On viewer connect, `takeWarmEncoder()` reuses the primed pipeline if ready.
3. After session ends, warm encoder starts again.

Rules:

- Never warm while a viewer session is active.
- DXGI live probe runs during warm **only** when `CONNECT_ENCODER_DXGI=1`.

## Local dev (`deploy/start.ps1`)

| Component | Behavior |
|-----------|----------|
| connectd | `-no-tls`, `-public-url http://LAN_IP:8787`, embedded TURN on UDP 3478 |
| connect-agent | `ws://LAN_IP:8787/ws` unless `config.json` overrides |
| Encoder | gdigrab (default); set `$env:CONNECT_ENCODER_DXGI="1"` for DXGI |

## Perf checklist (before merging encoder changes)

**Phase A (PC browser only)** — see **`docs/PHASE-A.md`** and `.\deploy\check-phase-a.ps1`.

Then a 10-minute viewer session on Wi‑Fi and confirm:

1. `first_frame_ms` in agent log < 3000
2. `send_fps` stays ≥ 15 (target 18+)
3. No `video stalled` log lines
4. Clicks align with cursor (Fit mode on mobile)
5. Agent process survives disconnect/reconnect

## Safe to tune (config.json)

- `serverUrl`, `width`, `height`, `fps`, `bitrate`, `gop`, `keyIntMin`, `insecureTls`

## Experimental (separate)

```powershell
go build -tags experimental ./experimental/captureenc-test
```

See `experimental/README.md` and `internal/captureenc/README.md`.

## Roadmap (to-be-implemented)

| # | Item | Notes |
|---|------|--------|
| 1 | **Render deploy** | `Dockerfile` + `render.yaml` + [DEPLOY-RENDER.md](DEPLOY-RENDER.md) — connectd only |
| 2 | **External TURN** | `CONNECT_TURN_URL` + `CONNECT_TURN_SECRET` env on server; ops setup (coturn VPS) for cellular |
| 3 | **Auth and branding** | Ed25519 key exists; verification not wired |

## Related

- [PHASE-A.md](PHASE-A.md) — local perf gate
- [DEPLOY-RENDER.md](DEPLOY-RENDER.md) — cloud signaling deploy
