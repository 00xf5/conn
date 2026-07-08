# Stable baseline



This document defines what **must not change** without a perf checklist pass.

Tune stream settings via `config.json` or CLI flags — not by editing encoder internals.



## Stable video pipeline (default)



```

DXGI desktop duplication (capture_only)

  → NV12 frames

  → ffmpeg H.264 (live-probed codec, pipe)

  → H.264 access units

  → WebRTC sample writer (latest-frame + pace)

  → browser viewer

```



**Default capture:** DXGI (`internal/agent/encoder_dxgi_ffmpeg_windows.go`). Requires CGO + gcc (enabled by `deploy/start.ps1 -Build`).



**Fallback:** gdigrab when DXGI fails (`internal/agent/encoder_ffmpeg_windows.go`).



**Dev escape hatch:** set env `CONNECT_ENCODER_GDIGRAB=1` to force gdigrab (still live-probed).



## Codec selection



| Path | How codec is chosen |

|------|---------------------|

| **DXGI (default)** | Live **3s DXGI → ffmpeg probe**; first codec ≥ 12 fps wins; cached to `encoder.json` |

| **gdigrab (fallback / forced)** | Live **3s gdigrab probe** when DXGI unavailable or `CONNECT_ENCODER_GDIGRAB=1` |



Override: `CONNECT_ENCODER_CODEC` env forces a specific codec (must be in probe order).



Probe order: `h264_nvenc` → `h264_amf` → `h264_qsv` → `libx264` (hardware preferred over libx264).



To force re-probe: `CONNECT_ENCODER_REPROBE=1` or delete `encoder.json`.



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



1. After server registration, `preloadEncoder()` runs live codec probe and starts a background warm encode.

2. On viewer connect, `takeWarmEncoder()` reuses the primed pipeline if ready.

3. After session ends, warm encoder starts again.



Rules:



- Never warm while a viewer session is active.

- Live probe runs on every agent start (cache hit skips re-probe unless `CONNECT_ENCODER_REPROBE=1`).



## Local dev (`deploy/start.ps1`)



| Component | Behavior |

|-----------|----------|

| connectd | `-no-tls`, `-public-url http://LAN_IP:8787`, embedded TURN on UDP 3478 |

| connect-agent | `ws://LAN_IP:8787/ws` unless `config.json` overrides |

| Encoder | DXGI + live probe (default); `$env:CONNECT_ENCODER_GDIGRAB="1"` for gdigrab |



## Perf checklist (before merging encoder changes)



**Release gate** — see **`docs/RELEASE-GATE.md`**, **`docs/PHASE-A.md`**, and `.\deploy\check-phase-a.ps1`.



Then a 10-minute viewer session and confirm:



1. `first_frame_ms` in agent log < 3000

2. `send_fps` stays ≥ 15 (target 18+)

3. No `video stalled` log lines

4. No DTS errors, no `encoder EOF`

5. Clicks align with cursor (Fit mode on mobile)

6. Agent process survives disconnect/reconnect



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



- [RELEASE-GATE.md](RELEASE-GATE.md) — must-pass sequence before stable release

- [PHASE-A.md](PHASE-A.md) — local perf gate

- [DEPLOY-RENDER.md](DEPLOY-RENDER.md) — cloud signaling deploy

