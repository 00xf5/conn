window.Connect = window.Connect || {};

Connect.layout = {
  create(ctx) {
    const { stage, layout, videoPane, video, btnFit, btnFill, overlay } = ctx;
    let layoutMode = 'fit';
    let streamW = 0;
    let streamH = 0;

    function sourceAspect() {
      const w = streamW || video.videoWidth || 16;
      const h = streamH || video.videoHeight || 9;
      return w / h;
    }

    function isFullscreen() {
      return document.fullscreenElement || document.webkitFullscreenElement || null;
    }

    function updateLayout() {
      if (!stage || !videoPane || !layout) return;
      if (isFullscreen()) {
        // The fullscreen element fills the screen; let CSS drive sizing.
        videoPane.style.width = '';
        videoPane.style.flex = '';
        return;
      }
      if (layoutMode === 'fill' || window.innerWidth <= 900) {
        layout.classList.add('fill-mode');
        videoPane.style.width = '';
        videoPane.style.flex = '';
        return;
      }
      layout.classList.remove('fill-mode');
      const ar = sourceAspect();
      const sh = stage.clientHeight;
      const minPanel = 480;
      let vw = sh * ar;
      const maxVideo = Math.max(240, stage.clientWidth - minPanel);
      if (vw > maxVideo) vw = maxVideo;
      videoPane.style.width = `${Math.round(vw)}px`;
      videoPane.style.flex = `0 0 ${Math.round(vw)}px`;
    }

    function setLayoutMode(mode) {
      layoutMode = mode;
      video.classList.toggle('fill', mode === 'fill');
      btnFit.classList.toggle('active', mode === 'fit');
      btnFill.classList.toggle('active', mode === 'fill');
      updateLayout();
    }

    function setStreamSize(w, h) {
      streamW = w;
      streamH = h;
      updateLayout();
    }

    btnFit.onclick = () => setLayoutMode('fit');
    btnFill.onclick = () => setLayoutMode('fill');

    function exitFullscreen() {
      const fn = document.exitFullscreen || document.webkitExitFullscreen;
      if (fn) fn.call(document);
    }
    function enterFullscreen() {
      // Fullscreen the pane that holds BOTH the video and the input overlay,
      // otherwise the transparent overlay alone shows a blank screen.
      const target = videoPane || overlay;
      const req = target.requestFullscreen || target.webkitRequestFullscreen || target.msRequestFullscreen;
      if (req) { req.call(target); return; }
      if (video.webkitEnterFullscreen) video.webkitEnterFullscreen(); // iOS Safari fallback
    }
    const btnFull = document.getElementById('btn-full');
    if (btnFull) {
      btnFull.onclick = () => { if (isFullscreen()) exitFullscreen(); else enterFullscreen(); };
    }
    const onFsChange = () => {
      updateLayout();
      if (isFullscreen()) {
        try { video.play().catch(() => {}); } catch (_) {}
        try { overlay.focus({ preventScroll: true }); } catch (_) { overlay.focus(); }
      }
    };
    document.addEventListener('fullscreenchange', onFsChange);
    document.addEventListener('webkitfullscreenchange', onFsChange);

    window.addEventListener('resize', updateLayout);
    video.addEventListener('loadedmetadata', updateLayout);
    setLayoutMode('fit');

    return { updateLayout, setStreamSize, getStreamSize: () => ({ w: streamW, h: streamH }), isCover: () => video.classList.contains('fill') };
  },
};
