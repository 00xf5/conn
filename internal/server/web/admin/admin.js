let selectedTenant = null;
let issuedCode = "";
let issuedEnrollCode = "";
let tenantsCache = [];

async function api(path, opts = {}) {
  const res = await fetch(path, {
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...(opts.headers || {}) },
    ...opts,
  });
  const text = await res.text();
  let data = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = text;
    }
  }
  if (!res.ok) {
    const msg = typeof data === "string" ? data : data?.error || text || res.statusText;
    throw new Error(msg.trim() || res.statusText);
  }
  return data;
}

function toast(msg) {
  const el = document.getElementById("toast");
  el.hidden = false;
  el.textContent = msg;
  clearTimeout(toast._t);
  toast._t = setTimeout(() => {
    el.hidden = true;
  }, 3000);
}

function escapeHtml(s) {
  return String(s ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function showApp(authed) {
  document.getElementById("login").hidden = !!authed;
  document.getElementById("app").hidden = !authed;
}

function setTab(name) {
  document.querySelectorAll(".nav-btn").forEach((b) => {
    b.classList.toggle("active", b.dataset.tab === name);
  });
  document.querySelectorAll(".tab").forEach((t) => {
    t.classList.toggle("active", t.id === `tab-${name}`);
  });
  const titles = {
    tenants: "Tenants",
    accounts: "Access codes",
    enrollments: "Enrollments",
    agents: "Agents",
  };
  document.getElementById("page-title").textContent = titles[name] || name;
}

async function boot() {
  showApp(false);
  try {
    await api("/api/admin/me");
    showApp(true);
    await refreshAll();
  } catch {
    showApp(false);
  }
}

document.getElementById("login-form").onsubmit = async (ev) => {
  ev.preventDefault();
  const err = document.getElementById("login-err");
  const btn = document.getElementById("login-btn");
  err.hidden = true;
  btn.disabled = true;
  try {
    await api("/api/admin/login", {
      method: "POST",
      body: JSON.stringify({ token: document.getElementById("admin-token").value.trim() }),
    });
    document.getElementById("admin-token").value = "";
    showApp(true);
    setTab("tenants");
    await refreshAll();
  } catch (e) {
    err.hidden = false;
    err.textContent = e.message || "Sign in failed";
  } finally {
    btn.disabled = false;
  }
};

document.getElementById("btn-logout").onclick = async () => {
  try {
    await api("/api/admin/logout", { method: "POST" });
  } catch {
    /* still clear UI */
  }
  showApp(false);
};

document.querySelector(".sidebar-nav").onclick = (ev) => {
  const btn = ev.target.closest("[data-tab]");
  if (!btn) return;
  setTab(btn.dataset.tab);
};

document.getElementById("btn-refresh").onclick = () => refreshAll().catch((e) => toast(e.message));

document.getElementById("tenant-form").onsubmit = async (ev) => {
  ev.preventDefault();
  const name = document.getElementById("tenant-name").value.trim();
  if (!name) return;
  try {
    const t = await api("/api/admin/tenants", {
      method: "POST",
      body: JSON.stringify({ name }),
    });
    document.getElementById("tenant-name").value = "";
    selectedTenant = t.id;
    toast(`Tenant “${t.name}” created`);
    await refreshAll();
  } catch (e) {
    toast(e.message);
  }
};

document.getElementById("account-tenant").onchange = async (ev) => {
  selectedTenant = ev.target.value || null;
  document.getElementById("issued").hidden = true;
  syncTenantSelects();
  await loadAccounts();
  await loadEnrollments();
};

document.getElementById("enroll-tenant").onchange = async (ev) => {
  selectedTenant = ev.target.value || null;
  document.getElementById("issued-enroll").hidden = true;
  syncTenantSelects();
  await loadAccounts();
  await loadEnrollments();
};

document.getElementById("issue-form").onsubmit = async (ev) => {
  ev.preventDefault();
  if (!selectedTenant) {
    toast("Select a tenant first");
    return;
  }
  try {
    const label = document.getElementById("issue-label").value.trim();
    const body = await api(`/api/admin/tenants/${selectedTenant}/access-accounts`, {
      method: "POST",
      body: JSON.stringify({ label }),
    });
    issuedCode = body.accessCode;
    document.getElementById("issued").hidden = false;
    document.getElementById("issued-code").textContent = issuedCode;
    document.getElementById("issue-label").value = "";
    toast("Access code issued — copy it now");
    await loadAccounts();
  } catch (e) {
    toast(e.message);
  }
};

document.getElementById("enroll-form").onsubmit = async (ev) => {
  ev.preventDefault();
  if (!selectedTenant) {
    toast("Select a tenant first");
    return;
  }
  try {
    const label = document.getElementById("enroll-label").value.trim();
    const ttlHours = Number(document.getElementById("enroll-ttl").value) || 168;
    const body = await api(`/api/admin/tenants/${selectedTenant}/enrollments`, {
      method: "POST",
      body: JSON.stringify({ label, ttlHours }),
    });
    issuedEnrollCode = body.enrollmentCode;
    const hint =
      body.agentHint ||
      `connect-agent.exe -server wss://${location.host}/ws -enroll ${issuedEnrollCode}`;
    document.getElementById("issued-enroll").hidden = false;
    document.getElementById("issued-enroll-code").textContent = issuedEnrollCode;
    document.getElementById("issued-enroll-hint").textContent = hint.replace("HOST", location.host);
    document.getElementById("issued-enroll-ttl").textContent =
      `One-time use · expires ${fmtTime(body.expiresAt)} · not shown again.`;
    document.getElementById("enroll-label").value = "";
    toast("Enrollment code issued — copy it now");
    await loadEnrollments();
  } catch (e) {
    toast(e.message);
  }
};

document.getElementById("copy-code").onclick = async () => {
  if (!issuedCode) return;
  try {
    await navigator.clipboard.writeText(issuedCode);
    toast("Copied");
  } catch {
    toast(issuedCode);
  }
};

document.getElementById("copy-enroll").onclick = async () => {
  if (!issuedEnrollCode) return;
  try {
    await navigator.clipboard.writeText(issuedEnrollCode);
    toast("Copied");
  } catch {
    toast(issuedEnrollCode);
  }
};

async function refreshAll() {
  tenantsCache = (await api("/api/admin/tenants")) || [];
  renderTenants();
  fillTenantSelects();
  await loadAccounts();
  await loadEnrollments();
  await loadAgents();
}

function renderTenants() {
  const body = document.getElementById("tenant-body");
  if (!tenantsCache.length) {
    body.innerHTML = '<tr><td colspan="4" class="empty">No tenants yet — create one above</td></tr>';
    return;
  }
  if (!selectedTenant || !tenantsCache.some((t) => t.id === selectedTenant)) {
    selectedTenant = tenantsCache[0].id;
  }
  body.innerHTML = tenantsCache
    .map((t) => {
      const sel = t.id === selectedTenant ? " selected" : "";
      return `<tr class="${sel}" data-id="${escapeHtml(t.id)}">
        <td><strong>${escapeHtml(t.name)}</strong></td>
        <td class="mono" title="${escapeHtml(t.id)}">${escapeHtml(t.id)}</td>
        <td>${escapeHtml(t.status)}</td>
        <td class="hint">${escapeHtml(fmtTime(t.createdAt))}</td>
      </tr>`;
    })
    .join("");
  body.onclick = (ev) => {
    const row = ev.target.closest("[data-id]");
    if (!row) return;
    selectedTenant = row.dataset.id;
    document.getElementById("issued").hidden = true;
    document.getElementById("issued-enroll").hidden = true;
    renderTenants();
    fillTenantSelects();
    loadAccounts();
    loadEnrollments();
  };
}

function fillTenantSelects() {
  const opts = tenantsCache.length
    ? tenantsCache.map((t) => `<option value="${escapeHtml(t.id)}">${escapeHtml(t.name)}</option>`).join("")
    : '<option value="">No tenants</option>';
  for (const id of ["account-tenant", "enroll-tenant"]) {
    const sel = document.getElementById(id);
    sel.innerHTML = opts;
    if (selectedTenant) sel.value = selectedTenant;
  }
}

function syncTenantSelects() {
  for (const id of ["account-tenant", "enroll-tenant"]) {
    const sel = document.getElementById(id);
    if (selectedTenant) sel.value = selectedTenant;
  }
}

async function loadAccounts() {
  const body = document.getElementById("account-body");
  if (!selectedTenant) {
    body.innerHTML = '<tr><td colspan="4" class="empty">Create a tenant first</td></tr>';
    return;
  }
  try {
    const list = (await api(`/api/admin/tenants/${selectedTenant}/access-accounts`)) || [];
    body.innerHTML = list.length
      ? list
          .map(
            (a) => `<tr>
          <td>${escapeHtml(a.label || "—")}</td>
          <td>${escapeHtml(a.status)}</td>
          <td class="hint">${escapeHtml(fmtTime(a.createdAt))}</td>
          <td>${
            a.status === "revoked"
              ? ""
              : `<button type="button" class="btn-danger" data-revoke="${escapeHtml(a.id)}">Revoke</button>`
          }</td>
        </tr>`
          )
          .join("")
      : '<tr><td colspan="4" class="empty">No access codes for this tenant</td></tr>';
    body.onclick = async (ev) => {
      const btn = ev.target.closest("[data-revoke]");
      if (!btn) return;
      try {
        await api(`/api/admin/access-accounts/${btn.dataset.revoke}/revoke`, { method: "POST" });
        toast("Access code revoked");
        await loadAccounts();
      } catch (e) {
        toast(e.message);
      }
    };
  } catch (e) {
    body.innerHTML = `<tr><td colspan="4" class="empty">${escapeHtml(e.message)}</td></tr>`;
  }
}

async function loadEnrollments() {
  const body = document.getElementById("enroll-body");
  if (!selectedTenant) {
    body.innerHTML = '<tr><td colspan="5" class="empty">Create a tenant first</td></tr>';
    return;
  }
  try {
    const list = (await api(`/api/admin/tenants/${selectedTenant}/enrollments`)) || [];
    body.innerHTML = list.length
      ? list
          .map(
            (e) => `<tr>
          <td>${escapeHtml(e.label || "—")}</td>
          <td>${escapeHtml(e.status)}</td>
          <td class="mono">${escapeHtml(e.deviceId || "—")}</td>
          <td class="hint">${escapeHtml(fmtTime(e.createdAt))}${
              e.expiresAt ? `<br><span title="Expires">→ ${escapeHtml(fmtTime(e.expiresAt))}</span>` : ""
            }</td>
          <td>${
            e.status === "pending"
              ? `<button type="button" class="btn-danger" data-revoke-enroll="${escapeHtml(e.id)}">Revoke</button>`
              : ""
          }</td>
        </tr>`
          )
          .join("")
      : '<tr><td colspan="5" class="empty">No enrollment codes for this tenant</td></tr>';
    body.onclick = async (ev) => {
      const btn = ev.target.closest("[data-revoke-enroll]");
      if (!btn) return;
      try {
        await api(`/api/admin/enrollments/${btn.dataset.revokeEnroll}/revoke`, { method: "POST" });
        toast("Enrollment revoked");
        await loadEnrollments();
      } catch (err) {
        toast(err.message);
      }
    };
  } catch (e) {
    body.innerHTML = `<tr><td colspan="5" class="empty">${escapeHtml(e.message)}</td></tr>`;
  }
}

async function loadAgents() {
  const body = document.getElementById("agent-body");
  const byTen = Object.fromEntries(tenantsCache.map((t) => [t.id, t.name]));
  try {
    const agents = (await api("/api/admin/agents")) || [];
    body.innerHTML = agents.length
      ? agents
          .map(
            (a) => `<tr>
          <td><span class="badge"><span class="dot${a.online ? " on" : ""}"></span>${a.online ? "Online" : "Offline"}</span></td>
          <td>${escapeHtml(a.hostname || "—")}</td>
          <td>${escapeHtml(byTen[a.tenantId] || a.tenantId || "—")}</td>
          <td class="mono" title="${escapeHtml(a.deviceId || "")}">${escapeHtml(a.deviceId || "—")}</td>
          <td class="hint">${escapeHtml(a.lastSeen ? fmtTime(a.lastSeen) : "—")}</td>
        </tr>`
          )
          .join("")
      : '<tr><td colspan="5" class="empty">No agents registered</td></tr>';
  } catch (e) {
    body.innerHTML = `<tr><td colspan="5" class="empty">${escapeHtml(e.message)}</td></tr>`;
  }
}

function fmtTime(iso) {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleString();
}

boot();
