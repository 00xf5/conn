# Next — WorthyJoin / Connect

Priority order for upcoming work. Check items off as they land.

## P0 — Stability

- [x] **Reconnect after network change** — Inventory refresh is async; heartbeat never waits on sampling.
- [x] **Strengthen signaling connection**
  - Exponential reconnect backoff (1s → 30s), reset after a live session.
  - Tray state `reconnecting…` while dialing / waiting.
  - TCP keepalive (30s) on the WebSocket dialer.
  - Existing: app heartbeat 15s + server WS PingPump 30s + 90s read deadlines (unchanged; already correct).

## P1 — Remote tools (tech workflow)

- [x] **Download file from machine** — `fs_get` + transfer folder.
- [x] **Filesystem browser** — `fs_roots` / `fs_list`.
- [x] **Local input lock** — `BlockInput`; unlock on session end.
- [x] **Remote terminal (ConPTY)** — Full Windows TTY via ConPTY + xterm.js; `term_open` / `term_in` / `term_out` / `term_resize` / `term_close`; killed on session end.

## P2 — Security / abuse (skipped for now)

- [ ] **Hardening against bots / scrapers / abuse** — deferred by choice; revisit later.

## Notes

- Inventory on `/api/agents` is **online + in-memory only**; offline machines won’t show live hardware until reconnect.
- New host features require a rebuilt agent on the machine — old exes won’t send inventory / ConPTY / fs_* / input lock.
- Do not expand scope into process kill / arbitrary remote exec without explicit product decision.
