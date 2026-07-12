# Next — WorthyJoin / Connect

Priority order for upcoming work. Check items off as they land.

## P0 — Stability (investigate now)

- [x] **Reconnect after network change** — Host stays offline after WiFi switch (seen after inventory/heartbeat work). Agent must recover without manual restart.
  - Suspect: inventory collection ran on the heartbeat goroutine and could block during NIC churn (`net.Interfaces` / registry).
  - **Mitigation landed:** inventory refresh is async; heartbeat never waits on sampling. Still verify: rebuild agent → WiFi toggle → machine returns online without manual restart.
  - Remaining: faster reconnect backoff, tray “reconnecting…”, keepalive review (see connection item below).
- [ ] **Strengthen signaling connection** — Occasional disconnects mid-session / idle.
  - Review WS read deadlines, heartbeat cadence, server-side idle drops, NAT/WiFi sleep.
  - Faster reconnect backoff, clearer agent tray state (“reconnecting…”), optional TCP keepalive.

## P1 — Remote tools (tech workflow)

- [ ] **Download file from machine** — At least pull a single file for analysis (path → browser download or tech-side save). Authz + size limits + audit log.
- [ ] **Filesystem browser** — Smooth navigate (drives → folders → files), list/stat, no full-tree dumps; pair with download.
- [ ] **Seamless remote terminal** — Low-latency interactive shell in the viewer/tech UI; reconnect-safe; clear UX when session drops.

## P2 — Security / abuse

- [ ] **Hardening against bots / scrapers / abuse**
  - Rate limits on public install, enroll, auth, admin login, WS upgrade.
  - Bot friction where needed (e.g. enroll/install surfaces): CAPTCHA or equivalent without breaking legit host setup.
  - Tighten CORS, cookies, CSRF on cookie auth; review exposed `/download/*` and `/install*`.
  - Fail2ban-style / IP throttle notes for VPS deploy docs.
  - Audit logging for enroll, join, file pull, terminal open.

## Notes

- Inventory on `/api/agents` is **online + in-memory only**; offline machines won’t show live hardware until reconnect.
- New host inventory requires the rebuilt agent package (`data/agent/agent.zip`) on the machine — old exes won’t send inventory.
- Do not expand scope into process kill / arbitrary remote exec without explicit product decision.
