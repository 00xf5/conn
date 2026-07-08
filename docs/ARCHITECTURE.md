# Connect architecture (canonical)

This document locks the product architecture. New code must fit here — no parallel stacks.

## Goal

Control a Windows PC from a browser **over the internet**, without renting a VPS.

## Layers (single owner each)

| Layer | Owner | Notes |
|-------|--------|------|
| Capture | DXGI Desktop Duplication (`captureenc`) | gdigrab = emergency fallback only |
| Encode | Media Foundation H.264 MFT in-process | ffmpeg = legacy fallback only |
| Signaling | `connectd` WebSocket hub | existing `internal/signaling` |
| Media (happy path) | WebRTC (Pion) H.264 + DataChannel | browser `<video>` |
| Media (ICE fail) | connectd WSS relay (planned) | forward H.264, no transcode |
| Input | DataChannel → `SendInput` | existing `inputproto` |
| Viewer | Browser WebRTC | no custom TCP video protocol |

## Data flow

```
DXGI NV12 → MF H.264 MFT → Annex-B access unit → Pion WriteSample → WebRTC → browser
Input: browser → DataChannel → agent SendInput
Setup: agent/viewer ↔ connectd WSS (SDP/ICE relay)
```

## Explicitly rejected (no redundancy)

- Custom 0xDEAD frame packets over TCP alongside WebRTC
- ffmpeg as the default live encoder (fallback only)
- Relay transcode (agent H.264 → VP8) unless browser path is abandoned
- Second input path over TCP
- LAN-only as the product definition
- User-managed VPS for signaling + coturn (see `docs/DEPLOY-VPS.md`); Render is legacy/dev-only

## Phases

### Phase 1 (current) — correct host encode

- [x] Video gate opens after viewer SDP answer
- [x] **Host pipeline** `captureenc.HostPipeline` (DXGI → NVENC/QSV in-process, Annex-B AU)
- [x] H.264 access-unit validation (`MinKeyframeBytes`, `MinDeltaBytes`)
- [x] SDP `profile-level-id` from stream SPS
- [x] `cmd/encode-gate` — local PASS/FAIL without browser
- [ ] MF H.264 as secondary fallback (when HW encoders unavailable)
- [x] ffmpeg demoted to `CONNECT_ENCODER_FFMPEG=1` fallback only

### Phase 2 — performance

- DXGI dirty-rect / skip unchanged frames
- Capture → encode → WebRTC ring buffers (depth 2–3)
- Real adaptive bitrate + resolution from viewer RTT/loss

### Phase 3 — internet (VPS + coturn)

- WebRTC P2P + STUN first (direct when possible)
- **coturn on VPS** for cellular / symmetric NAT (`deploy/setup-vps.sh`)
- connectd WSS on VPS behind Caddy (`docs/DEPLOY-VPS.md`)
- connectd WSS media relay when ICE fails (planned — forward NALs, no transcode)

### Phase 4 — polish

- Windows service install, multi-monitor, auth, installer

## Success metrics

| Check | Target |
|-------|--------|
| `send_fps` | ≥ 15 after 60s (`check-phase-a.ps1`) |
| `first_frame_ms` | < 500 warm, < 3000 cold |
| Artifacts | No green/crack; no 48-byte P-frames |
| Internet | Viewer on cellular connects (Phase 3) |

## Related

- [STABLE.md](STABLE.md) — stream tunables via config
- [RELEASE-GATE.md](RELEASE-GATE.md) — pass/fail gates
