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

function agentOnline(a) {
  return !!(a && a.online !== false && a.online !== 0);
}

function filteredAgents() {
  let list = agents.slice();
  if (group === "online") {
    list = list.filter((a) => agentOnline(a));
  }
  const q = filterText.trim().toLowerCase();
  if (q) {
    list = list.filter((a) => {
      const hay = `${a.hostname || ""} ${a.deviceId || ""} ${a.encoder || ""}`.toLowerCase();
      return hay.includes(q);
    });
  }
  return list.sort((a, b) => {
    const ao = agentOnline(a) ? 0 : 1;
    const bo = agentOnline(b) ? 0 : 1;
    if (ao !== bo) return ao - bo;
    return String(a.hostname || a.deviceId).localeCompare(String(b.hostname || b.deviceId));
  });
}

function vuBars(level) {
  const n = Math.max(0, Math.min(1, Number(level) || 0));
  const hot = n > 0.04 ? " hot" : "";
  const h1 = Math.max(2, Math.round(4 + n * 8));
  const h2 = Math.max(2, Math.round(4 + n * 12));
  const h3 = Math.max(2, Math.round(4 + n * 10));
  const h4 = Math.max(2, Math.round(4 + n * 14));
  return `<span class="vu${hot}" aria-hidden="true"><span style="height:${h1}px"></span><span style="height:${h2}px"></span><span style="height:${h3}px"></span><span style="height:${h4}px"></span></span>`;
}

function renderList() {
  const list = document.getElementById("session-list");
  const empty = document.getElementById("list-empty");
  const rows = filteredAgents();

  document.getElementById("count-all").textContent = String(agents.length);
  document.getElementById("count-online").textContent = String(
    agents.filter((a) => agentOnline(a)).length
  );
  document.getElementById("count-sessions").textContent = String(sessions.length);

  if (group === "sessions") {
    // Show session tickets themselves (not agent rows filtered by ticket).
    list.innerHTML = sessions
      .map((s) => {
        const host = agentById(s.deviceId)?.hostname || shortId(s.deviceId);
        const selected = selectedId === s.deviceId ? " selected" : "";
        const on = agentOnline(agentById(s.deviceId));
        return `<div class="session-row${selected}" role="option" data-device="${escapeHtml(s.deviceId)}" data-session="${escapeHtml(s.code)}" tabindex="0">
          <span><span class="dot${on ? " on" : ""}" title="${on ? "Online" : "Offline"}"></span></span>
          <span class="listen-cell"></span>
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
      : "No machines enrolled yet — use Add machine.";
    return;
  }
  empty.hidden = true;
  list.innerHTML = rows
    .map((a) => {
      const selected = selectedId === a.deviceId ? " selected" : "";
      const on = agentOnline(a);
      const listening = on && window.ConnectListen && ConnectListen.isUnmuted(a.deviceId);
      const listenCls = listening ? " on" : "";
      const listenTitle = !on
        ? "Host offline"
        : listening
          ? "Mute host audio"
          : "Listen to host mic (muted by default)";
      const listenLabel = listening ? "🔊" : "🔇";
      const listenBtn = on
        ? `<button type="button" class="btn-listen${listenCls}" data-listen="${escapeHtml(a.deviceId)}" title="${listenTitle}" aria-pressed="${listening ? "true" : "false"}">${listenLabel}</button>`
        : "";
      return `<div class="session-row${selected}${on ? "" : " offline"}" role="option" data-device="${escapeHtml(a.deviceId)}" tabindex="0">
        <span><span class="dot${on ? " on" : ""}" title="${on ? "Online" : "Offline"}"></span></span>
        <span class="listen-cell">
          ${on ? vuBars(a.audioLevel) : ""}
          ${listenBtn}
        </span>
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
    const on = agentOnline(a);
    document.getElementById("detail-name").textContent = a.hostname || "host";
    document.getElementById("detail-status").innerHTML = on
      ? '<span class="dot on"></span> Online'
      : '<span class="dot"></span> Offline';
    document.getElementById("detail-guest").textContent = a.hostname || "—";
    document.getElementById("detail-device").textContent = a.deviceId;
    const keyEl = document.getElementById("detail-host-key");
    const keyActions = document.getElementById("detail-key-actions");
    if (a.hostKey) {
      keyEl.textContent = a.hostKey;
      keyActions.hidden = false;
    } else {
      keyEl.textContent = "—";
      keyActions.hidden = true;
    }
    document.getElementById("detail-seen").textContent = fmtTime(a.lastSeen);
    document.getElementById("detail-pipe").textContent =
      [a.encoder, a.resolution].filter(Boolean).join(" · ") || "—";
    document.getElementById("detail-conn").textContent = on ? fmtTime(a.connected) : "—";
    document.getElementById("btn-join").disabled = !on || joining;
    document.getElementById("btn-join").textContent = joining ? "Joining…" : "Join";
    document.getElementById("btn-share").disabled = !on;
    document.getElementById("detail-note").textContent = on
      ? "Join opens a remote session in this browser. Share link creates a ticket for another device. Give the Host key to the host person so they can open the WorthyJoin Host app."
      : "This machine is offline. It stays listed so you can find it when it reconnects. Host key still works for the local Host app.";
  } else {
    const sess = sessions.find((s) => s.deviceId === selectedId);
    document.getElementById("detail-name").textContent = sess ? sess.code : "Session";
    document.getElementById("detail-status").textContent = "Ticket";
    document.getElementById("detail-guest").textContent = shortId(selectedId);
    document.getElementById("detail-device").textContent = selectedId || "—";
    document.getElementById("detail-host-key").textContent = "—";
    document.getElementById("detail-key-actions").hidden = true;
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

function setDetailOpen(open) {
  const body = document.getElementById("host-body");
  if (body) body.classList.toggle("show-detail", !!open);
}

function selectDevice(id) {
  selectedId = id;
  setDetailOpen(!!id);
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
  if (!a || !agentOnline(a)) {
    toast("Machine offline");
    return;
  }

  if (window.ConnectListen) {
    try { await ConnectListen.mute(selectedId); } catch (_) {}
  }

  joining = true;
  renderDetail();
  try {
    const body = await api("/api/session", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ deviceId: selectedId, mode: "full" }),
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
  if (!agentById(selectedId) || !agentOnline(agentById(selectedId))) {
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
    // Keep selection when a machine goes offline (still in the merged list).
    if (selectedId && !agentById(selectedId) && group !== "sessions") {
      selectedId = agents[0]?.deviceId || null;
    }
    if (!selectedId && agents[0]) selectedId = agents[0].deviceId;
    const onlineN = agents.filter((x) => agentOnline(x)).length;
    document.getElementById("health").textContent =
      `${onlineN} online / ${agents.length} · ${(health.publicKey || "").slice(0, 10)}…`;
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

function scheduleRefresh() {
  if (refreshTimer) clearInterval(refreshTimer);
  const ms = window.ConnectListen && ConnectListen.hasActiveListen() ? 1000 : 5000;
  refreshTimer = setInterval(refresh, ms);
}

async function boot() {
  try {
    me = await api("/api/me");
    document.getElementById("tenant-chip").textContent = me.tenantName || me.tenantId;
    showHost(true);
    await refresh();
    scheduleRefresh();
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
    scheduleRefresh();
  } catch (e) {
    err.hidden = false;
    err.textContent = e.message;
  }
};

document.getElementById("btn-logout").onclick = async () => {
  if (window.ConnectListen) ConnectListen.stopAll();
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

document.getElementById("session-list").onclick = async (ev) => {
  const listenBtn = ev.target.closest("[data-listen]");
  if (listenBtn) {
    ev.preventDefault();
    ev.stopPropagation();
    const id = listenBtn.dataset.listen;
    try {
      await ConnectListen.toggle(id);
      scheduleRefresh();
      renderList();
    } catch (err) {
      toast(`Listen failed: ${err.message}`);
      renderList();
    }
    return;
  }
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
document.getElementById("btn-detail-back").onclick = () => {
  selectedId = null;
  setDetailOpen(false);
  renderList();
  renderDetail();
};

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

let lastEnrollCode = "";
let lastEnrollLink = "";

function enrollLink(code) {
  return `${location.origin}/install?code=${encodeURIComponent(code)}`;
}

function showEnrollModal(on) {
  document.getElementById("enroll-modal").hidden = !on;
  if (on) {
    document.getElementById("enroll-result").hidden = true;
    document.getElementById("enroll-label").value = "";
    loadEnrollments().catch((e) => toast(e.message));
  }
}

async function loadEnrollments() {
  const el = document.getElementById("enroll-list");
  try {
    const list = (await api("/api/enrollments")) || [];
    // Only open (pending) codes — used/revoked drop off. Cap at 4 newest.
    const recent = list.filter((e) => e.status === "pending").slice(0, 4);
    if (!recent.length) {
      el.innerHTML = '<div class="enroll-empty">No open enrollment codes</div>';
      return;
    }
    el.innerHTML = recent
      .map((e) => {
        const revoke = `<button type="button" class="link-btn" data-revoke-enroll="${escapeHtml(e.id)}">Revoke</button>`;
        const codeBtn = e.code
          ? `<button type="button" class="link-btn" data-copy-enroll-code="${escapeHtml(e.code)}">Copy code</button>`
          : "";
        const linkBtn = e.code
          ? `<button type="button" class="link-btn" data-copy-enroll-link="${escapeHtml(enrollLink(e.code))}">Copy link</button>`
          : "";
        return `<div class="enroll-row">
          <span>${escapeHtml(e.label || "—")}${e.code ? `<br><code class="mono">${escapeHtml(e.code)}</code>` : ""}</span>
          <span class="mono">open</span>
          <span>${codeBtn} ${linkBtn} ${revoke}</span>
        </div>`;
      })
      .join("");
  } catch (err) {
    el.innerHTML = `<div class="enroll-empty">${escapeHtml(err.message)}</div>`;
  }
}

document.getElementById("btn-add-machine").onclick = () => showEnrollModal(true);
document.getElementById("enroll-close").onclick = () => showEnrollModal(false);
document.getElementById("enroll-modal").onclick = (ev) => {
  if (ev.target.id === "enroll-modal") showEnrollModal(false);
};

document.getElementById("enroll-issue-form").onsubmit = async (ev) => {
  ev.preventDefault();
  const btn = document.getElementById("enroll-issue-btn");
  btn.disabled = true;
  try {
    const label = document.getElementById("enroll-label").value.trim();
    const body = await api("/api/enrollments", {
      method: "POST",
      body: JSON.stringify({ label }),
    });
    lastEnrollCode = body.enrollmentCode;
    lastEnrollLink = body.installUrl || enrollLink(lastEnrollCode);
    document.getElementById("enroll-code").textContent = lastEnrollCode;
    document.getElementById("enroll-link").textContent = lastEnrollLink;
    document.getElementById("enroll-ttl-note").textContent =
      "Stays valid until the host installs or you revoke it. Host opens link → Download WorthyJoin → paste code → Install.";
    document.getElementById("enroll-pkg-warn").hidden = body.packageReady !== false;
    document.getElementById("enroll-result").hidden = false;
    toast("Install link ready — send it to the host");
    await loadEnrollments();
  } catch (e) {
    toast(e.message);
  } finally {
    btn.disabled = false;
  }
};

document.getElementById("enroll-copy-code").onclick = () => {
  if (lastEnrollCode) copyText(lastEnrollCode);
};
document.getElementById("enroll-copy-link").onclick = () => {
  if (lastEnrollLink) copyText(lastEnrollLink);
};

document.getElementById("enroll-list").onclick = async (ev) => {
  const copyCode = ev.target.closest("[data-copy-enroll-code]");
  if (copyCode) {
    await copyText(copyCode.dataset.copyEnrollCode || "");
    return;
  }
  const copyLink = ev.target.closest("[data-copy-enroll-link]");
  if (copyLink) {
    await copyText(copyLink.dataset.copyEnrollLink || "");
    return;
  }
  const btn = ev.target.closest("[data-revoke-enroll]");
  if (!btn) return;
  try {
    await api(`/api/enrollments/${btn.dataset.revokeEnroll}/revoke`, { method: "POST" });
    toast("Enrollment revoked");
    await loadEnrollments();
  } catch (e) {
    toast(e.message);
  }
};

document.getElementById("btn-copy-host-key").onclick = async () => {
  const a = agentById(selectedId);
  if (!a?.hostKey) {
    toast("No host key yet");
    return;
  }
  await copyText(a.hostKey);
  toast("Host key copied");
};

document.getElementById("btn-rotate-host-key").onclick = async () => {
  if (!selectedId) return;
  if (!confirm("Rotate host key? The host person will need the new key to open the Host app.")) return;
  try {
    const res = await api(`/api/agents/${encodeURIComponent(selectedId)}/host-key/rotate`, {
      method: "POST",
    });
    const a = agentById(selectedId);
    if (a && res.hostKey) a.hostKey = res.hostKey;
    renderDetail();
    await copyText(res.hostKey || "");
    toast("New host key copied — ask the host to Lock and unlock with this key");
    refresh().catch(() => {});
  } catch (e) {
    toast(e.message);
  }
};

boot();
