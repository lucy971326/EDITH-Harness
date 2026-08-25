(() => {
  const storedTheme = localStorage.getItem("edith-theme");
  if (storedTheme === "dark") document.body.dataset.theme = "dark";

  const storedSidebarState = localStorage.getItem("edith-sidebar");
  if (storedSidebarState === "collapsed") document.body.dataset.sidebar = "collapsed";

  let cancelRequestXHR = null;

  function isCancelRequest(detail) {
    const requestConfig = detail && detail.requestConfig;
    const element = detail && detail.elt;
    const path = requestConfig && requestConfig.path || element && element.getAttribute("hx-post");
    return typeof path === "string" && path.includes("/sessions/cancel");
  }

  function isCancelResponse(detail) {
    return isCancelRequest(detail) || cancelRequestXHR && detail && detail.xhr === cancelRequestXHR;
  }

  function finishCancelRequest() {
    cancelRequestXHR = null;
    delete document.body.dataset.canceling;
  }

  function syncSidebarToggle() {
    const collapsed = document.body.dataset.sidebar === "collapsed";
    document.querySelectorAll("[data-sidebar-toggle]").forEach((toggle) => {
      toggle.setAttribute("aria-expanded", collapsed ? "false" : "true");
      toggle.setAttribute("aria-label", collapsed ? "展开侧边栏" : "收起侧边栏");
      toggle.textContent = collapsed ? "›" : "‹";
    });
  }

  function resizeComposer(textarea) {
    if (!textarea) return;
    textarea.style.height = "auto";
    textarea.style.height = `${Math.min(textarea.scrollHeight, 180)}px`;
  }

  function bindComposer() {
    document.querySelectorAll("[data-autogrow]").forEach((textarea) => {
      resizeComposer(textarea);
      textarea.addEventListener("input", () => resizeComposer(textarea));
    });
  }

  function readCollapsedProjects() {
    try {
      const value = JSON.parse(localStorage.getItem("edith-collapsed-projects") || "[]");
      return new Set(Array.isArray(value) ? value : []);
    } catch {
      return new Set();
    }
  }

  function saveCollapsedProjects(projects) {
    localStorage.setItem("edith-collapsed-projects", JSON.stringify([...projects]));
  }

  function syncProjectBranches() {
    const collapsedProjects = readCollapsedProjects();
    document.querySelectorAll("[data-project-branch]").forEach((branch) => {
      const collapsed = collapsedProjects.has(branch.dataset.projectId || "");
      branch.dataset.collapsed = collapsed ? "true" : "false";
      const toggle = branch.querySelector("[data-project-toggle]");
      if (toggle) toggle.setAttribute("aria-expanded", collapsed ? "false" : "true");
    });
  }

  syncSidebarToggle();
  bindComposer();
  syncProjectBranches();

  document.addEventListener("click", (event) => {
    if (!event.target.closest("[data-theme-toggle]")) return;
    const next = document.body.dataset.theme === "dark" ? "light" : "dark";
    document.body.dataset.theme = next;
    localStorage.setItem("edith-theme", next);
  });

  function closePicker(picker) {
    if (!picker) return;
    const menu = picker.querySelector(".model-picker-menu");
    const toggle = picker.querySelector("[data-model-picker-toggle]");
    if (menu) menu.hidden = true;
    if (toggle) toggle.setAttribute("aria-expanded", "false");
    setPickerPane(picker, "root");
  }

  function setPickerPane(picker, paneName) {
    if (!picker) return;
    picker.querySelectorAll("[data-picker-pane]").forEach((pane) => {
      pane.hidden = pane.dataset.pickerPane !== paneName;
    });
  }

  function openPicker(picker) {
    document.querySelectorAll(".model-picker").forEach((item) => {
      if (item !== picker) closePicker(item);
    });
    const menu = picker.querySelector(".model-picker-menu");
    const toggle = picker.querySelector("[data-model-picker-toggle]");
    if (menu) menu.hidden = false;
    if (toggle) toggle.setAttribute("aria-expanded", "true");
    setPickerPane(picker, "root");
  }

  function commitModelState() {
    const state = document.getElementById("model-state");
    if (state) htmx.trigger(state, "model-change");
  }

  function setHiddenModelState(picker) {
    const state = document.getElementById("model-state");
    if (!state || !picker) return;
    const model = state.querySelector('input[name="model_id"]');
    const thinking = state.querySelector('input[name="thinking"]');
    if (model) {
      const provider = picker.dataset.selectedProvider || "";
      const modelName = picker.dataset.selectedModel || "";
      model.value = provider && modelName ? `${provider}\u001f${modelName}` : "";
    }
    if (thinking) thinking.value = picker.dataset.selectedThinking || "";
  }

  document.addEventListener("click", (event) => {
    const target = event.target instanceof Element ? event.target : null;
    if (!target) return;

    const cancelButton = target.closest("[data-cancel-session]");
    if (cancelButton) {
      document.body.dataset.canceling = "true";
      cancelButton.disabled = true;
      return;
    }

    const sidebarToggle = target.closest("[data-sidebar-toggle]");
    if (sidebarToggle) {
      const collapsed = document.body.dataset.sidebar === "collapsed";
      document.body.dataset.sidebar = collapsed ? "expanded" : "collapsed";
      localStorage.setItem("edith-sidebar", collapsed ? "expanded" : "collapsed");
      syncSidebarToggle();
      return;
    }

    const projectToggle = target.closest("[data-project-toggle]");
    if (projectToggle) {
      const branch = projectToggle.closest("[data-project-branch]");
      const projectID = branch && branch.dataset.projectId;
      if (branch && projectID && projectID === document.body.dataset.project) {
        event.preventDefault();
        const collapsedProjects = readCollapsedProjects();
        if (collapsedProjects.has(projectID)) collapsedProjects.delete(projectID);
        else collapsedProjects.add(projectID);
        saveCollapsedProjects(collapsedProjects);
        syncProjectBranches();
      }
      return;
    }

    const picker = target.closest(".model-picker");
    if (!picker) {
      document.querySelectorAll(".model-picker").forEach(closePicker);
      return;
    }

    const toggle = target.closest("[data-model-picker-toggle]");
    if (toggle) {
      const menu = picker.querySelector(".model-picker-menu");
      if (menu && menu.hidden) openPicker(picker);
      else closePicker(picker);
      return;
    }

    const paneButton = target.closest("[data-picker-pane-button]");
    if (paneButton) {
      setPickerPane(picker, paneButton.dataset.pickerPaneButton);
      return;
    }

    if (target.closest("[data-picker-back]")) {
      setPickerPane(picker, "root");
      return;
    }

    const modelOption = target.closest("[data-model-option]");
    if (modelOption) {
      const state = document.getElementById("model-state");
      const model = state && state.querySelector('input[name="model_id"]');
      if (model) {
        const provider = modelOption.dataset.modelProvider || "";
        const modelName = modelOption.dataset.modelName || "";
        model.value = provider && modelName ? `${provider}\u001f${modelName}` : "";
      }
      closePicker(picker);
      commitModelState();
      return;
    }

    const thinkingOption = target.closest("[data-thinking-option]");
    if (thinkingOption) {
      const state = document.getElementById("model-state");
      const thinking = state && state.querySelector('input[name="thinking"]');
      if (thinking) thinking.value = thinkingOption.dataset.thinking || "";
      closePicker(picker);
      commitModelState();
    }
  });

  document.addEventListener("keydown", (event) => {
    if (event.key !== "Escape") return;
    document.querySelectorAll(".model-picker").forEach(closePicker);
  });

  document.body.addEventListener("htmx:beforeRequest", (event) => {
    const detail = event.detail || {};
    if (isCancelRequest(detail)) {
      cancelRequestXHR = detail.xhr || null;
      return;
    }
    if (document.body.dataset.canceling === "true" && detail.target && detail.target.id === "composer") {
      event.preventDefault();
    }
  });

  document.body.addEventListener("htmx:beforeSwap", (event) => {
    if (document.body.dataset.canceling !== "true") return;
    const detail = event.detail || {};
    const target = detail.target;
    if (!target || target.id !== "composer" || isCancelResponse(detail)) return;
    event.preventDefault();
  });

  document.body.addEventListener("htmx:afterSwap", (event) => {
    const detail = event.detail || {};
    const target = detail.target;
    bindComposer();
    if (target && target.id === "composer" && isCancelResponse(detail)) {
      finishCancelRequest();
    }
    if (!target || target.id !== "model-picker") return;
    const picker = document.getElementById("model-picker");
    setHiddenModelState(picker);
    closePicker(picker);
  });

  document.body.addEventListener("htmx:afterRequest", (event) => {
    const detail = event.detail || {};
    if (!cancelRequestXHR || detail.xhr !== cancelRequestXHR || detail.successful) return;
    finishCancelRequest();
    const button = document.querySelector("[data-cancel-session]");
    if (button) button.disabled = false;
  });

  const sessionID = document.body.dataset.session;
  const projectID = document.body.dataset.project;
  if (!sessionID || !projectID) return;
  let stream = null;
  const connectLiveRefresh = () => {
    if (stream) return;
    stream = new EventSource(`/events?session=${encodeURIComponent(sessionID)}`);
    stream.addEventListener("refresh", () => {
      htmx.ajax("GET", `/fragments/chat-log?project=${encodeURIComponent(projectID)}&session=${encodeURIComponent(sessionID)}`, {target: "#chat-log", swap: "outerHTML"});
    });
    stream.addEventListener("composer", () => {
      if (document.body.dataset.canceling === "true") return;
      htmx.ajax("GET", `/fragments/composer?project=${encodeURIComponent(projectID)}&session=${encodeURIComponent(sessionID)}`, {target: "#composer", swap: "outerHTML"});
    });
  };
  connectLiveRefresh();
})();
