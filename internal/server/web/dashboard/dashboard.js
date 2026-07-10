let agents = [];
let sessions = [];
let selectedId = null;
let group = "all";
let filterText = "";
let joining = false;
let me = null;

async function api(path, opts = {}) {
  const res = await fetch(path, {
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...(opts.headers || {}) },
    ...opts,
  });
  if (!res.ok) throw new Error((await res.text()) || res.statusText);
  return res.json();
}

function showHost(on) {
  document.getElementById("login-gate").hidden = on;
  document.getElementById("host-app").hidden = !on;
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

async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text);
    toast("Copied to clipboard");
  } catch {
    toast(text);
  }
}

function escapeHtml(s) {
  return String(s ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function shortId(id) {
  if (!id) return "—";
  return id.length > 14 ? id.slice(0, 8) + "…" + id.slice(-4) : id;
}

function fmtTime(iso) {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleString([], {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function agentById(id) {
  return agents.find((a) => a.deviceId === id);
}

function filteredAgents() {
  let list = agents.slice();
  const q = filterText.trim().toLowerCase();
  if (q) {
    list = list.filter((a) => {
      const hay = `${a.hostname || ""} ${a.deviceId || ""} ${a.encoder || ""}`.toLowerCase();
      return hay.includes(q);
    });
  }
  return list.sort((a, b) =>
    String(a.hostname || a.deviceId).localeCompare(String(b.hostname || b.deviceId))
  );
}

function renderList() {
  const list = document.getElementById("session-list");
  const empty = document.getElementById("list-empty");
  const rows = filteredAgents();

  document.getElementById("count-all").textContent = String(agents.length);
  document.getElementById("count-online").textContent = String(agents.length);
  document.getElementById("count-sessions").textContent = String(sessions.length);

  if (group === "sessions") {
    // Show session tickets themselves (not agent rows filtered by ticket).
    list.innerHTML = sessions
      .map((s) => {
        const host = agentById(s.deviceId)?.hostname || shortId(s.deviceId);
        const selected = selectedId === s.deviceId ? " selected" : "";
        return `<div class="session-row${selected}" role="option" data-device="${escapeHtml(s.deviceId)}" data-session="${escapeHtml(s.code)}" tabindex="0">
          <span><span class="dot${agentById(s.deviceId) ? " on" : ""}"></span></span>
          <span class="name">${escapeHtml(host)}</span>
          <span class="guest mono">${escapeHtml(s.code)}</span>
          <span class="activity">${escapeHtml(fmtTime(s.expiresAt))}</span>
        </div>`;
      })
      .join("");
    empty.hidden = sessions.length > 0;
    if (!sessions.length) {
      empty.hidden = false;
      empty.textContent = "No active session tickets.";
    }
    return;
  }

  if (!rows.length) {
    list.innerHTML = "";
    empty.hidden = false;
    empty.textContent = agents.length
      ? "No machines match this filter."
      : "No agents online — start connect-agent on the host PC.";
    return;
  }
  empty.hidden = true;
  list.innerHTML = rows
    .map((a) => {
      const selected = selectedId === a.deviceId ? " selected" : "";
      return `<div class="session-row${selected}" role="option" data-device="${escapeHtml(a.deviceId)}" tabindex="0">
        <span><span class="dot on" title="Online"></span></span>
        <span class="name" title="${escapeHtml(a.hostname || "host")}">${escapeHtml(a.hostname || "host")}</span>
        <span class="guest">${escapeHtml(shortId(a.deviceId))}</span>
        <span class="activity">${escapeHtml(fmtTime(a.lastSeen))}</span>
      </div>`;
    })
    .join("");
}

function renderDetail() {
  const empty = document.getElementById("detail-empty");
  const body = document.getElementById("detail-body");
  const a = agentById(selectedId);

  if (!selectedId || (!a && group !== "sessions")) {
    empty.hidden = false;
    body.hidden = true;
    return;
  }

  empty.hidden = true;
  body.hidden = false;

  if (a) {
    document.getElementById("detail-name").textContent = a.hostname || "host";
    document.getElementById("detail-status").innerHTML =
      '<span class="dot on"></span> Online';
    document.getElementById("detail-guest").textContent = a.hostname || "—";
    document.getElementById("detail-device").textContent = a.deviceId;
    document.getElementById("detail-seen").textContent = fmtTime(a.lastSeen);
    document.getElementById("detail-pipe").textContent =
      [a.encoder, a.resolution].filter(Boolean).join(" · ") || "—";
    document.getElementById("detail-conn").textContent = fmtTime(a.connected);
    document.getElementById("btn-join").disabled = joining;
    document.getElementById("btn-join").textContent = joining ? "Joining…" : "Join";
    document.getElementById("detail-note").textContent =
      "Join opens a remote session in this browser. Share link creates a ticket for another device.";
  } else {
    const sess = sessions.find((s) => s.deviceId === selectedId);
    document.getElementById("detail-name").textContent = sess ? sess.code : "Session";
    document.getElementById("detail-status").textContent = "Ticket";
    document.getElementById("detail-guest").textContent = shortId(selectedId);
    document.getElementById("detail-device").textContent = selectedId || "—";
    document.getElementById("detail-seen").textContent = sess ? fmtTime(sess.expiresAt) : "—";
    document.getElementById("detail-pipe").textContent = "—";
    document.getElementById("detail-conn").textContent = sess ? fmtTime(sess.createdAt) : "—";
    document.getElementById("btn-join").disabled = !sess || joining;
    document.getElementById("btn-join").textContent = joining ? "Joining…" : "Open";
    document.getElementById("detail-note").textContent = sess
      ? `Session expires ${fmtTime(sess.expiresAt)}`
      : "";
  }
}

function selectDevice(id) {
  selectedId = id;
  renderList();
  renderDetail();
}

async function joinSelected() {
  if (!selectedId || joining) return;
  const a = agentById(selectedId);
  const existing = sessions.find((s) => s.deviceId === selectedId);

  if (!a && existing) {
    location.href = `/v/${existing.code}`;
    return;
  }
  if (!a) {
    toast("Machine offline");
    return;
  }

  joining = true;
  renderDetail();
  try {
    const body = await api("/api/session", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ deviceId: selectedId }),
    });
    toast(`Joining ${a.hostname || "host"} · ${body.code}`);
    location.href = body.viewer || `/v/${body.code}`;
  } catch (err) {
    toast(`Join failed: ${err.message}`);
    joining = false;
    renderDetail();
  }
}

async function shareSelected() {
  if (!selectedId) return;
  if (!agentById(selectedId)) {
    const sess = sessions.find((s) => s.deviceId === selectedId);
    if (sess) {
      await copyText(new URL(`/v/${sess.code}`, location.origin).href);
      return;
    }
    toast("Machine offline");
    return;
  }
  try {
    const body = await api("/api/session", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ deviceId: selectedId }),
    });
    const url = body.viewer || `${location.origin}/v/${body.code}`;
    await copyText(url);
    toast(`Share link · ${body.code}`);
    await refresh();
  } catch (err) {
    toast(`Share failed: ${err.message}`);
  }
}

async function refresh() {
  try {
    const [a, s, health] = await Promise.all([
      api("/api/agents"),
      api("/api/sessions"),
      api("/api/health"),
    ]);
    agents = a;
    sessions = s;
    if (selectedId && !agentById(selectedId) && group !== "sessions") {
      selectedId = agents[0]?.deviceId || null;
    }
    if (!selectedId && agents[0]) selectedId = agents[0].deviceId;
    document.getElementById("health").textContent =
      `${agents.length} online · ${(health.publicKey || "").slice(0, 10)}…`;
    renderList();
    renderDetail();
  } catch (err) {
    if (/unauthorized/i.test(err.message)) {
      showHost(false);
      return;
    }
    document.getElementById("health").textContent = "offline";
    document.getElementById("session-list").innerHTML = "";
    document.getElementById("list-empty").hidden = false;
    document.getElementById("list-empty").textContent = `Server unreachable: ${err.message}`;
  }
}

let refreshTimer = null;

async function boot() {
  try {
    me = await api("/api/me");
    document.getElementById("tenant-chip").textContent = me.tenantName || me.tenantId;
    showHost(true);
    await refresh();
    if (!refreshTimer) refreshTimer = setInterval(refresh, 5000);
  } catch {
    showHost(false);
  }
}

document.getElementById("redeem-form").onsubmit = async (ev) => {
  ev.preventDefault();
  const err = document.getElementById("redeem-err");
  err.hidden = true;
  try {
    me = await api("/api/auth/redeem", {
      method: "POST",
      body: JSON.stringify({ accessCode: document.getElementById("access-code").value }),
    });
    document.getElementById("tenant-chip").textContent = me.tenantName || me.tenantId;
    showHost(true);
    await refresh();
    if (!refreshTimer) refreshTimer = setInterval(refresh, 5000);
  } catch (e) {
    err.hidden = false;
    err.textContent = e.message;
  }
};

document.getElementById("btn-logout").onclick = async () => {
  await api("/api/auth/logout", { method: "POST" });
  me = null;
  showHost(false);
};

document.querySelector(".groups").onclick = (ev) => {
  const btn = ev.target.closest("[data-group]");
  if (!btn) return;
  group = btn.dataset.group;
  document.querySelectorAll(".group").forEach((g) => {
    g.classList.toggle("active", g.dataset.group === group);
  });
  renderList();
};

document.getElementById("filter").oninput = (ev) => {
  filterText = ev.target.value;
  renderList();
};

document.getElementById("session-list").onclick = (ev) => {
  const row = ev.target.closest("[data-device]");
  if (!row) return;
  selectDevice(row.dataset.device);
};

document.getElementById("session-list").ondblclick = (ev) => {
  const row = ev.target.closest("[data-device]");
  if (!row) return;
  selectDevice(row.dataset.device);
  joinSelected();
};

document.getElementById("btn-join").onclick = () => joinSelected();
document.getElementById("btn-share").onclick = () => shareSelected();
document.getElementById("btn-refresh").onclick = () => refresh();

document.querySelector(".mode-tabs").onclick = (ev) => {
  const tab = ev.target.closest("[data-mode]");
  if (!tab) return;
  if (tab.dataset.mode === "support") {
    toast("Support (guest code) — not in this build");
    return;
  }
  document.querySelectorAll(".mode-tab").forEach((t) => {
    t.classList.toggle("active", t === tab);
  });
};

boot();
