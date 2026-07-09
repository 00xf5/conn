let currentView = "machines";
let connectingId = null;

async function api(path, opts) {
  const res = await fetch(path, opts);
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || res.statusText);
  }
  return res.json();
}

function toast(msg) {
  const el = document.getElementById("toast");
  el.hidden = false;
  el.textContent = msg;
  clearTimeout(toast._t);
  toast._t = setTimeout(() => {
    el.hidden = true;
  }, 3200);
}

async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text);
    toast("Copied");
  } catch {
    toast(text);
  }
}

function shortId(id) {
  if (!id) return "—";
  return id.length > 12 ? id.slice(0, 8) + "…" : id;
}

function fmtTime(iso) {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function setView(name) {
  currentView = name;
  document.querySelectorAll(".nav-item").forEach((btn) => {
    btn.classList.toggle("active", btn.dataset.view === name);
  });
  document.getElementById("panel-machines").classList.toggle("active", name === "machines");
  document.getElementById("panel-sessions").classList.toggle("active", name === "sessions");
  document.getElementById("view-title").textContent = name === "machines" ? "Machines" : "Sessions";
  document.getElementById("view-sub").textContent =
    name === "machines"
      ? "Online agents ready for remote access"
      : "Active remote session tickets";
}

async function connectDevice(deviceId) {
  if (!deviceId || connectingId) return;
  connectingId = deviceId;
  refresh();
  try {
    const body = await api("/api/session", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ deviceId }),
    });
    toast(`Session ${body.code} — opening viewer`);
    location.href = body.viewer || `/v/${body.code}`;
  } catch (err) {
    toast(`Connect failed: ${err.message}`);
    connectingId = null;
    refresh();
  }
}

function renderMachines(agents) {
  const tbody = document.getElementById("machines-body");
  const hint = document.getElementById("machines-hint");
  if (!agents.length) {
    tbody.innerHTML =
      '<tr class="empty-row"><td colspan="6">No agents online — start connect-agent on the host PC</td></tr>';
    hint.textContent = "";
    return;
  }
  tbody.innerHTML = agents
    .map((a) => {
      const busy = connectingId === a.deviceId;
      const pipeline = [a.encoder, a.resolution].filter(Boolean).join(" · ") || "—";
      return `<tr data-device="${a.deviceId}">
        <td><span class="status-cell"><span class="dot on"></span>Online</span></td>
        <td class="host-name">${escapeHtml(a.hostname || "host")}</td>
        <td class="mono" title="${escapeHtml(a.deviceId)}">${escapeHtml(shortId(a.deviceId))}</td>
        <td class="muted">${escapeHtml(fmtTime(a.lastSeen))}</td>
        <td class="muted">${escapeHtml(pipeline)}</td>
        <td>
          <div class="row-actions">
            <button type="button" class="btn primary" data-act="connect" ${busy ? "disabled" : ""}>
              ${busy ? "Connecting…" : "Connect"}
            </button>
            <button type="button" class="btn ghost sm" data-act="share">Share</button>
          </div>
        </td>
      </tr>`;
    })
    .join("");
  hint.textContent = `${agents.length} machine${agents.length === 1 ? "" : "s"} online · Connect opens a browser session`;
}

function renderSessions(sessions, agents) {
  const tbody = document.getElementById("sessions-body");
  if (!sessions.length) {
    tbody.innerHTML = '<tr class="empty-row"><td colspan="4">No active sessions</td></tr>';
    return;
  }
  const byId = Object.fromEntries(agents.map((a) => [a.deviceId, a]));
  tbody.innerHTML = sessions
    .map((s) => {
      const host = byId[s.deviceId]?.hostname || shortId(s.deviceId);
      const viewer = `/v/${s.code}`;
      return `<tr>
        <td class="mono">${escapeHtml(s.code)}</td>
        <td>${escapeHtml(host)}</td>
        <td class="muted">${escapeHtml(fmtTime(s.expiresAt))}</td>
        <td>
          <div class="row-actions">
            <a class="btn primary sm" href="${viewer}">Open</a>
            <button type="button" class="btn ghost sm" data-act="copy-link" data-url="${viewer}">Copy link</button>
            <button type="button" class="btn ghost sm" data-act="copy-code" data-code="${escapeHtml(s.code)}">Copy code</button>
          </div>
        </td>
      </tr>`;
    })
    .join("");
}

function escapeHtml(s) {
  return String(s ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

async function refresh() {
  try {
    const [agents, sessions, health] = await Promise.all([
      api("/api/agents"),
      api("/api/sessions"),
      api("/api/health"),
    ]);
    renderMachines(agents);
    renderSessions(sessions, agents);
    document.getElementById("health").textContent =
      `OK · ${health.agents} agent(s)\n${(health.publicKey || "").slice(0, 16)}…`;
  } catch (err) {
    document.getElementById("machines-body").innerHTML =
      `<tr class="empty-row"><td colspan="6">Server unreachable: ${escapeHtml(err.message)}</td></tr>`;
    document.getElementById("health").textContent = "Server offline";
  }
}

document.querySelector(".rail-nav").onclick = (ev) => {
  const btn = ev.target.closest("[data-view]");
  if (!btn) return;
  setView(btn.dataset.view);
};

document.getElementById("btn-refresh").onclick = () => refresh();

document.getElementById("machines-body").onclick = async (ev) => {
  const btn = ev.target.closest("[data-act]");
  const row = ev.target.closest("[data-device]");
  if (!btn || !row) return;
  const deviceId = row.dataset.device;
  if (btn.dataset.act === "connect") {
    connectDevice(deviceId);
    return;
  }
  if (btn.dataset.act === "share") {
    try {
      const body = await api("/api/session", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ deviceId }),
      });
      const url = body.viewer || `${location.origin}/v/${body.code}`;
      await copyText(url);
      toast(`Share link ready · ${body.code}`);
      refresh();
    } catch (err) {
      toast(`Share failed: ${err.message}`);
    }
  }
};

document.getElementById("sessions-body").onclick = async (ev) => {
  const btn = ev.target.closest("[data-act]");
  if (!btn) return;
  if (btn.dataset.act === "copy-link") {
    await copyText(new URL(btn.dataset.url, location.origin).href);
  } else if (btn.dataset.act === "copy-code") {
    await copyText(btn.dataset.code);
  }
};

setView("machines");
refresh();
setInterval(refresh, 5000);
