(() => {
  function key(tab) {
    return `${tab.type}:${tab.instanceKey}`;
  }

  function createState() {
    return { tabs: [], activeKey: "" };
  }

  function openTab(state, tab) {
    const tabKey = key(tab);
    if (!state.tabs.some((item) => key(item) === tabKey)) state.tabs.push(tab);
    state.activeKey = tabKey;
    return state;
  }

  function closeTab(state, tabKey) {
    const index = state.tabs.findIndex((tab) => key(tab) === tabKey);
    if (index < 0) return state;
    state.tabs.splice(index, 1);
    if (state.activeKey === tabKey) {
      const next = state.tabs[index] || state.tabs[index - 1];
      state.activeKey = next ? key(next) : "";
    }
    return state;
  }

  const stateAPI = { key, createState, openTab, closeTab };
  if (typeof module !== "undefined") module.exports = stateAPI;
  if (typeof window === "undefined") return;
  window.HarnessPanelState = stateAPI;

  function initialize() {
    const page = document.getElementById("chat-page");
    const panel = document.getElementById("sidepanel");
    const toggle = document.getElementById("panel-toggle");
    if (!page || !panel || !toggle || page.dataset.panelReady === "true") return;
    page.dataset.panelReady = "true";

    const tabs = document.getElementById("panel-tabs");
    const content = document.getElementById("panel-content");
    const add = document.getElementById("panel-add");
    const menu = document.getElementById("panel-menu");
    const close = document.getElementById("panel-close");
    const resize = document.getElementById("panel-resize-handle");
    const state = createState();
    let dragging = false;

    function activeTab() {
      return state.tabs.find((tab) => key(tab) === state.activeKey);
    }

    async function loadActivePanel() {
      const tab = activeTab();
      if (!tab) {
        content.innerHTML = '<div class="grid h-full place-items-center p-6 text-center text-sm text-[var(--color-subtle)]">从 + 打开一个面板</div>';
        return;
      }
      content.innerHTML = '<div class="p-6 text-sm text-[var(--color-subtle)]">加载中…</div>';
      const url = `${panel.dataset.panelUrl}${encodeURIComponent(tab.type)}?instance=${encodeURIComponent(tab.instanceKey)}`;
      try {
        const response = await fetch(url);
        if (!response.ok) throw new Error(await response.text());
        content.innerHTML = await response.text();
      } catch (error) {
        content.textContent = error.message || "面板加载失败";
      }
    }

    function renderTabs() {
      tabs.replaceChildren(...state.tabs.map((tab) => {
        const tabKey = key(tab);
        const button = document.createElement("button");
        button.type = "button";
        button.className = "max-w-36 truncate rounded px-2 py-1.5 text-sm hover:bg-[var(--color-muted)]";
        if (tabKey === state.activeKey) button.classList.add("bg-[var(--color-muted)]", "font-medium");
        button.textContent = tab.title;
        button.addEventListener("click", () => {
          state.activeKey = tabKey;
          renderTabs();
          loadActivePanel();
        });
        return button;
      }));
    }

    function open(tab) {
      openTab(state, tab);
      panel.hidden = false;
      toggle.setAttribute("aria-expanded", "true");
      renderTabs();
      loadActivePanel();
    }

    toggle.addEventListener("click", () => {
      panel.hidden = !panel.hidden;
      toggle.setAttribute("aria-expanded", String(!panel.hidden));
    });
    close.addEventListener("click", () => {
      panel.hidden = true;
      toggle.setAttribute("aria-expanded", "false");
    });
    add.addEventListener("click", () => {
      menu.hidden = !menu.hidden;
      add.setAttribute("aria-expanded", String(!menu.hidden));
    });
    for (const item of menu.querySelectorAll(".panel-menu-item")) {
      item.addEventListener("click", () => {
        menu.hidden = true;
        add.setAttribute("aria-expanded", "false");
        open({ type: item.dataset.panelType, instanceKey: item.dataset.panelInstance, title: item.dataset.panelTitle });
      });
    }
    resize.addEventListener("pointerdown", (event) => {
      dragging = true;
      resize.setPointerCapture(event.pointerId);
    });
    resize.addEventListener("pointermove", (event) => {
      if (!dragging) return;
      const bounds = page.getBoundingClientRect();
      const width = Math.max(320, Math.min(720, bounds.right - event.clientX));
      panel.style.width = `${width}px`;
    });
    resize.addEventListener("pointerup", () => { dragging = false; });
  }

  document.addEventListener("DOMContentLoaded", initialize);
  document.addEventListener("htmx:afterSettle", initialize);
  initialize();
})();
