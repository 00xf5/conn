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

    function updateLayout() {
      if (!stage || !videoPane || !layout) return;
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
    overlay.requestFullscreen && (document.getElementById('btn-full').onclick = () => overlay.requestFullscreen());
    window.addEventListener('resize', updateLayout);
    video.addEventListener('loadedmetadata', updateLayout);
    setLayoutMode('fit');

    return { updateLayout, setStreamSize, getStreamSize: () => ({ w: streamW, h: streamH }), isCover: () => video.classList.contains('fill') };
  },
};
