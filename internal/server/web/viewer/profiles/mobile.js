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

    // On-screen keys are provided by the floating keyboard (Connect.floatkb),
    // which works on every viewport and is draggable/closable.

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
