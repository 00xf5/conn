window.ConnectListen = {
  _listens: new Map(), // deviceId -> { pc, ws, audio, code, unmuted }
  _ice: null,

  async iceServers() {
    if (this._ice) return this._ice;
    try {
      const r = await fetch('/api/ice', { credentials: 'same-origin' });
      if (r.ok) {
        const j = await r.json();
        if (j.iceServers && j.iceServers.length) {
          this._ice = j.iceServers;
          return this._ice;
        }
      }
    } catch (_) {}
    this._ice = [{ urls: 'stun:stun.l.google.com:19302' }];
    return this._ice;
  },

  isUnmuted(deviceId) {
    return !!this._listens.get(deviceId)?.unmuted;
  },

  async toggle(deviceId) {
    const cur = this._listens.get(deviceId);
    if (cur && cur.unmuted) {
      await this.mute(deviceId);
      return false;
    }
    await this.unmute(deviceId);
    return true;
  },

  async mute(deviceId) {
    const cur = this._listens.get(deviceId);
    if (!cur) return;
    cur.unmuted = false;
    if (cur.audio) cur.audio.muted = true;
    this._teardown(deviceId);
  },

  async unmute(deviceId) {
    let cur = this._listens.get(deviceId);
    if (cur && cur.pc && cur.audio) {
      cur.unmuted = true;
      cur.audio.muted = false;
      cur.audio.play().catch(() => {});
      return;
    }
    this._teardown(deviceId);

    const body = await fetch('/api/session', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ deviceId, mode: 'audio' }),
    }).then(async (r) => {
      if (!r.ok) throw new Error(await r.text());
      return r.json();
    });

    const code = body.code;
    const audio = document.createElement('audio');
    audio.autoplay = true;
    audio.playsInline = true;
    audio.muted = false;
    audio.style.display = 'none';
    document.body.appendChild(audio);

    const iceServers = await this.iceServers();
    const wsProto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(`${wsProto}//${location.host}/ws?role=viewer&session=${encodeURIComponent(code)}`);

    let pc = null;
    const entry = { pc: null, ws, audio, code, unmuted: true, pendingICE: [] };
    this._listens.set(deviceId, entry);

    const flushICE = async () => {
      if (!pc || !pc.remoteDescription) return;
      const q = entry.pendingICE;
      entry.pendingICE = [];
      for (const c of q) {
        try { await pc.addIceCandidate(c); } catch (_) {}
      }
    };

    ws.onmessage = async (ev) => {
      const msg = JSON.parse(ev.data);
      if (msg.type === 'offer') {
        if (pc) pc.close();
        pc = new RTCPeerConnection({
          iceServers,
          bundlePolicy: 'max-bundle',
          rtcpMuxPolicy: 'require',
        });
        entry.pc = pc;
        pc.ontrack = (e) => {
          if (e.track.kind !== 'audio') return;
          const stream = e.streams[0] || new MediaStream([e.track]);
          audio.srcObject = stream;
          audio.muted = !entry.unmuted;
          audio.play().catch(() => {});
        };
        pc.onicecandidate = (e) => {
          if (e.candidate && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type: 'ice', session: code, payload: e.candidate.toJSON() }));
          }
        };
        await pc.setRemoteDescription(msg.payload);
        const answer = await pc.createAnswer();
        await pc.setLocalDescription(answer);
        ws.send(JSON.stringify({ type: 'answer', session: code, payload: answer }));
        await flushICE();
      } else if (msg.type === 'ice') {
        if (!pc || !pc.remoteDescription) entry.pendingICE.push(msg.payload);
        else {
          try { await pc.addIceCandidate(msg.payload); } catch (_) {}
        }
      } else if (msg.type === 'no-host') {
        this._teardown(deviceId);
        throw new Error('host offline');
      }
    };

    await new Promise((resolve, reject) => {
      const t = setTimeout(() => reject(new Error('listen timeout')), 20000);
      ws.onopen = () => { clearTimeout(t); resolve(); };
      ws.onerror = () => { clearTimeout(t); reject(new Error('signaling failed')); };
    });
  },

  _teardown(deviceId) {
    const cur = this._listens.get(deviceId);
    if (!cur) return;
    this._listens.delete(deviceId);
    try { if (cur.pc) cur.pc.close(); } catch (_) {}
    try { if (cur.ws) cur.ws.close(); } catch (_) {}
    try {
      if (cur.audio) {
        cur.audio.pause();
        cur.audio.srcObject = null;
        cur.audio.remove();
      }
    } catch (_) {}
    if (cur.code) {
      fetch(`/api/session?code=${encodeURIComponent(cur.code)}`, {
        method: 'DELETE',
        credentials: 'same-origin',
      }).catch(() => {});
    }
  },

  stopAll() {
    for (const id of [...this._listens.keys()]) {
      this._teardown(id);
    }
  },

  hasActiveListen() {
    for (const v of this._listens.values()) {
      if (v.unmuted) return true;
    }
    return false;
  },
};
