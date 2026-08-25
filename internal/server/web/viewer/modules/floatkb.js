window.Connect = window.Connect || {};

// Floating on-screen keyboard: draggable, closable, works on every viewport.
// Provides sticky modifiers, nav/function keys, and a text field that forwards
// real typing to the host (native soft keyboard friendly).
Connect.floatkb = {
  create(getDC) {
    const enc = Connect.input;
    if (!enc || !enc.encKey) return null;

    const VK = {
      ctrl: 0x11, alt: 0x12, shift: 0x10, win: 0x5b,
      esc: 0x1b, tab: 0x09, enter: 0x0d, back: 0x08, del: 0x2e,
      left: 0x25, up: 0x26, right: 0x27, down: 0x28,
      home: 0x24, end: 0x23, pgup: 0x21, pgdn: 0x22,
    };

    // armed = apply to next key then clear; locked = apply until toggled off.
    const mod = {
      ctrl: { armed: false, locked: false },
      alt: { armed: false, locked: false },
      shift: { armed: false, locked: false },
      win: { armed: false, locked: false },
    };

    function dcOpen() {
      const dc = getDC && getDC();
      return dc && dc.readyState === 'open' ? dc : null;
    }
    function sendBin(bytes) {
      const dc = dcOpen();
      if (dc && bytes) dc.send(bytes);
    }
    function activeModVks() {
      const out = [];
      for (const name of ['ctrl', 'alt', 'shift', 'win']) {
        if (mod[name].armed || mod[name].locked) out.push(VK[name]);
      }
      return out;
    }
    function clearArmed() {
      let changed = false;
      for (const name of ['ctrl', 'alt', 'shift', 'win']) {
        if (mod[name].armed) { mod[name].armed = false; changed = true; }
      }
      if (changed) refreshModUI();
    }
    // Wrap the key with any active modifiers (down…up) so nothing stays held.
    function pressVK(vk) {
      const mods = activeModVks();
      for (const m of mods) sendBin(enc.encKey(true, m));
      sendBin(enc.encKey(true, vk));
      sendBin(enc.encKey(false, vk));
      for (let i = mods.length - 1; i >= 0; i--) sendBin(enc.encKey(false, mods[i]));
      clearArmed();
    }
    function sendText(s) {
      if (!s) return;
      const b = enc.encText ? enc.encText(s) : null;
      if (b) sendBin(b);
    }
    function charToVK(ch) {
      if (!ch) return 0;
      const code = ch[0].toUpperCase().charCodeAt(0);
      if ((code >= 65 && code <= 90) || (code >= 48 && code <= 57)) return code;
      return 0;
    }

    // ---- DOM ----
    const wrap = document.createElement('div');
    wrap.className = 'fkb';
    wrap.hidden = true;
    wrap.setAttribute('role', 'group');
    wrap.setAttribute('aria-label', 'On-screen keyboard');

    const header = document.createElement('div');
    header.className = 'fkb-header';
    header.innerHTML = '<span class="fkb-grip" aria-hidden="true">⋮⋮</span><span class="fkb-title">Keyboard</span>';
    const btnClose = document.createElement('button');
    btnClose.type = 'button';
    btnClose.className = 'fkb-close';
    btnClose.setAttribute('aria-label', 'Close keyboard');
    btnClose.textContent = '×';
    header.appendChild(btnClose);
    wrap.appendChild(header);

    const typeRow = document.createElement('div');
    typeRow.className = 'fkb-type';
    const field = document.createElement('input');
    field.type = 'text';
    field.className = 'fkb-field';
    field.setAttribute('autocomplete', 'off');
    field.setAttribute('autocapitalize', 'off');
    field.setAttribute('autocorrect', 'off');
    field.setAttribute('spellcheck', 'false');
    field.setAttribute('enterkeyhint', 'enter');
    field.placeholder = 'Type to host…';
    typeRow.appendChild(field);
    wrap.appendChild(typeRow);

    const keysWrap = document.createElement('div');
    keysWrap.className = 'fkb-keys';
    wrap.appendChild(keysWrap);

    const fnWrap = document.createElement('div');
    fnWrap.className = 'fkb-fn';
    fnWrap.hidden = true;
    wrap.appendChild(fnWrap);

    // Rows definition: mod = sticky modifier, vk = one-shot key, act = special.
    const rows = [
      [
        { label: 'Ctrl', mod: 'ctrl' },
        { label: 'Alt', mod: 'alt' },
        { label: 'Shift', mod: 'shift' },
        { label: 'Win', mod: 'win' },
        { label: 'Fn', act: 'fn' },
      ],
      [
        { label: 'Esc', vk: VK.esc },
        { label: 'Tab', vk: VK.tab },
        { label: 'Home', vk: VK.home },
        { label: 'End', vk: VK.end },
        { label: 'PgUp', vk: VK.pgup },
        { label: 'PgDn', vk: VK.pgdn },
      ],
      [
        { label: '⌫', vk: VK.back, wide: true },
        { label: 'Del', vk: VK.del },
        { label: '←', vk: VK.left },
        { label: '↑', vk: VK.up },
        { label: '↓', vk: VK.down },
        { label: '→', vk: VK.right },
        { label: 'Enter', vk: VK.enter, wide: true },
      ],
    ];

    const modBtns = {};
    function makeKeyBtn(def) {
      const b = document.createElement('button');
      b.type = 'button';
      b.className = 'fkb-key';
      if (def.wide) b.classList.add('wide');
      b.textContent = def.label;
      // Don't steal focus from the type field / video overlay.
      b.addEventListener('pointerdown', (e) => e.preventDefault());
      if (def.mod) {
        b.classList.add('fkb-mod');
        modBtns[def.mod] = b;
        let lastTap = 0;
        b.addEventListener('click', () => {
          const now = Date.now();
          const m = mod[def.mod];
          if (now - lastTap < 320) {
            // double tap → lock
            m.locked = !m.locked;
            m.armed = false;
          } else {
            if (m.locked) { m.locked = false; m.armed = false; }
            else m.armed = !m.armed;
          }
          lastTap = now;
          refreshModUI();
        });
      } else if (def.act === 'fn') {
        b.addEventListener('click', () => {
          fnWrap.hidden = !fnWrap.hidden;
          b.classList.toggle('on', !fnWrap.hidden);
        });
      } else if (def.vk) {
        b.addEventListener('click', () => pressVK(def.vk));
      }
      return b;
    }

    for (const row of rows) {
      const r = document.createElement('div');
      r.className = 'fkb-row';
      for (const def of row) r.appendChild(makeKeyBtn(def));
      keysWrap.appendChild(r);
    }
    for (let i = 1; i <= 12; i++) {
      const b = document.createElement('button');
      b.type = 'button';
      b.className = 'fkb-key fkb-fnkey';
      b.textContent = 'F' + i;
      b.addEventListener('pointerdown', (e) => e.preventDefault());
      b.addEventListener('click', () => pressVK(0x70 + (i - 1)));
      fnWrap.appendChild(b);
    }

    function refreshModUI() {
      for (const name of ['ctrl', 'alt', 'shift', 'win']) {
        const b = modBtns[name];
        if (!b) continue;
        b.classList.toggle('armed', mod[name].armed);
        b.classList.toggle('locked', mod[name].locked);
        b.setAttribute('aria-pressed', (mod[name].armed || mod[name].locked) ? 'true' : 'false');
      }
    }

    // ---- text field: a compose box that mirrors typing to the host ----
    // You SEE what you type here; each change is forwarded to the remote PC.
    let prev = '';
    let composing = false;

    function flushDiff() {
      const val = field.value;
      if (val === prev) return;
      if (val.length > prev.length && val.startsWith(prev)) {
        sendText(val.slice(prev.length));
      } else if (val.length < prev.length && prev.startsWith(val)) {
        for (let i = 0; i < prev.length - val.length; i++) pressVK(VK.back);
      } else {
        // Non-linear edit (autocorrect, mid-line): re-sync best effort.
        for (let i = 0; i < prev.length; i++) pressVK(VK.back);
        if (val) sendText(val);
      }
      prev = val;
    }

    // Modifier + letter (e.g. armed Ctrl + "c") = real VK combo, not text.
    field.addEventListener('beforeinput', (e) => {
      if ((e.inputType || '') === 'insertText' && e.data) {
        const vk = charToVK(e.data);
        if (activeModVks().length && vk) {
          e.preventDefault();
          pressVK(vk); // clears armed mods
        }
      }
    });
    field.addEventListener('compositionstart', () => { composing = true; });
    field.addEventListener('compositionend', () => { composing = false; flushDiff(); });
    field.addEventListener('input', () => { if (!composing) flushDiff(); });
    field.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        pressVK(VK.enter);
        field.value = '';
        prev = '';
      } else if (e.key === 'Backspace' && field.value === '') {
        // Let backspace reach the host even when the compose box is empty.
        e.preventDefault();
        pressVK(VK.back);
      }
    });

    const btnClear = document.createElement('button');
    btnClear.type = 'button';
    btnClear.className = 'fkb-clear';
    btnClear.title = 'Clear box (does not affect host)';
    btnClear.textContent = 'Clear';
    btnClear.addEventListener('pointerdown', (e) => e.preventDefault());
    btnClear.addEventListener('click', () => { field.value = ''; prev = ''; try { field.focus(); } catch (_) {} });
    typeRow.appendChild(btnClear);

    // ---- FAB toggle ----
    const fab = document.createElement('button');
    fab.type = 'button';
    fab.className = 'fkb-fab';
    fab.title = 'Show keyboard';
    fab.setAttribute('aria-label', 'Show keyboard');
    fab.textContent = '⌨';

    document.body.appendChild(wrap);
    document.body.appendChild(fab);

    // ---- position + drag ----
    function clampPos(left, top) {
      const w = wrap.offsetWidth || 280;
      const h = wrap.offsetHeight || 200;
      const maxL = Math.max(4, window.innerWidth - w - 4);
      const maxT = Math.max(4, window.innerHeight - h - 4);
      return [Math.min(Math.max(4, left), maxL), Math.min(Math.max(4, top), maxT)];
    }
    function applyPos(left, top) {
      const [l, t] = clampPos(left, top);
      wrap.style.left = l + 'px';
      wrap.style.top = t + 'px';
      wrap.style.right = 'auto';
      wrap.style.bottom = 'auto';
    }
    function savePos() {
      try {
        localStorage.setItem('fkb.pos', JSON.stringify({
          left: parseInt(wrap.style.left, 10) || 0,
          top: parseInt(wrap.style.top, 10) || 0,
        }));
      } catch (_) {}
    }
    function restorePos() {
      let p = null;
      try { p = JSON.parse(localStorage.getItem('fkb.pos') || 'null'); } catch (_) {}
      if (p && typeof p.left === 'number') applyPos(p.left, p.top);
      else {
        // default: bottom-center
        const w = wrap.offsetWidth || 300;
        applyPos((window.innerWidth - w) / 2, window.innerHeight - (wrap.offsetHeight || 220) - 16);
      }
    }

    let drag = null;
    header.addEventListener('pointerdown', (e) => {
      if (e.target === btnClose) return;
      drag = {
        dx: e.clientX - wrap.offsetLeft,
        dy: e.clientY - wrap.offsetTop,
      };
      try { header.setPointerCapture(e.pointerId); } catch (_) {}
      e.preventDefault();
    });
    header.addEventListener('pointermove', (e) => {
      if (!drag) return;
      applyPos(e.clientX - drag.dx, e.clientY - drag.dy);
    });
    const endDrag = (e) => {
      if (!drag) return;
      drag = null;
      try { header.releasePointerCapture(e.pointerId); } catch (_) {}
      savePos();
    };
    header.addEventListener('pointerup', endDrag);
    header.addEventListener('pointercancel', endDrag);

    window.addEventListener('resize', () => {
      if (wrap.hidden) return;
      applyPos(wrap.offsetLeft, wrap.offsetTop);
    });

    // ---- open / close ----
    function open() {
      wrap.hidden = false;
      fab.classList.add('hidden');
      restorePos();
      field.value = '';
      prev = '';
      try { field.focus({ preventScroll: true }); } catch (_) { field.focus(); }
      try { localStorage.setItem('fkb.open', '1'); } catch (_) {}
    }
    function close() {
      wrap.hidden = true;
      fab.classList.remove('hidden');
      // release any locked modifiers so the host isn't left with a stuck key
      for (const name of ['ctrl', 'alt', 'shift', 'win']) { mod[name].armed = false; mod[name].locked = false; }
      refreshModUI();
      try { localStorage.setItem('fkb.open', '0'); } catch (_) {}
    }
    fab.addEventListener('click', open);
    btnClose.addEventListener('click', close);

    let wasOpen = '0';
    try { wasOpen = localStorage.getItem('fkb.open') || '0'; } catch (_) {}
    if (wasOpen === '1') open();

    return { open, close, toggle() { wrap.hidden ? open() : close(); } };
  },
};
