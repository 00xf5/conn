# Deploy connectd to Render

Signaling server only. **connect-agent stays on your Windows PC.**

## Prerequisites

1. Phase A passed locally (`.\deploy\check-phase-a.ps1`) — see [PHASE-A.md](PHASE-A.md)
2. GitHub repo pushed with this codebase
3. Render account (free tier works for testing)

## One-click (Blueprint)

1. In Render: **New → Blueprint** → connect this repo
2. Render builds `Dockerfile` and starts `connectd` with `-no-tls -no-turn`
3. Note your service URL, e.g. `https://connectd-xxxx.onrender.com`

`connectd` reads **`RENDER_EXTERNAL_URL`** automatically for viewer links and ICE hostnames. Override with **`CONNECT_PUBLIC_URL`** if needed.

## Manual web service

| Setting | Value |
|---------|--------|
| Runtime | Docker |
| Dockerfile path | `./Dockerfile` |
| Health check | `/api/health` |

Environment (set in Render dashboard):

| Variable | Required | Value |
|----------|----------|--------|
| `CONNECT_NO_TLS` | yes | `true` |
| `CONNECT_NO_TURN` | yes | `true` |
| `CONNECT_STUN_URLS` | recommended | `stun:stun.l.google.com:19302,stun:stun1.l.google.com:19302` |
| `CONNECT_PUBLIC_URL` | optional | `https://your-service.onrender.com` (auto via `RENDER_EXTERNAL_URL`) |
| `CONNECT_TURN_URL` | optional | External TURN for cellular (see below) |
| `CONNECT_TURN_SECRET` | optional | Shared secret for `CONNECT_TURN_URL` |
| `CONNECT_ICE_SERVERS` | optional | Full JSON override (advanced) |

Render sets **`PORT`**; connectd binds to it automatically.

## Point the Windows agent at Render

Copy `deploy/config.render-agent.example.json` to `%LOCALAPPDATA%\Connect\config.json` (or repo `config.json` for dev):

```json
{
  "serverUrl": "wss://YOUR-SERVICE.onrender.com/ws",
  "insecureTls": false
}
```

Restart connect-agent. Tray → **Open dashboard** should open your Render URL.

## Verify

1. `GET https://YOUR-SERVICE.onrender.com/api/health` → `"ok": true`
2. Dashboard shows agent online after connect-agent starts
3. Create session code → open viewer link → video + input

Check agent log: `%LOCALAPPDATA%\Connect\agent.log`

## ICE / TURN notes

- **Embedded TURN does not run on Render** (UDP not available on web services).
- **Same-network / simple NAT:** public STUN (`CONNECT_STUN_URLS`) is usually enough.
- **Cellular / symmetric NAT:** set `CONNECT_TURN_URL` + `CONNECT_TURN_SECRET` pointing at an external coturn VPS (to-be-implemented in ops — not bundled in this repo).

## Limitations (current codebase)

| Item | Status |
|------|--------|
| Auth on dashboard / API | To-be-implemented |
| Ed25519 server key | Generated; not used for verification yet |
| Ephemeral disk | Sessions + `data/server.key` reset on redeploy |
| Free tier spin-down | Service sleeps when idle; agent reconnects in ~3s |
| Adaptive bitrate (`SetBitrate`) | No-op in encoder; config bitrate is fixed at session start |

See [STABLE.md](STABLE.md) for the full baseline and roadmap.
