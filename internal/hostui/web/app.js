(function () {
  const toast = document.getElementById('toast');
  const viewHome = document.getElementById('view-home');
  const viewMeet = document.getElementById('view-meet');
  const timerEl = document.getElementById('meet-timer');
  const hostName = document.getElementById('host-name');
  let toastTimer = null;
  let meetSeconds = 0;
  let meetInterval = null;
  let muted = false;
  let camOff = false;

  try {
    hostName.textContent = (window.navigator && navigator.platform) ? navigator.platform : 'This PC';
  } catch (_) {}

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

  function openMeet() {
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
})();
