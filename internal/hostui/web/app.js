(function () {
  const toast = document.getElementById('toast');
  let toastTimer = null;

  function showToast(msg) {
    if (!toast) return;
    toast.hidden = false;
    toast.textContent = msg;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => {
      toast.hidden = true;
    }, 2200);
  }

  async function stub(name) {
    if (name === 'hide') {
      if (typeof window.hostHide === 'function') {
        try {
          await window.hostHide();
          return;
        } catch (_) {}
      }
      showToast('Close the window with X to hide');
      return;
    }
    if (typeof window.hostStub === 'function') {
      try {
        await window.hostStub(name);
      } catch (_) {}
    }
    showToast('Coming soon');
  }

  document.addEventListener('click', (ev) => {
    const btn = ev.target.closest('[data-stub]');
    if (!btn || btn.disabled) return;
    stub(btn.getAttribute('data-stub') || 'action');
  });

  const hideBtn = document.getElementById('btn-hide');
  if (hideBtn) {
    hideBtn.addEventListener('click', function () {
      stub('hide');
    });
  }
})();
