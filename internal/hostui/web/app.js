(function () {
  const toast = document.getElementById('toast');
  const viewUnlock = document.getElementById('view-unlock');
  const viewHome = document.getElementById('view-home');
  const viewMeet = document.getElementById('view-meet');
  const timerEl = document.getElementById('meet-timer');
  const hostName = document.getElementById('host-name');
  const unlockHost = document.getElementById('unlock-host');
  const unlockErr = document.getElementById('unlock-err');
  const unlockKey = document.getElementById('unlock-key');
  let toastTimer = null;
  let meetSeconds = 0;
  let meetInterval = null;
  let muted = false;
  let camOff = false;
  let unlocked = false;

  function showToast(msg) {
    if (!toast) return;
    toast.hidden = false;
    toast.textContent = msg;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => {
      toast.hidden = true;
    }, 2200);
  }

  function fmtTime(s) {
    const m = Math.floor(s / 60);
    const r = s % 60;
    return String(m).padStart(2, '0') + ':' + String(r).padStart(2, '0');
  }

  function showUnlock() {
    unlocked = false;
    if (viewUnlock) viewUnlock.hidden = false;
    if (viewHome) viewHome.hidden = true;
    if (viewMeet) viewMeet.hidden = true;
    if (unlockKey) {
      unlockKey.value = '';
      setTimeout(() => unlockKey.focus(), 50);
    }
    if (unlockErr) unlockErr.hidden = true;
  }

  function showHome() {
    unlocked = true;
    if (viewUnlock) viewUnlock.hidden = true;
    if (viewHome) viewHome.hidden = false;
    if (viewMeet) viewMeet.hidden = true;
  }

  function openMeet() {
    if (!unlocked) return;
    viewHome.hidden = true;
    viewMeet.hidden = false;
    meetSeconds = 0;
    if (timerEl) timerEl.textContent = '00:00';
    clearInterval(meetInterval);
    meetInterval = setInterval(() => {
      meetSeconds += 1;
      if (timerEl) timerEl.textContent = fmtTime(meetSeconds);
    }, 1000);
  }

  function leaveMeet() {
    clearInterval(meetInterval);
    meetInterval = null;
    viewMeet.hidden = true;
    viewHome.hidden = false;
    muted = false;
    camOff = false;
    syncCtrl();
  }

  function syncCtrl() {
    const muteBtn = document.getElementById('btn-mute');
    const camBtn = document.getElementById('btn-cam');
    if (muteBtn) {
      muteBtn.classList.toggle('off', muted);
      muteBtn.querySelector('span:last-child').textContent = muted ? 'Unmute' : 'Mute';
      muteBtn.querySelector('.ctrl-ico').textContent = muted ? '🔇' : '🎙';
    }
    if (camBtn) {
      camBtn.classList.toggle('off', camOff);
      camBtn.querySelector('span:last-child').textContent = camOff ? 'Start video' : 'Stop video';
    }
  }

  async function hideWindow() {
    if (typeof window.hostHide === 'function') {
      try {
        await window.hostHide();
        return;
      } catch (_) {}
    }
    showToast('Close the window with X to hide');
  }

  async function bootUnlock() {
    let status = { unlocked: false, hostname: '', deviceId: '' };
    if (typeof window.hostUnlockStatus === 'function') {
      try {
        status = await window.hostUnlockStatus();
      } catch (_) {}
    }
    const name = status.hostname || (navigator.platform || 'This PC');
    if (hostName) hostName.textContent = name;
    if (unlockHost) unlockHost.textContent = name;
    if (status.unlocked) {
      showHome();
    } else {
      showUnlock();
    }
  }

  document.getElementById('unlock-form').addEventListener('submit', async (ev) => {
    ev.preventDefault();
    const btn = document.getElementById('btn-unlock');
    const key = (unlockKey && unlockKey.value) || '';
    if (unlockErr) unlockErr.hidden = true;
    if (btn) btn.disabled = true;
    try {
      if (typeof window.hostUnlock !== 'function') {
        throw new Error('Unlock is unavailable in this build');
      }
      const res = await window.hostUnlock(key);
      if (res && res.ok === false) {
        throw new Error(res.error || 'Unlock failed');
      }
      showHome();
      showToast('Host app unlocked');
    } catch (e) {
      if (unlockErr) {
        unlockErr.textContent = (e && e.message) || String(e);
        unlockErr.hidden = false;
      }
    } finally {
      if (btn) btn.disabled = false;
    }
  });

  document.getElementById('btn-lock').addEventListener('click', async () => {
    if (typeof window.hostLock === 'function') {
      try { await window.hostLock(); } catch (_) {}
    }
    leaveMeet();
    showUnlock();
    showToast('Host app locked');
  });

  document.getElementById('btn-connect').addEventListener('click', openMeet);
  document.getElementById('btn-leave').addEventListener('click', leaveMeet);
  document.getElementById('btn-leave-top').addEventListener('click', leaveMeet);
  document.getElementById('btn-hide').addEventListener('click', hideWindow);

  document.getElementById('btn-mute').addEventListener('click', () => {
    muted = !muted;
    syncCtrl();
    showToast(muted ? 'Muted (preview)' : 'Unmuted (preview)');
  });
  document.getElementById('btn-cam').addEventListener('click', () => {
    camOff = !camOff;
    syncCtrl();
    showToast(camOff ? 'Camera off (preview)' : 'Camera on (preview)');
  });

  document.addEventListener('click', (ev) => {
    const btn = ev.target.closest('[data-stub]');
    if (!btn || btn.disabled) return;
    const name = btn.getAttribute('data-stub') || 'action';
    if (typeof window.hostStub === 'function') {
      try { window.hostStub(name); } catch (_) {}
    }
    showToast('Coming soon');
  });

  bootUnlock();
})();
