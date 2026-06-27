window.Connect = window.Connect || {};

Connect.input = (function () {
  const Input = {
    MouseMove: 0x01, MouseDown: 0x02, MouseUp: 0x03,
    KeyDown: 0x04, KeyUp: 0x05, Wheel: 0x06,
  };

  function u16(n) { return Math.max(0, Math.min(65535, n | 0)); }

  return {
    encMouseMove(x, y) {
      const b = new Uint8Array(5);
      b[0] = Input.MouseMove;
      new DataView(b.buffer).setUint16(1, u16(x), true);
      new DataView(b.buffer).setUint16(3, u16(y), true);
      return b;
    },
    encMouseBtn(down, btn, x, y) {
      const b = new Uint8Array(6);
      b[0] = down ? Input.MouseDown : Input.MouseUp;
      b[1] = btn;
      new DataView(b.buffer).setUint16(2, u16(x), true);
      new DataView(b.buffer).setUint16(4, u16(y), true);
      return b;
    },
    encKey(down, vk) {
      const b = new Uint8Array(3);
      b[0] = down ? Input.KeyDown : Input.KeyUp;
      new DataView(b.buffer).setUint16(1, vk, true);
      return b;
    },
    bindOverlay(overlay, video, getStreamSize, sendBinary, queueMove, isCover) {
      const { encMouseMove, encMouseBtn, encKey } = Connect.input;

      function mapPointer(clientX, clientY) {
        const vr = video.getBoundingClientRect();
        const { w: streamW, h: streamH } = getStreamSize();
        const srcW = streamW || video.videoWidth || vr.width;
        const srcH = streamH || video.videoHeight || vr.height;
        if (!srcW || !srcH) return [0, 0];
        const cover = isCover ? isCover() : video.classList.contains('fill');
        const scale = cover
          ? Math.max(vr.width / srcW, vr.height / srcH)
          : Math.min(vr.width / srcW, vr.height / srcH);
        const dispW = srcW * scale;
        const dispH = srcH * scale;
        const left = vr.left + (vr.width - dispW) / 2;
        const top = vr.top + (vr.height - dispH) / 2;
        let nx = (clientX - left) / dispW;
        let ny = (clientY - top) / dispH;
        nx = Math.max(0, Math.min(1, nx));
        ny = Math.max(0, Math.min(1, ny));
        return [u16(nx * 65535), u16(ny * 65535)];
      }

      overlay.addEventListener('mousemove', (e) => {
        const [x, y] = mapPointer(e.clientX, e.clientY);
        queueMove(encMouseMove(x, y));
      });
      overlay.addEventListener('mousedown', (e) => {
        const [x, y] = mapPointer(e.clientX, e.clientY);
        sendBinary(encMouseMove(x, y));
        sendBinary(encMouseBtn(true, e.button, x, y));
      });
      overlay.addEventListener('mouseup', (e) => {
        const [x, y] = mapPointer(e.clientX, e.clientY);
        sendBinary(encMouseBtn(false, e.button, x, y));
      });
      overlay.addEventListener('touchstart', (e) => {
        e.preventDefault();
        if (video.paused) video.play().catch(() => {});
        const t = e.changedTouches[0];
        const [x, y] = mapPointer(t.clientX, t.clientY);
        sendBinary(encMouseMove(x, y));
        sendBinary(encMouseBtn(true, 0, x, y));
      }, { passive: false });
      overlay.addEventListener('touchmove', (e) => {
        e.preventDefault();
        const t = e.changedTouches[0];
        const [x, y] = mapPointer(t.clientX, t.clientY);
        queueMove(encMouseMove(x, y));
      }, { passive: false });
      overlay.addEventListener('touchend', (e) => {
        e.preventDefault();
        const t = e.changedTouches[0];
        const [x, y] = mapPointer(t.clientX, t.clientY);
        sendBinary(encMouseBtn(false, 0, x, y));
      }, { passive: false });
      overlay.addEventListener('keydown', (e) => {
        sendBinary(encKey(true, e.keyCode));
        e.preventDefault();
      });
      overlay.addEventListener('keyup', (e) => {
        sendBinary(encKey(false, e.keyCode));
        e.preventDefault();
      });
    },
  };
})();
