(() => {
  function initialize() {
    const sidebar = document.getElementById("sidebar");
    const toggle = document.getElementById("sidebar-toggle");
    if (!sidebar || !toggle || sidebar.dataset.ready === "true") return;
    sidebar.dataset.ready = "true";

    toggle.addEventListener("click", () => {
      const collapsed = sidebar.classList.toggle("is-collapsed");
      toggle.setAttribute("aria-expanded", String(!collapsed));
      toggle.setAttribute("aria-label", collapsed ? "展开侧栏" : "收起侧栏");
    });
  }

  document.addEventListener("DOMContentLoaded", initialize);
  initialize();
})();
