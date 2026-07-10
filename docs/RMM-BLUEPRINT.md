# Connect RMM blueprint (Access + multi-tenant)

Canonical product blueprint for packaging Connect as a ScreenConnect-style **Access RMM**, with a **Main Admin** console that issues accounts, and the existing Host dashboard as the **tenant tech console**.

Wire protocol (DXGI → H.264 → WebRTC) stays as in [ARCHITECTURE.md](ARCHITECTURE.md). This document owns **identity, tenancy, auth, and console surfaces**.

---

## 1. Product north star

**Access-first RMM:** agents stay online under a tenant; techs log into the Host dashboard; pick a machine; Join.

Not RustDesk: no primary “type Peer ID + password” flow.  
Not Support-first: guest “enter join code with no agent” is Phase later.

```
Main Admin  ──issues Access account──►  Tech (tenant console / Host dashboard)
                                              │
                                              │ Join (session ticket)
                                              ▼
                                         Viewer /v/{CODE}
                                              ▲
                                              │ WebRTC
                                         Agent (PC)
                                              │
                                         tenant-bound register
```

---

## 2. Three surfaces (do not merge)

| Surface | Route (target) | Actor | Purpose |
|---------|----------------|-------|---------|
| **Main Admin** | `/admin/` | Platform owner | Tenants, issue/revoke Access accounts, enrollment, health |
| **Host dashboard** | `/dashboard/` | Tech (per tenant) | Machine list → Join / Share (current UI, gated by auth) |
| **Viewer** | `/v/{CODE}` | Tech (or shared link holder) | Live remote session |

Optional later: Support guest landing (attended). Out of scope until Access + tenancy works.

---

## 3. Two kinds of “code” (never conflate)

| Name | What it is | Who issues | Lifetime | Used for |
|------|------------|------------|----------|----------|
| **Access code** | Invite / account redeem token | Main Admin | One-time or short TTL; then becomes login session | Auth into Host dashboard |
| **Session ticket** | 6-char `/v/CODE` | Host dashboard (`POST /api/session`) | ~minutes; replace-on-create; delete on viewer leave | Join / Share remote desktop |

Rules:

- Access codes are **identity**.  
- Session tickets are **connection handles**.  
- Do **not** use session tickets as tech login.

---

## 4. Roles

| Role | Auth | Capabilities |
|------|------|--------------|
| **Platform admin** | Bootstrap secret / admin account (POC) | Full `/admin` |
| **Tech** | Redeem Access code → cookie/JWT | Host dashboard for **one tenant**; Join machines in that tenant |
| **Agent** | Device UUID + tenant binding | Register, stream, accept Join |
| **Shared viewer** (optional) | Valid session ticket only | `/v/CODE` without full tech login (Share link) — policy flag later |

POC default: Share link still works with session ticket alone (current behavior). Harden later if needed.

---

## 5. Data model (minimum)

Persist on `connectd` (SQLite preferred; JSON file acceptable for first POC). Restart-safe.

### Tenant

| Field | Notes |
|-------|--------|
| `id` | Stable ID (UUID) |
| `name` | Display name (“Acme IT”) |
| `createdAt` | |
| `status` | `active` \| `suspended` |

### AccessAccount

| Field | Notes |
|-------|--------|
| `id` | |
| `tenantId` | FK |
| `label` | Optional (“Alex – helpdesk”) |
| `role` | `tech` (only role in POC) |
| `accessCodeHash` | Hash of redeemable code (never store plaintext after issue UI) |
| `status` | `pending` \| `active` \| `revoked` |
| `redeemedAt` | |
| `expiresAt` | Optional on unused invite |
| `createdAt` | |

**Issue flow:** Admin creates account → UI shows **plaintext Access code once** → store hash only.

### Agent binding

| Field | Notes |
|-------|--------|
| `deviceId` | Existing UUID (`device.id`) |
| `tenantId` | Required once multi-tenant is on |
| `hostname` | Existing |
| `lastSeen` / `connected` | Existing registry fields |
| `encoder` / `resolution` | Optional metadata |

In-memory registry today (`internal/rendezvous`) gains `tenantId`; durable map optional for offline history later.

### Session ticket (existing)

Unchanged semantics ([`internal/session`](../internal/session/session.go)):

- Bound to `deviceId`
- TTL ~30m
- **One open ticket per device** (replace-on-create)
- Delete when viewer disconnects
- List purges expired

### Auth session (tech cookie/JWT)

| Field | Notes |
|-------|--------|
| `accountId` | |
| `tenantId` | |
| `expiresAt` | |

Signed by server key (reuse/extend `data/server.key` or dedicated JWT secret).

---

## 6. Auth flows

### 6.1 Admin bootstrap (POC)

- Env: `CONNECT_ADMIN_TOKEN` (or first-run generated token printed once)
- `/admin/` requires this token (header, cookie after login form, or Basic — pick one and document)
- Not end-user Auth0 in v1

### 6.2 Issue Access account

1. Admin selects/creates tenant  
2. Admin clicks **Issue Access account**  
3. Server generates high-entropy Access code (not 6-char session alphabet collision-prone)  
4. UI shows code **once** + copy  
5. Store hash; status `pending`

### 6.3 Tech redeem → Host dashboard

1. Open `/dashboard/` (unauthenticated) → redeem / login screen  
2. Submit Access code (+ optional label later)  
3. Server verifies hash, marks `active`, sets HttpOnly cookie / JWT  
4. Redirect into Host UI (current 3-pane Access console)

### 6.4 API gate

Authenticated (tech cookie):

- `GET /api/agents` — **only** agents with matching `tenantId`
- `POST /api/session` — only if `deviceId` belongs to tech’s tenant
- `GET /api/sessions` — only tickets for that tenant’s devices

Admin token:

- All `/api/admin/*`

Public / ticket-auth:

- `GET /v/{CODE}`, viewer WS join — session ticket validity (unchanged)
- `GET /api/ice`, `/api/health` — decide: health public or admin-only (prefer public health for probes; agents API never public)

### 6.5 Revoke

- Admin sets AccessAccount `revoked`  
- Invalidate outstanding tech sessions for that account  
- Agent binding stays; new techs for same tenant still work unless tenant suspended

---

## 7. Agent enrollment (tenant binding)

Agents must not join a global pool after multi-tenant ships.

**POC-acceptable options (pick one in implementation; document in release notes):**

1. **Config field** — `tenantId` in `%LOCALAPPDATA%\Connect\config.json` (admin-provided)  
2. **Enrollment code** — one-time code from Admin redeemable by agent on first connect; server maps device → tenant  

Recommended long-term: (2). Acceptable first cut: (1) for internal testing.

Register WS query/body must include `tenantId` (or enrollment redemption before `registered`).

---

## 8. Host dashboard behavior (locked)

Keep ScreenConnect-style Access Host page:

- Top: Access (active) / Support (disabled until later)  
- Left: Session groups  
- Center: Machine list + filter  
- Right: Detail + **Join** / Share link  
- Double-click row = Join  

Auth wrapper:

- No cookie → redeem screen  
- Show tenant name in top bar after login  
- Logout clears cookie  

Session leak rules (already intended):

- Join/Share replace prior ticket for that device  
- Leave viewer deletes ticket  
- Active Sessions count = open tickets, not historical Joins  

---

## 9. Main Admin UI (locked scope for v1)

**Must have**

- Login with admin bootstrap  
- Tenant list / create  
- Access accounts: issue (show code once), list, revoke  
- Agents overview (read-only): device, hostname, tenant, last seen  
- Basic health: agent count, version stamp  

**Must not have in v1**

- Billing, SSO/OIDC, LDAP  
- Per-machine ACL matrices  
- Support guest portal  
- Full branding pack / white-label  
- Installer builder UI (document manual config first)

---

## 10. API sketch

### Admin

| Method | Path | Notes |
|--------|------|-------|
| POST | `/api/admin/login` | Bootstrap token → admin session |
| GET/POST | `/api/admin/tenants` | List / create |
| POST | `/api/admin/tenants/{id}/access-accounts` | Issue; response includes plaintext code once |
| GET | `/api/admin/tenants/{id}/access-accounts` | List (no hashes, no plaintext) |
| POST | `/api/admin/access-accounts/{id}/revoke` | Revoke |
| GET | `/api/admin/agents` | All agents + tenant |

### Tech / Host

| Method | Path | Notes |
|--------|------|-------|
| POST | `/api/auth/redeem` | Access code → tech session |
| POST | `/api/auth/logout` | |
| GET | `/api/me` | account + tenant |
| GET | `/api/agents` | Scoped |
| POST | `/api/session` | Scoped; replace-on-create |
| GET | `/api/sessions` | Scoped |
| DELETE | `/api/session?code=` | Optional explicit end |

### Agent / media

Existing WS + ICE; add tenant on register. Media stack unchanged.

---

## 11. Security checklist (do not skip)

- [ ] Access codes hashed at rest (argon2id or bcrypt)  
- [ ] Plaintext Access code shown once only  
- [ ] Tech cookie HttpOnly + Secure (+ SameSite) on HTTPS  
- [ ] CSRF strategy for cookie POSTs (or bearer-only API)  
- [ ] Rate-limit redeem + admin login  
- [ ] Agent registration rejects unknown/missing tenant when multi-tenant enforced  
- [ ] Session tickets remain unguessable (existing alphabet/length OK for short TTL)  
- [ ] Admin bootstrap token not committed; env/docs only  
- [ ] Revoke immediately kills tech API access  

---

## 12. Explicit non-goals (remember why)

| Non-goal | Why |
|----------|-----|
| RustDesk Peer ID as primary join | Wrong product model |
| 6-char Join ticket as tech login | Collides with session semantics |
| Global unscoped `/api/agents` after tenants | Breaks multi-tenant |
| Building Support before Access auth | Wrong order |
| Replacing WebRTC for tenancy | Orthogonal |

---

## 13. Implementation phases (ordered)

### Phase M0 — Session hygiene (done / verify on VPS)

- Replace-on-create, delete on viewer leave, list purge  
- Redeploy `connectd` so Host UI + leak fix are live  

### Phase M1 — Persistence + tenants

- [x] SQLite store: tenants, access accounts, agent bindings (`internal/store`)
- [x] Access-code hash (bcrypt) + HMAC session cookies (`internal/auth`)

### Phase M2 — Main Admin UI

- [x] `/admin/` issue/revoke Access accounts, create tenants

### Phase M3 — Tech auth on Host dashboard

- [x] Redeem gate, cookie, `/api/me`
- [x] Scope agents/sessions/create by tenant

### Phase M4 — Agent tenant binding

- [x] `tenantId` in agent config / `-tenant` flag; WS register + SQLite binding
- [ ] Optional: reject unbound agents with `CONNECT_REQUIRE_TENANT=1` (flag exists; enable in prod)

### Phase M5 — Harden

- [x] Rate limits on admin login + redeem
- [x] Revoke kills tech API access (checked on each request)
- [ ] Audit log (who joined whom)
- [ ] Optional: require tech login for Share (disable anonymous `/v`)

### Phase M6 — Support (optional)

- Attended guest join product; separate from Access codes

**Ops notes**

- Set `CONNECT_ADMIN_TOKEN` before production (otherwise a one-shot token is logged at startup).
- SQLite file: `data/connect.db` (override with `-db`).
- Bind agents with `"tenantId": "<uuid>"` in config or `-tenant`.
- Access codes are **not** session tickets (`/v/CODE`).
---

## 14. Acceptance criteria (do not ship M3 without)

1. Admin creates tenant **T**, issues Access code **A**  
2. Tech redeems **A**, sees only agents enrolled in **T**  
3. Second tenant **U** cannot see **T** machines  
4. Join still opens viewer; Leave drops Active Sessions ticket  
5. Revoke **A** → next API call 401; redeem fails  
6. Session ticket ≠ Access code (different formats/lifetimes)  

---

## 15. Doc map

| Doc | Owns |
|-----|------|
| [ARCHITECTURE.md](ARCHITECTURE.md) | Capture / encode / WebRTC / signaling |
| **This file** | Tenancy, Admin, Access accounts, Host auth |
| [DEPLOY-VPS.md](DEPLOY-VPS.md) | How to run `connectd` + coturn |
| [STABLE.md](STABLE.md) | Stream tunables only |

When implementing, update this blueprint if the data model or code types change — do not silently diverge.
