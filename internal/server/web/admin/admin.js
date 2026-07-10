let selectedTenant = null;
let issuedCode = "";

async function api(path, opts = {}) {
  const res = await fetch(path, {
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...(opts.headers || {}) },
    ...opts,
  });
  if (!res.ok) throw new Error((await res.text()) || res.statusText);
  if (res.status === 204) return null;
  return res.json();
}

function toast(msg) {
  const el = document.getElementById("toast");
  el.hidden = false;
  el.textContent = msg;
  clearTimeout(toast._t);
  toast._t = setTimeout(() => { el.hidden = true; }, 2800);
}

function showApp(on) {
  document.getElementById("login").hidden = on;
  document.getElementById("app").hidden = !on;
}

async function boot() {
  try {
    await api("/api/admin/me");
    showApp(true);
    await refresh();
  } catch {
    showApp(false);
  }
}

document.getElementById("login-form").onsubmit = async (ev) => {
  ev.preventDefault();
  const err = document.getElementById("login-err");
  err.hidden = true;
  try {
    await api("/api/admin/login", {
      method: "POST",
      body: JSON.stringify({ token: document.getElementById("admin-token").value }),
    });
    showApp(true);
    await refresh();
  } catch (e) {
    err.hidden = false;
    err.textContent = e.message;
  }
};

document.getElementById("btn-logout").onclick = async () => {
  await api("/api/admin/logout", { method: "POST" });
  showApp(false);
};

document.getElementById("tenant-form").onsubmit = async (ev) => {
  ev.preventDefault();
  const name = document.getElementById("tenant-name").value.trim();
  const t = await api("/api/admin/tenants", { method: "POST", body: JSON.stringify({ name }) });
  document.getElementById("tenant-name").value = "";
  selectedTenant = t.id;
  toast("Tenant created");
  await refresh();
};

document.getElementById("issue-form").onsubmit = async (ev) => {
  ev.preventDefault();
  if (!selectedTenant) return;
  const label = document.getElementById("issue-label").value.trim();
  const body = await api(`/api/admin/tenants/${selectedTenant}/access-accounts`, {
    method: "POST",
    body: JSON.stringify({ label }),
  });
  issuedCode = body.accessCode;
  document.getElementById("issued").hidden = false;
  document.getElementById("issued-code").textContent = issuedCode;
  document.getElementById("issue-label").value = "";
  toast("Access code issued");
  await loadAccounts();
};

document.getElementById("copy-code").onclick = async () => {
  if (!issuedCode) return;
  await navigator.clipboard.writeText(issuedCode);
  toast("Copied");
};

async function refresh() {
  const tenants = await api("/api/admin/tenants");
  const ul = document.getElementById("tenant-list");
  if (!tenants.length) {
    ul.innerHTML = '<li class="muted">No tenants yet</li>';
    selectedTenant = null;
  } else {
    if (!selectedTenant || !tenants.some((t) => t.id === selectedTenant)) {
      selectedTenant = tenants[0].id;
    }
    ul.innerHTML = tenants
      .map((t) => `<li data-id="${t.id}" class="${t.id === selectedTenant ? "active" : ""}"><strong>${escapeHtml(t.name)}</strong><div class="muted mono">${escapeHtml(t.id.slice(0, 8))}…</div></li>`)
      .join("");
  }
  ul.onclick = (ev) => {
    const li = ev.target.closest("[data-id]");
    if (!li) return;
    selectedTenant = li.dataset.id;
    document.getElementById("issued").hidden = true;
    refresh();
  };
  document.getElementById("issue-form").hidden = !selectedTenant;
  document.getElementById("tenant-label").textContent = selectedTenant
    ? tenants.find((t) => t.id === selectedTenant)?.name || selectedTenant
    : "Select a tenant";
  await loadAccounts();
  await loadAgents(tenants);
}

async function loadAccounts() {
  const body = document.getElementById("account-body");
  if (!selectedTenant) {
    body.innerHTML = "";
    return;
  }
  const list = await api(`/api/admin/tenants/${selectedTenant}/access-accounts`);
  body.innerHTML = list.length
    ? list
        .map(
          (a) => `<tr>
        <td>${escapeHtml(a.label || "—")}</td>
        <td>${escapeHtml(a.status)}</td>
        <td class="muted">${escapeHtml(new Date(a.createdAt).toLocaleString())}</td>
        <td>${a.status === "revoked" ? "" : `<button type="button" class="ghost" data-revoke="${a.id}">Revoke</button>`}</td>
      </tr>`
        )
        .join("")
    : '<tr><td colspan="4" class="muted">No access accounts</td></tr>';
  body.onclick = async (ev) => {
    const btn = ev.target.closest("[data-revoke]");
    if (!btn) return;
    await api(`/api/admin/access-accounts/${btn.dataset.revoke}/revoke`, { method: "POST" });
    toast("Revoked");
    await loadAccounts();
  };
}

async function loadAgents(tenants) {
  const byTen = Object.fromEntries((tenants || []).map((t) => [t.id, t.name]));
  const agents = await api("/api/admin/agents");
  const body = document.getElementById("agent-body");
  body.innerHTML = agents.length
    ? agents
        .map(
          (a) => `<tr>
        <td><span class="dot${a.online ? " on" : ""}"></span> ${a.online ? "Online" : "Offline"}</td>
        <td>${escapeHtml(a.hostname || "—")}</td>
        <td>${escapeHtml(byTen[a.tenantId] || a.tenantId || "—")}</td>
        <td class="mono">${escapeHtml((a.deviceId || "").slice(0, 8))}…</td>
      </tr>`
        )
        .join("")
    : '<tr><td colspan="4" class="muted">No agents</td></tr>';
}

function escapeHtml(s) {
  return String(s ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

boot();
