window.ConnectViewer = {
  async init(opts) {
    const code = (opts.sessionCode || '').toUpperCase();
    let iceServers = [{ urls: 'stun:stun.l.google.com:19302' }];
    try {
      const r = await fetch('/api/ice');
      if (r.ok) {
        const j = await r.json();
        if (j.iceServers && j.iceServers.length) iceServers = j.iceServers;
      }
    } catch (_) {}

    const ctx = {
      code,
      iceServers,
      status: document.getElementById('status'),
      video: document.getElementById('video'),
      overlay: document.getElementById('overlay'),
      statsEl: document.getElementById('stats'),
      stage: document.getElementById('stage'),
      layout: document.getElementById('layout'),
      videoPane: document.getElementById('video-pane'),
      btnFit: document.getElementById('btn-fit'),
      btnFill: document.getElementById('btn-fill'),
    };

    const layout = Connect.layout.create(ctx);
    const taskmgr = Connect.taskmgr.create(ctx);
    const webrtcRef = { getDC: () => null };
    const control = Connect.control.create(() => webrtcRef.getDC());
    taskmgr.setOnControlView(() => control.refreshFileList());

    const webrtc = Connect.webrtc.create(ctx, { layout, taskmgr, control });
    webrtcRef.getDC = webrtc.getDC;
  },
};
