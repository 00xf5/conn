window.Connect = window.Connect || {};

Connect.webrtc = {
  create(ctx, hooks) {
    const { code, status, video, overlay } = ctx;
    const { layout, taskmgr, control } = hooks;

    const isMobile = /Android|iPhone|iPad|iPod|Mobile/i.test(navigator.userAgent);

    const wsProto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(`${wsProto}//${location.host}/ws?role=viewer&session=${encodeURIComponent(code)}`);

    let pc = null;
    let dc = null;
    let moveQueue = null;
    let moveTimer = null;
    let pendingICE = [];
    let receiverTuneTimer = null;

    function getDC() { return dc; }

    function sendBinary(buf) {
      if (dc && dc.readyState === 'open') dc.send(buf);
    }

    function flushMove() {
      moveTimer = null;
      if (moveQueue) {
        sendBinary(moveQueue);
        moveQueue = null;
      }
    }

    function queueMove(buf) {
      moveQueue = buf;
      if (!moveTimer) moveTimer = setTimeout(flushMove, 16);
    }

    function tuneVideoReceivers() {
      if (!pc) return;
      for (const rx of pc.getReceivers()) {
        if (!rx.track || rx.track.kind !== 'video') continue;
        if ('playoutDelayHint' in rx) rx.playoutDelayHint = 0;
        if ('jitterBufferTarget' in rx) rx.jitterBufferTarget = 0;
      }
    }

    async function addIceCandidateSafe(cand) {
      if (!pc || !pc.remoteDescription) {
        pendingICE.push(cand);
        return;
      }
      try {
        await pc.addIceCandidate(cand);
      } catch (e) {
        console.warn('ICE candidate failed', e);
      }
    }

    async function flushPendingICE() {
      if (!pc || !pc.remoteDescription) return;
      const queued = pendingICE;
      pendingICE = [];
      for (const cand of queued) {
        try {
          await pc.addIceCandidate(cand);
        } catch (e) {
          console.warn('queued ICE failed', e);
        }
      }
    }

    function setupInput(channel) {
      dc = channel;
      overlay.focus();
      Connect.input.bindOverlay(overlay, video, () => layout.getStreamSize(), sendBinary, queueMove, () => layout.isCover());

      channel.onmessage = (ev) => {
        if (typeof ev.data !== 'string') return;
        try {
          const m = JSON.parse(ev.data);
          if (m.type === 'screen' && m.w > 0 && m.h > 0) {
            layout.setStreamSize(m.w, m.h);
          } else if (m.type === 'host') {
            taskmgr.renderHostStats(m);
          } else if (m.type === 'control_result') {
            control.handleControlResult(m);
          }
        } catch (_) {}
      };

      if (channel.readyState === 'open') {
        channel.send(JSON.stringify({ type: 'viewer', mobile: isMobile }));
      } else {
        channel.onopen = () => {
          channel.send(JSON.stringify({ type: 'viewer', mobile: isMobile }));
        };
      }
    }

    let statsTimer = null;

    document.addEventListener('visibilitychange', () => {
      if (!document.hidden && video && video.srcObject) {
        video.play().catch(() => {});
      }
    });

    async function handleOffer(offer) {
      if (statsTimer) {
        clearInterval(statsTimer);
        statsTimer = null;
      }
      if (pc) {
        pc.close();
        pc = null;
      }
      if (receiverTuneTimer) {
        clearInterval(receiverTuneTimer);
        receiverTuneTimer = null;
      }
      pendingICE = [];

      pc = new RTCPeerConnection({
        iceServers: ctx.iceServers || [{ urls: 'stun:stun.l.google.com:19302' }],
        bundlePolicy: 'max-bundle',
        rtcpMuxPolicy: 'require',
      });

      pc.ontrack = (e) => {
        const stream = e.streams[0] || new MediaStream([e.track]);
        video.srcObject = stream;
        video.muted = true;
        video.playsInline = true;
        video.setAttribute('playsinline', '');
        video.setAttribute('webkit-playsinline', '');
        video.disablePictureInPicture = true;
        tuneVideoReceivers();
        video.play().catch(() => { status.textContent = 'tap screen to start video'; });
        status.textContent = 'streaming';
        layout.updateLayout();
      };

      pc.onicecandidate = (e) => {
        if (e.candidate) {
          ws.send(JSON.stringify({ type: 'ice', session: code, payload: e.candidate.toJSON() }));
        }
      };
      pc.ondatachannel = (e) => setupInput(e.channel);
      pc.onconnectionstatechange = () => {
        const st = pc.connectionState;
        if (st === 'connected') tuneVideoReceivers();
        if (st === 'failed' || st === 'disconnected') {
          status.textContent = st + ' — check same Wi‑Fi / firewall';
        } else if (status.textContent !== 'streaming') {
          status.textContent = st;
        }
      };
      pc.oniceconnectionstatechange = () => {
        if (pc.iceConnectionState === 'connected') tuneVideoReceivers();
        if (pc.iceConnectionState === 'failed') {
          status.textContent = 'ICE failed — check firewall / TURN / same network';
        }
      };

      try {
        await pc.setRemoteDescription(offer);
        const answer = await pc.createAnswer();
        await pc.setLocalDescription(answer);
        ws.send(JSON.stringify({ type: 'answer', session: code, payload: answer }));
        await flushPendingICE();
        tuneVideoReceivers();
        if (isMobile) {
          receiverTuneTimer = setInterval(tuneVideoReceivers, 1500);
        }
      } catch (e) {
        status.textContent = 'WebRTC error: ' + e.message;
      }

      statsTimer = setInterval(async () => {
        if (!pc) return;
        const rep = await pc.getStats();
        let rtt = 0, loss = 0, frames = 0;
        rep.forEach((s) => {
          if (s.type === 'candidate-pair' && s.state === 'succeeded') rtt = s.currentRoundTripTime || 0;
          if (s.type === 'inbound-rtp' && s.kind === 'video') {
            loss = s.packetsLost / Math.max(1, s.packetsReceived);
            frames = s.framesDecoded || 0;
          }
        });
        taskmgr.renderConnStats(rtt, loss, frames);
        if (frames === 0 && pc.connectionState === 'connected') {
          status.textContent = 'connected — waiting for video decode…';
        } else if (frames > 0 && status.textContent.includes('waiting for video')) {
          status.textContent = 'streaming';
        }
        if (dc && dc.readyState === 'open') {
          dc.send(JSON.stringify({ type: 'viewer', packetLoss: loss, rtt: rtt * 1000, mobile: isMobile }));
        }
      }, 2000);
    }

    ws.onmessage = async (ev) => {
      const msg = JSON.parse(ev.data);
      if (msg.type === 'offer') await handleOffer(msg.payload);
      else if (msg.type === 'ice') await addIceCandidateSafe(msg.payload);
      else if (msg.type === 'joined') status.textContent = 'waiting for host…';
      else if (msg.type === 'peer-present') status.textContent = 'host online — starting stream…';
      else if (msg.type === 'no-host') status.textContent = 'host offline — start connect-agent, then refresh';
    };

    ws.onopen = () => { status.textContent = 'signaling connected'; };

    ws.onerror = () => {
      if (/connecting|signaling connected/i.test(status.textContent)) {
        status.textContent = 'cannot connect — return to Machines and Connect again';
      }
    };

    ws.onclose = (ev) => {
      if (ev.code === 1006 || /connecting/i.test(status.textContent)) {
        status.textContent = 'session invalid or expired — Connect again from Machines';
      } else {
        status.textContent = 'disconnected — refresh page';
      }
    };

    setTimeout(() => {
      if (!pc && !status.textContent.startsWith('streaming')) {
        status.textContent = 'no stream yet — refresh or Connect again from Machines';
      }
    }, 25000);

    return { getDC };
  },
};
