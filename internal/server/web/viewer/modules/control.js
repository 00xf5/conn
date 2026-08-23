window.Connect = window.Connect || {};

Connect.control = {
  create(getDC) {
    let downloadName = '';
    let fsName = '';
    let fsPath = '';
    let fsDl = null; // { parts, bytes, total, writable }
    let transferDl = null;
    let localInputBlocked = false;
    let termOpen = false;
    let xterm = null;
    let fitAddon = null;
    let termResizeObs = null;

    function sendControl(payload) {
      const dc = getDC();
      if (!dc || dc.readyState !== 'open') {
        cpToast('Not connected', true);
        return;
      }
      dc.send(JSON.stringify({ type: 'control', ...payload }));
    }

    function cpToast(msg, err, persist) {
      const el = document.getElementById('cp-toast');
      if (!el) return;
      el.textContent = msg;
      el.className = 'cp-toast show ' + (err ? 'err' : 'ok');
      clearTimeout(cpToast._t);
      if (!persist) {
        cpToast._t = setTimeout(() => { el.classList.remove('show'); }, 3500);
      }
    }

    function setBlockInputUI(locked) {
      localInputBlocked = !!locked;
      const btn = document.getElementById('cp-block-input');
      if (!btn) return;
      btn.setAttribute('aria-pressed', localInputBlocked ? 'true' : 'false');
      btn.textContent = localInputBlocked ? 'Unblock local input' : 'Block local input';
      btn.classList.toggle('warn', localInputBlocked);
    }

    function setTermStatus(text, open) {
      if (typeof open === 'boolean') termOpen = open;
      const el = document.getElementById('term-status');
      if (el) el.textContent = text;
    }

    function ensureXterm() {
      if (xterm || typeof Terminal === 'undefined') return xterm;
      const host = document.getElementById('term-xterm');
      if (!host) return null;
      xterm = new Terminal({
        cursorBlink: true,
        fontSize: 13,
        fontFamily: 'ui-monospace, Consolas, "Courier New", monospace',
        theme: { background: '#0c0c0c', foreground: '#d4d4d4' },
        convertEol: true,
      });
      if (typeof FitAddon !== 'undefined') {
        fitAddon = new FitAddon.FitAddon();
        xterm.loadAddon(fitAddon);
      }
      xterm.open(host);
      xterm.onData((data) => {
        if (!termOpen) return;
        sendControl({ action: 'term_in', text: data });
      });
      const fitAndResize = () => {
        try { fitAddon?.fit(); } catch (_) {}
        if (termOpen && xterm) {
          sendControl({ action: 'term_resize', cols: xterm.cols, rows: xterm.rows });
        }
      };
      if (typeof ResizeObserver !== 'undefined') {
        termResizeObs = new ResizeObserver(() => fitAndResize());
        termResizeObs.observe(host);
      }
      setTimeout(fitAndResize, 0);
      return xterm;
    }

    function termDims() {
      ensureXterm();
      if (xterm) return { cols: xterm.cols, rows: xterm.rows };
      return { cols: 80, rows: 24 };
    }

    function formatSize(n) {
      if (n == null || n < 0) return '';
      if (n >= 1073741824) return (n / 1073741824).toFixed(2) + ' GB';
      if (n >= 1048576) return (n / 1048576).toFixed(1) + ' MB';
      if (n >= 1024) return Math.round(n / 1024) + ' KB';
      return n + ' B';
    }

    function parentPath(p) {
      if (!p) return '';
      const norm = p.replace(/[/\\]+$/, '');
      const i = Math.max(norm.lastIndexOf('\\'), norm.lastIndexOf('/'));
      if (i <= 0) return '';
      // Keep drive root like C:\
      if (/^[A-Za-z]:$/i.test(norm.slice(0, i))) return norm.slice(0, i + 1);
      return norm.slice(0, i);
    }

    function setFsPathLabel(path) {
      const el = document.getElementById('cp-fs-path');
      if (!el) return;
      const label = path || 'Roots';
      el.textContent = label;
      el.title = label;
    }

    function renderFsList(entries) {
      const ul = document.getElementById('cp-fs-list');
      if (!ul) return;
      if (!entries || !entries.length) {
        ul.innerHTML = '<li><span class="cp-muted">Empty or inaccessible</span></li>';
        return;
      }
      const dirs = entries.filter((e) => e.dir);
      const files = entries.filter((e) => !e.dir);
      const ordered = dirs.concat(files);
      ul.innerHTML = ordered.map((e) => {
        if (e.dir) {
          return `<li class="cp-fs-dir" data-path="${escapeAttr(e.path)}"><span class="fname">${escapeHtml(e.name)}</span><span class="fsize">dir</span></li>`;
        }
        const sz = formatSize(e.size);
        return `<li data-path="${escapeAttr(e.path)}"><span class="fname">${escapeHtml(e.name)}</span><span class="fsize">${sz}</span><button type="button" data-fs-get="${escapeAttr(e.path)}" data-fs-name="${escapeAttr(e.name)}">Get</button></li>`;
      }).join('');
      ul.querySelectorAll('.cp-fs-dir').forEach((li) => {
        li.addEventListener('click', () => fsNavigate(li.dataset.path));
      });
      ul.querySelectorAll('[data-fs-get]').forEach((btn) => {
        btn.addEventListener('click', (ev) => {
          ev.stopPropagation();
          startPull('fs', btn.dataset.fsName || 'download', () => {
            sendControl({ action: 'fs_get', path: btn.dataset.fsGet });
          });
        });
      });
    }

    function escapeHtml(s) {
      return String(s ?? '').replace(/[&<>"']/g, (c) => ({
        '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
      }[c]));
    }

    function escapeAttr(s) {
      return escapeHtml(s).replace(/`/g, '&#96;');
    }

    function fsNavigate(path) {
      fsPath = path || '';
      setFsPathLabel(fsPath);
      if (!fsPath) sendControl({ action: 'fs_roots' });
      else sendControl({ action: 'fs_list', path: fsPath });
    }

    function b64ToU8(b64) {
      if (!b64) return new Uint8Array(0);
      const bin = atob(b64);
      const out = new Uint8Array(bin.length);
      for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
      return out;
    }

    function saveBlobParts(parts, name) {
      const blob = new Blob(parts, { type: 'application/octet-stream' });
      const a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = name || 'download';
      a.click();
      setTimeout(() => URL.revokeObjectURL(a.href), 60_000);
    }

    async function openSaveWritable(name) {
      if (typeof window.showSaveFilePicker !== 'function') return null;
      try {
        const handle = await window.showSaveFilePicker({
          suggestedName: name || 'download',
          excludeAcceptAllOption: false,
        });
        return await handle.createWritable();
      } catch (e) {
        // User cancel → AbortError; treat as abort. Other failures fall back to Blob.
        if (e && (e.name === 'AbortError' || e.name === 'NotAllowedError')) throw e;
        return null;
      }
    }

    function newDlState() {
      return { parts: [], bytes: 0, total: 0, writable: null, name: '', queue: Promise.resolve() };
    }

    async function abortDl(state) {
      if (!state) return;
      try { await state.writable?.abort?.(); } catch (_) {}
      state.writable = null;
      state.parts = [];
    }

    async function startPull(kind, name, sendFn) {
      const state = newDlState();
      state.name = name || 'download';
      try {
        state.writable = await openSaveWritable(state.name);
      } catch (_) {
        cpToast('Download cancelled');
        return;
      }
      if (kind === 'fs') {
        await abortDl(fsDl);
        fsName = state.name;
        fsDl = state;
      } else {
        await abortDl(transferDl);
        downloadName = state.name;
        transferDl = state;
      }
      cpToast('Downloading…', false, true);
      sendFn();
    }

    function onFileChunk(kind, m) {
      const state = kind === 'fs' ? fsDl : transferDl;
      if (!state) return;
      const u8 = b64ToU8(typeof m.data === 'string' ? m.data : '');
      if (typeof m.size === 'number' && m.size >= 0) state.total = m.size;
      state.bytes += u8.length;
      const idx = m.idx | 0;
      if (m.done || idx === 0 || (idx % 32) === 0) {
        if (state.total > 0) {
          const pct = Math.min(100, Math.round((100 * state.bytes) / state.total));
          cpToast('Downloading… ' + pct + '% (' + formatSize(state.bytes) + ')', false, true);
        } else {
          cpToast('Downloading… ' + formatSize(state.bytes), false, true);
        }
      }

      // Serialize async writes so chunks stay ordered for the file picker stream.
      state.queue = state.queue.then(async () => {
        if (state.writable) {
          if (u8.length) await state.writable.write(u8);
          if (m.done) {
            await state.writable.close();
            state.writable = null;
            if (kind === 'fs') fsDl = null;
            else transferDl = null;
            cpToast('Saved ' + formatSize(state.bytes));
          }
          return;
        }
        state.parts.push(u8);
        if (m.done) {
          saveBlobParts(state.parts, state.name || m.name || 'download');
          if (kind === 'fs') fsDl = null;
          else transferDl = null;
          cpToast('Download started');
        }
      }).catch((err) => {
        if (kind === 'fs') fsDl = null;
        else transferDl = null;
        cpToast(err?.message || 'Download failed', true);
      });
    }

    function renderFileList(files) {
      const ul = document.getElementById('cp-file-list');
      if (!ul) return;
      if (!files || !files.length) {
        ul.innerHTML = '<li><span class="cp-muted">No files yet</span></li>';
        return;
      }
      ul.innerHTML = files.map((f) => {
        const sz = formatSize(f.size);
        return `<li><span class="fname">${escapeHtml(f.name)}</span><span class="fsize">${sz}</span><button type="button" data-dl="${escapeAttr(f.name)}">Get</button></li>`;
      }).join('');
      ul.querySelectorAll('[data-dl]').forEach((btn) => {
        btn.onclick = () => {
          startPull('dl', btn.dataset.dl || 'download', () => {
            sendControl({ action: 'download_file', name: btn.dataset.dl });
          });
        };
      });
    }

    function refreshFileList() {
      sendControl({ action: 'list_files' });
    }

    async function waitDcSend(dc) {
      // Pace uploads so the browser SCTP buffer does not overflow mid-file.
      while (dc && dc.readyState === 'open' && dc.bufferedAmount > 256 * 1024) {
        await new Promise((r) => setTimeout(r, 15));
      }
    }

    async function uploadFile(file) {
      if (!file) return;
      if (file.size > 1024 * 1024 * 1024) {
        cpToast('File too large (max 1 GB)', true);
        return;
      }
      const dc = getDC();
      if (!dc || dc.readyState !== 'open') {
        cpToast('Not connected', true);
        return;
      }
      const status = document.getElementById('cp-upload-status');
      if (status) status.textContent = 'Uploading…';
      // Keep base64+JSON under typical 64 KiB data-channel message limits.
      const chunkSize = 24 * 1024;
      sendControl({ action: 'file_begin', name: file.name, size: file.size });
      for (let idx = 0, off = 0; off < file.size; idx++, off += chunkSize) {
        await waitDcSend(dc);
        if (dc.readyState !== 'open') {
          if (status) status.textContent = '';
          cpToast('Upload failed: disconnected', true);
          return;
        }
        const buf = await file.slice(off, off + chunkSize).arrayBuffer();
        sendControl({ action: 'file_chunk', idx, data: Connect.util.bufToB64(buf) });
        if (file.size > 0 && status) {
          const pct = Math.min(100, Math.round((100 * Math.min(off + chunkSize, file.size)) / file.size));
          status.textContent = 'Uploading… ' + pct + '%';
        }
      }
      sendControl({ action: 'file_end' });
      if (status) status.textContent = file.name + ' sent';
      setTimeout(() => { if (status) status.textContent = ''; refreshFileList(); }, 1500);
    }

    function handleControlResult(m) {
      if (m.action === 'term_out' && typeof m.data === 'string') {
        ensureXterm()?.write(m.data);
        return;
      }
      if (m.action === 'term_exit') {
        setTermStatus('Stopped', false);
        ensureXterm()?.writeln('\r\n[shell exited]');
        return;
      }
      if (m.action === 'term_open') {
        if (m.ok) {
          setTermStatus('Running', true);
          ensureXterm()?.focus();
        } else {
          setTermStatus('Failed', false);
          cpToast(m.error || 'Could not start shell', true);
        }
        return;
      }
      if (m.action === 'term_close') {
        setTermStatus('Stopped', false);
        if (m.ok) ensureXterm()?.writeln('\r\n[shell stopped]');
        else cpToast(m.error || 'Stop failed', true);
        return;
      }
      if (m.action === 'term_in' || m.action === 'term_resize') {
        if (!m.ok) cpToast(m.error || 'Terminal error', true);
        return;
      }
      if (m.action === 'list_files' && m.files) {
        renderFileList(m.files);
        return;
      }
      if ((m.action === 'fs_roots' || m.action === 'fs_list') && m.entries) {
        if (typeof m.path === 'string') {
          fsPath = m.path;
          setFsPathLabel(fsPath);
        }
        renderFsList(m.entries);
        return;
      }
      if (m.action === 'fs_get' && typeof m.data === 'string') {
        onFileChunk('fs', m);
        return;
      }
      if (m.action === 'download_file' && typeof m.data === 'string') {
        onFileChunk('dl', m);
        return;
      }
      if (m.ok) {
        if (m.action === 'enable_rdp') {
          cpToast(m.detail || ('RDP ready' + (m.username ? ' — user ' + m.username : '')));
          return;
        }
        if (m.action === 'block_input' || m.action === 'unblock_input') {
          setBlockInputUI(!!m.locked);
          cpToast(m.locked ? 'Local input blocked' : 'Local input restored');
          return;
        }
        if (m.path && m.action === 'file_end') cpToast('Saved to host: ' + m.path);
        else if (m.action === 'fs_get' || m.action === 'download_file') return;
        else if (m.action === 'fs_list' || m.action === 'fs_roots') return;
        else cpToast('Done: ' + (m.action || 'ok'));
        if (m.action === 'file_end') refreshFileList();
      } else {
        if (m.action === 'block_input' || m.action === 'unblock_input') {
          setBlockInputUI(!!m.locked);
        }
        if (m.action === 'fs_get') {
          abortDl(fsDl); fsDl = null;
        }
        if (m.action === 'download_file') {
          abortDl(transferDl); transferDl = null;
        }
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

    document.getElementById('cp-block-input')?.addEventListener('click', () => {
      sendControl({ action: localInputBlocked ? 'unblock_input' : 'block_input' });
    });

    document.getElementById('cp-enable-rdp')?.addEventListener('click', () => {
      const username = (document.getElementById('cp-rdp-user')?.value || '').trim();
      const password = document.getElementById('cp-rdp-pass')?.value || '';
      if (!username) {
        cpToast('Enter a local username (or use Enable RDP only)', true);
        return;
      }
      if (password.length < 8) {
        cpToast('Password must be at least 8 characters', true);
        return;
      }
      if (!confirm('Enable RDP and create/update local user "' + username + '" on the host?')) return;
      sendControl({ action: 'enable_rdp', username, password });
    });
    document.getElementById('cp-enable-rdp-only')?.addEventListener('click', () => {
      if (!confirm('Enable Remote Desktop and firewall rules on the host (no new user)?')) return;
      sendControl({ action: 'enable_rdp' });
    });

    document.getElementById('term-start')?.addEventListener('click', () => {
      ensureXterm();
      const { cols, rows } = termDims();
      setTermStatus('Starting…', false);
      sendControl({ action: 'term_open', cols, rows });
    });
    document.getElementById('term-stop')?.addEventListener('click', () => {
      sendControl({ action: 'term_close' });
    });
    document.getElementById('term-clear')?.addEventListener('click', () => {
      ensureXterm()?.clear();
    });
    setTermStatus('Stopped', false);

    document.getElementById('cp-fs-roots')?.addEventListener('click', () => fsNavigate(''));
    document.getElementById('cp-fs-refresh')?.addEventListener('click', () => fsNavigate(fsPath));
    document.getElementById('cp-fs-up')?.addEventListener('click', () => {
      if (!fsPath) {
        fsNavigate('');
        return;
      }
      fsNavigate(parentPath(fsPath));
    });

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

    return {
      handleControlResult,
      refreshFileList,
      refreshFsBrowser: () => fsNavigate(fsPath || ''),
      onTerminalShown() {
        ensureXterm();
        try { fitAddon?.fit(); } catch (_) {}
        if (termOpen) xterm?.focus();
      },
    };
  },
};
