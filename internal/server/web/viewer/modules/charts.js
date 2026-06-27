window.Connect = window.Connect || {};

Connect.charts = {
  drawSpark(canvas, data, color) {
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    const w = canvas.width;
    const h = canvas.height;
    ctx.clearRect(0, 0, w, h);
    if (!data.length) return;
    ctx.strokeStyle = color || '#0078d4';
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    data.forEach((v, i) => {
      const x = (i / Math.max(1, data.length - 1)) * (w - 2) + 1;
      const y = h - 2 - (v / 100) * (h - 4);
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.stroke();
  },

  drawGraph(canvas, data, color) {
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    const w = canvas.width;
    const h = canvas.height;
    ctx.fillStyle = '#2b2b2b';
    ctx.fillRect(0, 0, w, h);
    ctx.strokeStyle = '#3a3a3a';
    ctx.lineWidth = 1;
    for (let i = 1; i < 4; i++) {
      const y = (h / 4) * i;
      ctx.beginPath();
      ctx.moveTo(0, y);
      ctx.lineTo(w, y);
      ctx.stroke();
    }
    if (!data.length) return;
    const stroke = color || '#0078d4';
    ctx.fillStyle = stroke.replace(')', ', 0.35)').replace('rgb', 'rgba').replace('#0078d4', 'rgba(0, 120, 212, 0.35)');
    if (stroke.startsWith('#')) ctx.fillStyle = 'rgba(0, 120, 212, 0.35)';
    ctx.strokeStyle = stroke;
    ctx.lineWidth = 2;
    ctx.beginPath();
    data.forEach((v, i) => {
      const x = (i / Math.max(1, data.length - 1)) * (w - 4) + 2;
      const y = h - 4 - (v / 100) * (h - 8);
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.lineTo(w - 2, h - 2);
    ctx.lineTo(2, h - 2);
    ctx.closePath();
    ctx.fill();
    ctx.beginPath();
    data.forEach((v, i) => {
      const x = (i / Math.max(1, data.length - 1)) * (w - 4) + 2;
      const y = h - 4 - (v / 100) * (h - 8);
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.stroke();
  },
};
