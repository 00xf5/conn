window.Connect = window.Connect || {};
Connect.profiles = Connect.profiles || {};

Connect.profiles.mobile = {
  isActive() {
    const q = new URLSearchParams(location.search);
    if (q.get("profile") === "desktop") return false;
    if (q.get("profile") === "mobile") return true;
    return (
      /Android|iPhone|iPad|iPod|Mobile/i.test(navigator.userAgent) ||
      window.innerWidth <= 900
    );
  },

  init(ctx, hooks) {
    if (!this.isActive()) return;

    document.body.classList.add("viewer-mobile");
    const panel = document.getElementById("host-panel");
    const getDC = hooks.getDC;

    const backdrop = document.createElement("div");
    backdrop.id = "panel-backdrop";
    backdrop.className = "mobile-backdrop";
    backdrop.hidden = true;
    document.getElementById("app").appendChild(backdrop);

    const btnTools = document.createElement("button");
    btnTools.type = "button";
    btnTools.id = "btn-tools";
    btnTools.textContent = "Tools";
    btnTools.title = "Performance, processes, control";
    const spacer = document.querySelector(".toolbar .spacer");
    if (spacer) spacer.before(btnTools);

    const closeBtn = document.createElement("button");
    closeBtn.type = "button";
    closeBtn.className = "mobile-drawer-close";
    closeBtn.setAttribute("aria-label", "Close tools");
    closeBtn.textContent = "×";
    const titlebar = panel?.querySelector(".tm-titlebar");
    if (titlebar) titlebar.appendChild(closeBtn);

    const keysBar = document.createElement("div");
    keysBar.className = "mobile-keys";
    keysBar.setAttribute("role", "toolbar");
    keysBar.setAttribute("aria-label", "Remote keys");
    const keys = [
      { label: "Ctrl", vk: 0x11 },
      { label: "Alt", vk: 0x12 },
      { label: "Tab", vk: 0x09 },
      { label: "Esc", vk: 0x1b },
      { label: "⌫", vk: 0x08 },
      { label: "Enter", vk: 0x0d },
    ];
    for (const k of keys) {
      const b = document.createElement("button");
      b.type = "button";
      b.textContent = k.label;
      b.addEventListener("click", () => tapKey(k.vk, getDC));
      keysBar.appendChild(b);
    }
    const stats = document.getElementById("stats");
    document.getElementById("app").insertBefore(keysBar, stats);

    function setOpen(open) {
      document.body.classList.toggle("mobile-panel-open", open);
      backdrop.hidden = !open;
      if (panel) panel.setAttribute("aria-hidden", open ? "false" : "true");
    }

    btnTools.addEventListener("click", () => setOpen(true));
    closeBtn.addEventListener("click", () => setOpen(false));
    backdrop.addEventListener("click", () => setOpen(false));

    document.querySelectorAll(".tm-nav-item").forEach((btn) => {
      btn.addEventListener("click", () => {
        if (btn.dataset.view === "control" && hooks.onControlView) hooks.onControlView();
      });
    });
  },
};

function tapKey(vk, getDC) {
  const dc = getDC?.();
  if (!dc || dc.readyState !== "open" || !Connect.input?.encKey) return;
  dc.send(Connect.input.encKey(true, vk));
  setTimeout(() => {
    if (dc.readyState === "open") dc.send(Connect.input.encKey(false, vk));
  }, 80);
}
