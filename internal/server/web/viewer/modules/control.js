window.Connect = window.Connect || {};

Connect.control = {
  create(getDC) {
    let downloadChunks = [];
    let downloadName = '';

    function sendControl(payload) {
      const dc = getDC();
      if (!dc || dc.readyState !== 'open') {
        cpToast('Not connected', true);
        return;
      }
      dc.send(JSON.stringify({ type: 'control', ...payload }));
    }

    function cpToast(msg, err) {
      const el = document.getElementById('cp-toast');
      if (!el) return;
      el.textContent = msg;
      el.className = 'cp-toast show ' + (err ? 'err' : 'ok');
      clearTimeout(cpToast._t);
      cpToast._t = setTimeout(() => { el.classList.remove('show'); }, 3500);
    }

    function renderFileList(files) {
      const ul = document.getElementById('cp-file-list');
      if (!ul) return;
      if (!files || !files.length) {
        ul.innerHTML = '<li><span class="cp-muted">No files yet</span></li>';
        return;
      }
      ul.innerHTML = files.map((f) => {
        const sz = f.size >= 1048576 ? (f.size / 1048576).toFixed(1) + ' MB' : Math.round(f.size / 1024) + ' KB';
        return `<li><span class="fname">${f.name}</span><span class="fsize">${sz}</span><button type="button" data-dl="${f.name}">Get</button></li>`;
      }).join('');
      ul.querySelectorAll('[data-dl]').forEach((btn) => {
        btn.onclick = () => {
          downloadName = btn.dataset.dl;
          downloadChunks = [];
          sendControl({ action: 'download_file', name: downloadName });
        };
      });
    }

    function refreshFileList() {
      sendControl({ action: 'list_files' });
    }

    async function uploadFile(file) {
      if (!file) return;
      const status = document.getElementById('cp-upload-status');
      if (status) status.textContent = 'Uploading…';
      const chunkSize = 48 * 1024;
      sendControl({ action: 'file_begin', name: file.name, size: file.size });
      for (let idx = 0, off = 0; off < file.size; idx++, off += chunkSize) {
        const buf = await file.slice(off, off + chunkSize).arrayBuffer();
        sendControl({ action: 'file_chunk', idx, data: Connect.util.bufToB64(buf) });
      }
      sendControl({ action: 'file_end' });
      if (status) status.textContent = file.name + ' sent';
      setTimeout(() => { if (status) status.textContent = ''; refreshFileList(); }, 1500);
    }

    function handleControlResult(m) {
      if (m.action === 'list_files' && m.files) {
        renderFileList(m.files);
        return;
      }
      if (m.action === 'download_file' && m.data) {
        downloadChunks.push(m.data);
        if (m.done) {
          const parts = downloadChunks.map((b64) => Uint8Array.from(atob(b64), (c) => c.charCodeAt(0)));
          const total = parts.reduce((s, p) => s + p.length, 0);
          const bin = new Uint8Array(total);
          let off = 0;
          parts.forEach((p) => { bin.set(p, off); off += p.length; });
          downloadChunks = [];
          const blob = new Blob([bin]);
          const a = document.createElement('a');
          a.href = URL.createObjectURL(blob);
          a.download = downloadName || m.name || 'download';
          a.click();
          URL.revokeObjectURL(a.href);
          cpToast('Download started');
        }
        return;
      }
      if (m.ok) {
        if (m.path) cpToast('Saved to host: ' + m.path);
        else cpToast('Done: ' + (m.action || 'ok'));
        if (m.action === 'file_end') refreshFileList();
      } else {
        cpToast(m.error || 'Action failed', true);
      }
    }

    document.querySelectorAll('.cp-btn[data-act]').forEach((btn) => {
      btn.addEventListener('click', () => {
        const act = btn.dataset.act;
        if (act === 'reboot' || act === 'shutdown') {
          if (!confirm(act === 'reboot' ? 'Restart the host PC in 5 seconds?' : 'Shut down the host PC in 5 seconds?')) return;
        }
        sendControl({ action: act });
      });
    });

    document.getElementById('cp-send-clip')?.addEventListener('click', () => {
      sendControl({ action: 'clipboard', text: document.getElementById('cp-clipboard')?.value || '' });
    });
    document.getElementById('cp-open-url')?.addEventListener('click', () => {
      sendControl({ action: 'open_url', url: document.getElementById('cp-url')?.value || '' });
    });
    document.getElementById('cp-run-cmd')?.addEventListener('click', () => {
      sendControl({ action: 'run', cmd: document.getElementById('cp-cmd')?.value || '' });
    });
    document.getElementById('cp-refresh-files')?.addEventListener('click', refreshFileList);

    const fileInput = document.getElementById('cp-file-input');
    const drop = document.getElementById('cp-drop');
    fileInput?.addEventListener('change', () => {
      if (fileInput.files[0]) uploadFile(fileInput.files[0]);
      fileInput.value = '';
    });
    drop?.addEventListener('dragover', (e) => { e.preventDefault(); drop.classList.add('drag'); });
    drop?.addEventListener('dragleave', () => drop.classList.remove('drag'));
    drop?.addEventListener('drop', (e) => {
      e.preventDefault();
      drop.classList.remove('drag');
      if (e.dataTransfer.files[0]) uploadFile(e.dataTransfer.files[0]);
    });

    const br = document.getElementById('cp-bitrate');
    const brVal = document.getElementById('cp-bitrate-val');
    let brTimer = null;
    br?.addEventListener('input', () => {
      if (brVal) brVal.textContent = br.value;
      clearTimeout(brTimer);
      brTimer = setTimeout(() => sendControl({ action: 'set_bitrate', bitrateK: parseInt(br.value, 10) }), 300);
    });

    return { handleControlResult, refreshFileList };
  },
};
