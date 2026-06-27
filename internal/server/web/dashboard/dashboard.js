let selectedDeviceId = null;

async function api(path, opts) {
  const res = await fetch(path, opts);
  return res.json();
}

async function refresh() {
  const agents = await api('/api/agents');
  const ul = document.getElementById('agents');
  if (!agents.length) {
    ul.innerHTML = '<li>No agents online</li>';
    selectedDeviceId = null;
  } else {
    ul.innerHTML = agents.map((a) => {
      const sel = a.deviceId === selectedDeviceId ? ' class="selected"' : '';
      return `<li${sel} data-device="${a.deviceId}"><strong>${a.hostname || 'host'}</strong> · ${a.deviceId.slice(0, 8)}…</li>`;
    }).join('');
    if (!selectedDeviceId || !agents.some((a) => a.deviceId === selectedDeviceId)) {
      selectedDeviceId = agents[0].deviceId;
      ul.querySelector(`[data-device="${selectedDeviceId}"]`)?.classList.add('selected');
    }
  }

  const sessions = await api('/api/sessions');
  document.getElementById('sessions').innerHTML = sessions.length
    ? sessions.map((s) => `<li>${s.code} → <a href="/v/${s.code}">viewer</a> (expires ${new Date(s.expiresAt).toLocaleTimeString()})</li>`).join('')
    : '<li>No active sessions</li>';

  const health = await api('/api/health');
  document.getElementById('health').textContent = `Server OK · ${health.agents} agent(s) · key ${health.publicKey.slice(0, 12)}…`;
}

document.getElementById('agents').onclick = (ev) => {
  const li = ev.target.closest('[data-device]');
  if (!li) return;
  selectedDeviceId = li.dataset.device;
  document.querySelectorAll('#agents li').forEach((el) => el.classList.remove('selected'));
  li.classList.add('selected');
};

document.getElementById('create-session').onclick = async () => {
  if (!selectedDeviceId) {
    document.getElementById('session-result').innerHTML = '<span class="err">No agent online — start connect-agent first.</span>';
    return;
  }
  const body = await api('/api/session', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ deviceId: selectedDeviceId }),
  });
  document.getElementById('session-result').innerHTML =
    `Code: <strong>${body.code}</strong><br/>Viewer: <a href="${body.viewer}">${body.viewer}</a>`;
  refresh();
};

refresh();
setInterval(refresh, 5000);
