# Deploy connectd to Render (legacy / dev only)

> **Production:** **https://worthyjoin.online** on a VPS with coturn — see **[DEPLOY-VPS.md](DEPLOY-VPS.md)**.  
> Render has no UDP TURN relay; cellular viewers will fail without a separate coturn VPS anyway.

Signaling-only smoke test. **connect-agent stays on your Windows PC.**

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
| `CONNECT_TURN_URL` | optional | External TURN on a **separate VPS** (see DEPLOY-VPS.md) |
| `CONNECT_TURN_SECRET` | optional | Shared secret for `CONNECT_TURN_URL` |

Render sets **`PORT`**; connectd binds to it automatically.

## Windows agent

```powershell
.\deploy\start-render-agent.ps1 -Server "wss://your-service.onrender.com/ws"
```

Prefer **`start-vps-agent.ps1`** when using your own domain + coturn.

## Related

- [DEPLOY-VPS.md](DEPLOY-VPS.md) — **recommended production deploy**
- [PHASE-A.md](PHASE-A.md)
