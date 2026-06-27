window.Connect = window.Connect || {};

Connect.taskmgr = {
  create(ctx) {
    const { statsEl } = ctx;
    const { fmtUptimeLong, fmtMem, pushHist } = Connect.util;
    const { drawSpark, drawGraph } = Connect.charts;

    const tm = {
      hostName: document.getElementById('host-name'),
      viewPerf: document.getElementById('view-performance'),
      viewProc: document.getElementById('view-processes'),
      viewControl: document.getElementById('view-control'),
      panelTitle: document.getElementById('panel-title'),
      perfTitle: document.getElementById('perf-title'),
      perfStats: document.getElementById('perf-stats'),
      perfGraph: document.getElementById('perf-graph'),
      sparkCpu: document.getElementById('spark-cpu'),
      sparkMem: document.getElementById('spark-mem'),
      sparkDisk: document.getElementById('spark-disk'),
      sparkNet: document.getElementById('spark-net'),
      sideCpu: document.getElementById('side-cpu'),
      sideMem: document.getElementById('side-mem'),
      sideDisk: document.getElementById('side-disk'),
      sideNet: document.getElementById('side-net'),
      procBody: document.getElementById('proc-body'),
      procCount: document.getElementById('proc-count'),
      procFilter: document.getElementById('proc-filter'),
    };

    let tmView = 'performance';
    let tmMetric = 'cpu';
    let lastHost = null;
    let connStats = { rtt: 0, loss: 0, frames: 0 };
    let procSort = { col: 'cpu', asc: false };
    const hist = { cpu: [], mem: [], disk: [], net: [] };
    let onControlView = null;

    function statCell(label, value) {
      return `<div class="tm-stat"><span class="tm-stat-label">${label}</span><span class="tm-stat-value">${value}</span></div>`;
    }

    function renderPerfDetail(m) {
      if (!m || !tm.perfStats) return;
      const diskUsedPct = m.diskTotalGb > 0 ? ((m.diskTotalGb - m.diskFreeGb) / m.diskTotalGb) * 100 : 0;
      const titles = { cpu: 'CPU', memory: 'Memory', disk: 'Disk 0 (C:)', ethernet: 'Ethernet' };
      if (tm.perfTitle) tm.perfTitle.textContent = titles[tmMetric] || 'CPU';
      let stats = '';
      if (tmMetric === 'cpu') {
        stats = [
          statCell('Utilization', `${Math.round(m.cpu || 0)}%`),
          statCell('Logical processors', String(m.cpuCores || '—')),
          statCell('Processes', String((m.processes || []).length)),
          statCell('Up time', fmtUptimeLong(m.uptimeSec)),
          statCell('Stream encoder', m.encoder || '—'),
          statCell('Stream target', m.fps && m.bitrateK ? `${m.fps} fps · ${m.bitrateK} kbps` : '—'),
        ].join('');
        drawGraph(tm.perfGraph, hist.cpu, '#0078d4');
      } else if (tmMetric === 'memory') {
        stats = [
          statCell('In use', `${(m.memUsedGb || 0).toFixed(1)} GB`),
          statCell('Available', `${((m.memTotalGb || 0) - (m.memUsedGb || 0)).toFixed(1)} GB`),
          statCell('Committed', `${(m.memTotalGb || 0).toFixed(1)} GB`),
          statCell('Cached', '—'),
        ].join('');
        drawGraph(tm.perfGraph, hist.mem, '#8764b8');
      } else if (tmMetric === 'disk') {
        stats = [
          statCell('Active time', `${Math.round(diskUsedPct)}%`),
          statCell('Free space', `${(m.diskFreeGb || 0).toFixed(1)} GB`),
          statCell('Total capacity', `${(m.diskTotalGb || 0).toFixed(1)} GB`),
          statCell('Used space', `${((m.diskTotalGb || 0) - (m.diskFreeGb || 0)).toFixed(1)} GB`),
        ].join('');
        drawGraph(tm.perfGraph, hist.disk, '#00a86b');
      } else {
        stats = [
          statCell('RTT', `${(connStats.rtt * 1000).toFixed(0)} ms`),
          statCell('Packet loss', `${(connStats.loss * 100).toFixed(1)}%`),
          statCell('Frames decoded', String(connStats.frames)),
          statCell('Session', 'WebRTC viewer'),
        ].join('');
        drawGraph(tm.perfGraph, hist.net, '#e67e22');
      }
      tm.perfStats.innerHTML = stats;
    }

    function renderProcessTable(m) {
      if (!tm.procBody) return;
      const q = (tm.procFilter && tm.procFilter.value || '').trim().toLowerCase();
      let rows = (m && m.processes) ? m.processes.slice() : [];
      if (q) {
        rows = rows.filter((p) =>
          (p.name || '').toLowerCase().includes(q) || String(p.pid || '').includes(q));
      }
      rows.sort((a, b) => {
        let av; let bv;
        switch (procSort.col) {
          case 'name': av = (a.name || '').toLowerCase(); bv = (b.name || '').toLowerCase(); break;
          case 'pid': av = a.pid || 0; bv = b.pid || 0; break;
          case 'mem': av = a.memMb || 0; bv = b.memMb || 0; break;
          default: av = a.cpu || 0; bv = b.cpu || 0;
        }
        if (av < bv) return procSort.asc ? -1 : 1;
        if (av > bv) return procSort.asc ? 1 : -1;
        return 0;
      });
      if (tm.procCount) tm.procCount.textContent = `${rows.length} processes`;
      tm.procBody.innerHTML = rows.map((p) => {
        const initial = (p.name || '?').charAt(0).toUpperCase();
        return `<tr>
          <td><div class="proc-name"><span class="proc-icon">${initial}</span>${p.name || '?'}</div></td>
          <td class="num">${(p.cpu || 0).toFixed(1)}%</td>
          <td class="num">${fmtMem(p.memMb || 0)}</td>
          <td class="num">${p.pid || ''}</td>
        </tr>`;
      }).join('');
    }

    function renderHostStats(m) {
      if (!m || m.type !== 'host') return;
      lastHost = m;
      if (tm.hostName) tm.hostName.textContent = m.hostname || 'Host';
      const diskUsedPct = m.diskTotalGb > 0 ? ((m.diskTotalGb - m.diskFreeGb) / m.diskTotalGb) * 100 : 0;
      pushHist(hist.cpu, m.cpu);
      pushHist(hist.mem, m.memPct);
      pushHist(hist.disk, diskUsedPct);
      const netLoad = Math.min(100, connStats.loss * 500 + connStats.rtt * 40);
      pushHist(hist.net, netLoad);
      if (tm.sideCpu) tm.sideCpu.textContent = `${Math.round(m.cpu || 0)}%`;
      if (tm.sideMem) tm.sideMem.textContent = `${Math.round(m.memPct || 0)}%`;
      if (tm.sideDisk) tm.sideDisk.textContent = `${Math.round(diskUsedPct)}%`;
      if (tm.sideNet) tm.sideNet.textContent = `${(connStats.rtt * 1000).toFixed(0)} ms`;
      drawSpark(tm.sparkCpu, hist.cpu, '#0078d4');
      drawSpark(tm.sparkMem, hist.mem, '#8764b8');
      drawSpark(tm.sparkDisk, hist.disk, '#00a86b');
      drawSpark(tm.sparkNet, hist.net, '#e67e22');
      renderPerfDetail(m);
      renderProcessTable(m);
    }

    function renderConnStats(rtt, loss, frames) {
      connStats = { rtt, loss, frames };
      const line = `rtt ${(rtt * 1000).toFixed(0)} ms · loss ${(loss * 100).toFixed(1)}% · decoded ${frames}`;
      if (statsEl) statsEl.textContent = line;
      if (lastHost) renderHostStats(lastHost);
    }

    function setTmView(view) {
      tmView = view;
      const titles = { performance: 'Task Manager', processes: 'Task Manager', control: 'Control Panel' };
      document.querySelectorAll('.tm-nav-item').forEach((el) => {
        el.classList.toggle('active', el.dataset.view === view);
      });
      if (tm.viewPerf) tm.viewPerf.classList.toggle('active', view === 'performance');
      if (tm.viewProc) tm.viewProc.classList.toggle('active', view === 'processes');
      if (tm.viewControl) tm.viewControl.classList.toggle('active', view === 'control');
      if (tm.panelTitle) tm.panelTitle.textContent = titles[view] || 'Remote Host';
      if (view === 'control' && onControlView) onControlView();
    }

    function setTmMetric(metric) {
      tmMetric = metric;
      document.querySelectorAll('.tm-perf-item').forEach((el) => {
        el.classList.toggle('active', el.dataset.metric === metric);
      });
      if (lastHost) renderPerfDetail(lastHost);
    }

    document.querySelectorAll('.tm-nav-item').forEach((el) => {
      el.addEventListener('click', () => setTmView(el.dataset.view));
    });
    document.querySelectorAll('.tm-perf-item').forEach((el) => {
      el.addEventListener('click', () => setTmMetric(el.dataset.metric));
    });
    document.querySelectorAll('.tm-table th[data-sort]').forEach((th) => {
      th.addEventListener('click', () => {
        const col = th.dataset.sort;
        if (procSort.col === col) procSort.asc = !procSort.asc;
        else { procSort.col = col; procSort.asc = col === 'name'; }
        document.querySelectorAll('.tm-table th').forEach((h) => h.classList.toggle('sorted', h === th));
        if (lastHost) renderProcessTable(lastHost);
      });
    });
    if (tm.procFilter) {
      tm.procFilter.addEventListener('input', () => {
        if (lastHost) renderProcessTable(lastHost);
      });
    }

    return {
      renderHostStats,
      renderConnStats,
      setOnControlView(fn) { onControlView = fn; },
    };
  },
};
