window.Connect = window.Connect || {};

Connect.webrtc = {
  create(ctx, hooks) {
    const { code, status, video, overlay } = ctx;
    const { layout, taskmgr, control } = hooks;
    const remoteAudio = document.getElementById('remote-audio');
    const btnMic = document.getElementById('btn-mic');

    const isMobile = /Android|iPhone|iPad|iPod|Mobile/i.test(navigator.userAgent);

    const wsProto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(`${wsProto}//${location.host}/ws?role=viewer&session=${encodeURIComponent(code)}`);

    let pc = null;
    let dc = null;
    let moveQueue = null;
    let moveTimer = null;
    let pendingICE = [];
    let receiverTuneTimer = null;
    let micTrack = null;
    let micStream = null;
    let micEnabled = false;

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

    function preferPCMU() {
      if (!pc || typeof RTCRtpSender === 'undefined' || !RTCRtpSender.getCapabilities) return;
      const caps = RTCRtpSender.getCapabilities('audio');
      if (!caps || !caps.codecs) return;
      const preferred = [
        ...caps.codecs.filter((c) => /pcmu/i.test(c.mimeType)),
        ...caps.codecs.filter((c) => !/pcmu/i.test(c.mimeType)),
      ];
      if (!preferred.length) return;
      for (const tr of pc.getTransceivers()) {
        if (tr.receiver?.track?.kind !== 'audio') continue;
        try { tr.setCodecPreferences(preferred); } catch (_) {}
      }
    }

    function setMicButton(on) {
      if (!btnMic) return;
      micEnabled = !!on;
      btnMic.classList.toggle('mic-on', micEnabled);
      btnMic.classList.toggle('mic-off', !micEnabled);
      btnMic.setAttribute('aria-pressed', micEnabled ? 'true' : 'false');
      btnMic.title = micEnabled
        ? 'Microphone is on — click to mute'
        : 'Microphone is off — click to unmute';
      btnMic.textContent = micEnabled ? 'Mic On' : 'Mic';
    }

    async function ensureMicTrack() {
      if (micTrack && micTrack.readyState !== 'ended') return micTrack;
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: {
          echoCancellation: true,
          noiseSuppression: true,
          autoGainControl: true,
        },
        video: false,
      });
      micStream = stream;
      micTrack = stream.getAudioTracks()[0] || null;
      return micTrack;
    }

    async function attachMicTrack(track) {
      if (!pc || !track) return;
      const audioSender = pc.getSenders().find((s) => s.track && s.track.kind === 'audio');
      if (audioSender) {
        await audioSender.replaceTrack(track);
        return;
      }
      const tr = pc.getTransceivers().find((t) =>
        (t.receiver && t.receiver.track && t.receiver.track.kind === 'audio') ||
        (t.mid != null && (!t.sender.track || t.sender.track.kind === 'audio'))
      );
      if (tr && tr.sender) {
        await tr.sender.replaceTrack(track);
        try { tr.direction = 'sendrecv'; } catch (_) {}
        return;
      }
      pc.addTrack(track, micStream || new MediaStream([track]));
    }

    async function toggleMic() {
      if (!pc) {
        status.textContent = 'wait for connection before enabling mic';
        return;
      }
      try {
        if (!micEnabled) {
          const track = await ensureMicTrack();
          if (!track) throw new Error('no microphone');
          track.enabled = true;
          await attachMicTrack(track);
          setMicButton(true);
        } else if (micTrack) {
          micTrack.enabled = false;
          setMicButton(false);
        }
      } catch (e) {
        setMicButton(false);
        status.textContent = 'mic unavailable: ' + (e && e.message ? e.message : e);
      }
    }

    function stopMic() {
      if (micTrack) {
        try { micTrack.stop(); } catch (_) {}
        micTrack = null;
      }
      if (micStream) {
        try { micStream.getTracks().forEach((t) => t.stop()); } catch (_) {}
        micStream = null;
      }
      setMicButton(false);
    }

    if (btnMic) {
      setMicButton(false);
      btnMic.addEventListener('click', () => { toggleMic(); });
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
      if (!document.hidden && remoteAudio && remoteAudio.srcObject) {
        remoteAudio.play().catch(() => {});
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
      stopMic();
      if (receiverTuneTimer) {
        clearInterval(receiverTuneTimer);
        receiverTuneTimer = null;
      }
      pendingICE = [];
      if (remoteAudio) {
        remoteAudio.srcObject = null;
      }

      pc = new RTCPeerConnection({
        iceServers: ctx.iceServers || [{ urls: 'stun:stun.l.google.com:19302' }],
        bundlePolicy: 'max-bundle',
        rtcpMuxPolicy: 'require',
      });

      pc.ontrack = (e) => {
        if (e.track.kind === 'audio') {
          if (remoteAudio) {
            const stream = e.streams[0] || new MediaStream([e.track]);
            remoteAudio.srcObject = stream;
            remoteAudio.play().catch(() => {});
          }
          return;
        }
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
        preferPCMU();
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
      stopMic();
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
