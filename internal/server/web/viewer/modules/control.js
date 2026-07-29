window.Connect = window.Connect || {};

Connect.control = {
  create(getDC) {
    let downloadChunks = [];
    let downloadName = '';
    let fsChunks = [];
    let fsName = '';
    let fsPath = '';
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

    function cpToast(msg, err) {
      const el = document.getElementById('cp-toast');
      if (!el) return;
      el.textContent = msg;
      el.className = 'cp-toast show ' + (err ? 'err' : 'ok');
      clearTimeout(cpToast._t);
      cpToast._t = setTimeout(() => { el.classList.remove('show'); }, 3500);
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
          fsName = btn.dataset.fsName || 'download';
          fsChunks = [];
          cpToast('Downloading…');
          sendControl({ action: 'fs_get', path: btn.dataset.fsGet });
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

    function assembleDownload(chunks, name) {
      const parts = chunks.map((b64) => Uint8Array.from(atob(b64), (c) => c.charCodeAt(0)));
      const total = parts.reduce((s, p) => s + p.length, 0);
      const bin = new Uint8Array(total);
      let off = 0;
      parts.forEach((p) => { bin.set(p, off); off += p.length; });
      const blob = new Blob([bin]);
      const a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = name || 'download';
      a.click();
      URL.revokeObjectURL(a.href);
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
      if (m.action === 'fs_get' && m.data) {
        fsChunks.push(m.data);
        if (m.done) {
          assembleDownload(fsChunks, fsName || m.name || 'download');
          fsChunks = [];
          cpToast('Download started');
        }
        return;
      }
      if (m.action === 'download_file' && m.data) {
        downloadChunks.push(m.data);
        if (m.done) {
          assembleDownload(downloadChunks, downloadName || m.name || 'download');
          downloadChunks = [];
          cpToast('Download started');
        }
        return;
      }
      if (m.ok) {
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
