# Phase A — local perf gate

Stop context-switching between perf, firewall, and deploy. **Finish Phase A before touching firewall or Render.**

## What Phase A is

One machine, one browser, no phone, no firewall rules, no Render.

```
PC: connectd + connect-agent
PC browser: http://127.0.0.1:8787/dashboard/ → open viewer
```

Network and ICE are out of scope. If video is bad here, deploying will not help.

**Default encoder under test:** gdigrab + ffmpeg (what `start.ps1` runs). Optional: `$env:CONNECT_ENCODER_DXGI="1"` for DXGI path.

## Run a session

```powershell
cd C:\Users\shiver\Desktop\connect
.\deploy\start.ps1 -Build    # first time or after code changes
.\deploy\start.ps1 -Restart  # clean restart

# Optional: watch logs live
.\deploy\start.ps1 -Console
```

1. Open **http://127.0.0.1:8787/dashboard/** on the same PC.
2. Connect to your host and keep the viewer open **at least 2 minutes** (10 minutes before any encoder merge).
3. Move the mouse and click a few times so the stream is not idle.

Log file: `%LOCALAPPDATA%\Connect\agent.log`

## Pass / fail (automated)

```powershell
.\deploy\check-phase-a.ps1
```

Manual equivalent — all must pass for the **latest viewer session**:

| Check | Pass | Fail |
|-------|------|------|
| WebRTC connected | log contains `WebRTC state connected` | stuck on `connecting` or `ICE failed` in viewer |
| First frame | `first_frame_ms` < **3000** | ≥ 3000 or missing after 30s |
| Throughput | `send_fps` ≥ **15** after **60s** uptime | below 15 or no perf line after 60s |
| Stalls | no `video stalled` | any stall line ends the session (15s stall timeout in code) |

The script prints `encoder ready_ms` when present but does **not** fail on codec cache — check agent log for `ffmpeg-gdigrab-*` or `dxgi-*` pipeline name.

**Phase A passed** → move to Phase B (phone on same Wi‑Fi, firewall once).  
**Phase A failed** → tune `config.json` only (`width`, `height`, `fps`, `bitrate`). Do not edit encoder internals ([STABLE.md](STABLE.md)).

## Phase B (after A passes)

Same checklist, but viewer on **phone, same Wi‑Fi**:

```powershell
.\deploy\start.ps1 -Firewall   # once, as Administrator
.\deploy\start.ps1 -Restart
```

Open dashboard at `http://YOUR_LAN_IP:8787/dashboard/`. Run `.\deploy\check-phase-a.ps1` again.

If Phase A passed but Phase B fails → firewall / ICE / TURN, not encoder.

## Phase C (after A + B pass)

Deploy **connectd only** to Render — see **[DEPLOY-RENDER.md](DEPLOY-RENDER.md)**.

Agent stays on the Windows PC with `serverUrl` pointing at Render (`deploy/config.render-agent.example.json`).

Cellular viewers need external TURN on the server (`CONNECT_TURN_URL` + `CONNECT_TURN_SECRET`) — to-be-implemented in ops.

Do not use Render to debug fps or stalls.

## When you're done tuning

Defaults are already the stable baseline (`854×480`, 20 fps, 2000 kbps). If Phase A passes twice in a row with those settings, **stop tuning** and proceed to Phase B.

Force encoder re-probe (DXGI path only):

```powershell
$env:CONNECT_ENCODER_DXGI = "1"
$env:CONNECT_ENCODER_REPROBE = "1"
.\deploy\start.ps1 -Restart
```

Or delete `%LOCALAPPDATA%\Connect\encoder.json`.

## Related

- [STABLE.md](STABLE.md) — frozen baseline and roadmap
- [DEPLOY-RENDER.md](DEPLOY-RENDER.md) — Render deploy
- `deploy/config.example.json` — local LAN config
- `deploy/config.render-agent.example.json` — agent config for Render server
