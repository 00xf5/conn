# Release gate — must pass before calling Connect stable

Do not skip steps. Each gate produces objective pass/fail from `agent.log` and the dashboard.

## Gate 1 — Phase A localhost (2×)

**Environment:** one PC, browser on same PC, no phone, no firewall changes, no Render.

```powershell
cd C:\Users\shiver\Desktop\connect
.\deploy\start.ps1 -Build
.\deploy\start.ps1 -Restart

# Clear stale encoder cache once after upgrading to DXGI default
Remove-Item "$env:LOCALAPPDATA\Connect\encoder.json" -ErrorAction SilentlyContinue

# Session 1: open dashboard, connect, keep viewer open ≥2 min (move mouse + click)
.\deploy\check-phase-a.ps1

# Session 2: disconnect, reconnect, repeat
.\deploy\check-phase-a.ps1
```

**Pass:** both runs exit 0. Log must show `agent: pipeline capture=dxgi ...` (or gdigrab with live probe) and no `video stalled`, no DTS errors, no `encoder EOF`.

## Gate 2 — Soak (10 min)

Same PC, one session kept open **≥10 minutes** with occasional mouse movement:

```powershell
.\deploy\check-phase-a.ps1 -SoakMinutes 10
```

**Pass:** script exit 0 with perf line at ≥600s uptime and `send_fps >= 15`.

## Gate 3 — Phase B LAN phone

After Gate 1 + 2 pass:

```powershell
.\deploy\start.ps1 -Firewall   # once, Administrator
.\deploy\start.ps1 -Restart
```

Viewer on phone (same Wi‑Fi): `http://YOUR_LAN_IP:8787/dashboard/`

```powershell
.\deploy\check-phase-a.ps1
```

**Pass:** script exit 0. If Gate 1 passed but Gate 3 fails → firewall / ICE / TURN, not encoder.

## Gate 4 — VPS signaling + coturn

**Only after Gates 1–3 pass.** VPS runs connectd + coturn + Caddy ([DEPLOY-VPS.md](DEPLOY-VPS.md)).

```bash
# On VPS
cd deploy && ./setup-vps.sh
```

```powershell
# On Windows host
.\deploy\start-vps-agent.ps1
.\deploy\check-vps.ps1
```

**Pass:**

1. `check-vps.ps1` exit 0 (`turnExternal: true`, TURN credentials present)
2. Dashboard shows agent online ≥5 minutes
3. Viewer session: log contains `WebRTC state connected`, `send_fps >= 15`
4. Cellular test: phone on LTE connects (proves coturn relay)

## Gate 5 — Cellular soak (optional)

10-minute session on phone **cellular only** (Wi‑Fi off). Same perf bar as Gate 2.

If Gate 4 passes on LAN/internet browser but Gate 5 fails → coturn firewall / `external-ip` / UDP relay range.

## Automated checks (all gates)

| Check | Pass |
|-------|------|
| WebRTC connected | log contains `WebRTC state connected` |
| First frame | `first_frame_ms` < 3000 |
| Throughput | `send_fps` ≥ 15 after 60s (600s for soak) |
| Stalls | no `video stalled` |
| Pipeline | `agent: pipeline capture=` or live probe log |
| FFmpeg | no `non monotonically increasing dts` |
| Encoder | no `encoder EOF` |

## Session proof lines

Every session should end with:

```
agent: session XXXXX summary connected=true pipeline=dxgi-h264_qsv send_fps=19.1 stalled=false duration_s=120
```

Startup should log:

```
agent: pipeline capture=dxgi codec=h264_qsv probe_fps=18.2 fallback=none
```

## When all gates pass

- Defaults in `config.json` stay **854×480 @ 20fps, 2 Mbps**
- Do not tune encoder internals — tune config only
- Tag release and ship host bundle with `bin\ffmpeg.exe`

## Related

- [PHASE-A.md](PHASE-A.md) — local perf workflow
- [STABLE.md](STABLE.md) — frozen baseline
- [DEPLOY-RENDER.md](DEPLOY-RENDER.md) — Render deploy
