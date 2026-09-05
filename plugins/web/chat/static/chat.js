(() => {
  function initialize() {
    const root = document.getElementById("chat-root");
    const composer = document.getElementById("composer");
    const runview = root?.querySelector("[data-runview]");
    if (!root || !composer || !runview || root.dataset.ready === "true") return;
    root.dataset.ready = "true";

    const status = document.getElementById("chat-status");
    const agent = document.getElementById("agent");
    const model = document.getElementById("model");
    const effort = document.getElementById("reasoning-effort");
    const mode = document.getElementById("live-mode");
    const send = document.getElementById("send-button");
    const liveSend = document.getElementById("live-send");
    const stop = document.getElementById("stop-button");
    const textInput = composer.querySelector('textarea[name="text"]');
    const messageSkillTemplate = document.getElementById("message-skill-template");
    const suggestionPopover = document.getElementById("composer-suggestions");
    const suggestionItems = document.getElementById("skill-suggestion-items");
    const suggestionsURL = root.dataset.suggestionsUrl;
    let activeSuggestionIndex = -1;
    let suggestionsRequest = 0;
    let loadedSuggestionPrefix = "/";
    let messageActions = [];
    try {
      messageActions = JSON.parse(root.dataset.messageActions || "[]");
    } catch { /* invalid action definitions leave the action bar empty */ }

    function escapeHTML(value) {
      if (value == null) return "";
      return String(value).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/\"/g, "&quot;");
    }

    function setRunning(value) {
      send.classList.toggle("hidden", value);
      send.disabled = value;
      mode.classList.toggle("hidden", !value);
      mode.disabled = !value;
      liveSend.classList.toggle("hidden", !value);
      liveSend.disabled = !value;
      stop.classList.toggle("hidden", !value);
    }

    function triggerState() {
      const cursor = textInput.selectionStart;
      const beforeCursor = textInput.value.slice(0, cursor);
      const match = /(?:^|\s)([\/$])([^\s]*)$/.exec(beforeCursor);
      if (!match) return null;
      return {
        prefix: match[1],
        query: match[2].toLowerCase(),
        start: cursor - match[2].length - 1,
        end: cursor,
      };
    }

    function visibleSuggestions() {
      return Array.from(suggestionItems.querySelectorAll("button[data-suggestion-name]")).filter((item) => !item.hidden);
    }

    function setActiveSuggestion(index) {
      const items = visibleSuggestions();
      items.forEach((item) => item.setAttribute("aria-selected", "false"));
      if (!items.length) {
        activeSuggestionIndex = -1;
        return;
      }
      activeSuggestionIndex = (index + items.length) % items.length;
      items[activeSuggestionIndex].setAttribute("aria-selected", "true");
    }

    function closeSuggestions() {
      suggestionPopover.hidden = true;
      suggestionPopover.setAttribute("aria-hidden", "true");
      activeSuggestionIndex = -1;
      suggestionItems.querySelectorAll("button[data-suggestion-name]").forEach((item) => item.setAttribute("aria-selected", "false"));
    }

    function paintSuggestions(state) {
      const items = Array.from(suggestionItems.querySelectorAll("button[data-suggestion-name]"));
      if (!items.length) {
        suggestionPopover.hidden = false;
        suggestionPopover.setAttribute("aria-hidden", "false");
        activeSuggestionIndex = -1;
        return;
      }
      items.forEach((item) => {
        const name = item.dataset.suggestionName.toLowerCase();
        item.hidden = !name.includes(state.query);
        item.setAttribute("aria-selected", "false");
      });
      if (!visibleSuggestions().length) {
        closeSuggestions();
        return;
      }
      suggestionPopover.hidden = false;
      suggestionPopover.setAttribute("aria-hidden", "false");
      setActiveSuggestion(0);
    }

    function filterSuggestions() {
      const state = triggerState();
      if (!state) {
        closeSuggestions();
        return;
      }
      if (state.prefix !== loadedSuggestionPrefix) {
        refreshSuggestions(state.prefix);
        return;
      }
      paintSuggestions(state);
    }

    function selectSuggestion(item) {
      const name = item.dataset.suggestionName;
      const state = triggerState();
      if (!state) return;
      textInput.setRangeText(`$${name} `, state.start, state.end, "end");
      closeSuggestions();
      textInput.focus();
    }

    async function refreshSuggestions(prefix = loadedSuggestionPrefix) {
      if (!suggestionsURL || !agent) return;
      const requestID = ++suggestionsRequest;
      const url = new URL(suggestionsURL, window.location.href);
      url.searchParams.set("prefix", prefix);
      url.searchParams.set("agentID", agent.value);
      try {
        const response = await fetch(url);
        if (!response.ok) throw new Error(await response.text());
        const html = await response.text();
        if (requestID !== suggestionsRequest) return;
        suggestionItems.innerHTML = html;
        loadedSuggestionPrefix = prefix;
        const state = triggerState();
        if (state && state.prefix === prefix) paintSuggestions(state);
        else closeSuggestions();
      } catch (error) {
        if (requestID === suggestionsRequest) status.textContent = error.message || "Skill 列表加载失败";
      }
    }

    function renderMessageActions() {
      for (const card of runview.querySelectorAll("[data-runview-card]")) {
        card.querySelector(".ui-message-actions")?.remove();
        const cardType = card.dataset.runviewCard;
        if (cardType === "assistant" && (card.dataset.runviewLive === "true" || card.dataset.runviewAnswer !== "true")) continue;
        const matching = messageActions.filter((action) => (action.targets || []).includes(cardType));
        if (!matching.length) continue;
        const fields = [
          `data-card-type="${escapeHTML(cardType)}"`,
          card.dataset.entryId ? `data-entry-id="${escapeHTML(card.dataset.entryId)}"` : "",
          card.dataset.runId ? `data-run-id="${escapeHTML(card.dataset.runId)}"` : "",
          card.dataset.boundaryEntryId ? `data-boundary-entry-id="${escapeHTML(card.dataset.boundaryEntryId)}"` : "",
        ].join(" ");
        card.insertAdjacentHTML("beforeend", `<div class="ui-message-actions">${matching.map((action) => `<button type="button" class="message-action ui-button-secondary ui-message-action" data-message-action="${escapeHTML(action.id)}" data-action-icon="${escapeHTML(action.icon)}" data-action-label="${escapeHTML(action.name)}" ${fields}>${escapeHTML(action.icon)} ${escapeHTML(action.name)}</button>`).join("")}</div>`);
      }
    }

    function renderMessageSkills() {
      if (!messageSkillTemplate) return;
      for (const card of runview.querySelectorAll('[data-runview-card="user"]')) {
        const walker = document.createTreeWalker(card, NodeFilter.SHOW_TEXT);
        const nodes = [];
        while (walker.nextNode()) nodes.push(walker.currentNode);
        for (const node of nodes) {
          if (node.parentElement?.closest("code, pre, a, .ui-message-skill")) continue;
          const pattern = /\$([a-z0-9]+(?:-[a-z0-9]+)*)(?![a-z0-9-])/g;
          if (!pattern.test(node.data)) continue;
          pattern.lastIndex = 0;
          const fragment = document.createDocumentFragment();
          let offset = 0;
          for (const match of node.data.matchAll(pattern)) {
            fragment.append(node.data.slice(offset, match.index));
            const skill = messageSkillTemplate.content.cloneNode(true);
            skill.querySelector("[data-message-skill-name]").textContent = match[1];
            fragment.append(skill);
            offset = match.index + match[0].length;
          }
          fragment.append(node.data.slice(offset));
          node.replaceWith(fragment);
        }
      }
    }

    function filterEfforts() {
      for (const option of effort.options) option.hidden = Boolean(option.dataset.model && option.dataset.model !== model.value);
      if (effort.selectedOptions[0]?.dataset.model && effort.selectedOptions[0].dataset.model !== model.value) effort.value = "";
    }

    composer.addEventListener("submit", async (event) => {
      event.preventDefault();
      const fields = new FormData(composer, event.submitter);
      fields.set("mode", runview.dataset.running === "true" ? mode.value : "run");
      let hasFile = false;
      for (const value of fields.values()) {
        if (value instanceof File && value.name !== "") {
          hasFile = true;
          break;
        }
      }
      const body = hasFile ? fields : new URLSearchParams(fields);
      const response = await fetch(composer.action, { method: "POST", body });
      if (!response.ok) {
        status.textContent = await response.text();
      } else {
        textInput.value = "";
        closeSuggestions();
        composer.querySelectorAll('input[type="file"]').forEach((input) => { input.value = ""; });
      }
    });

    textInput.addEventListener("input", filterSuggestions);
    textInput.addEventListener("keydown", (event) => {
      if (event.key === "Escape") {
        if (!suggestionPopover.hidden) {
          event.preventDefault();
          closeSuggestions();
        }
        return;
      }
      if (suggestionPopover.hidden) return;
      const items = visibleSuggestions();
      if (event.key === "ArrowDown" && items.length) {
        event.preventDefault();
        setActiveSuggestion(activeSuggestionIndex + 1);
      } else if (event.key === "ArrowUp" && items.length) {
        event.preventDefault();
        setActiveSuggestion(activeSuggestionIndex - 1);
      } else if (event.key === "Enter" && !event.shiftKey && items[activeSuggestionIndex]) {
        event.preventDefault();
        selectSuggestion(items[activeSuggestionIndex]);
      }
    });
    suggestionItems.addEventListener("click", (event) => {
      const item = event.target.closest("button[data-suggestion-name]");
      if (item) selectSuggestion(item);
    });
    agent?.addEventListener("change", () => refreshSuggestions(triggerState()?.prefix || "/"));

    runview.addEventListener("runview:updated", (event) => {
      setRunning(event.detail.running);
      renderMessageSkills();
      renderMessageActions();
    });
    root.addEventListener("click", async (event) => {
      const button = event.target.closest(".message-action");
      if (!button) return;
      button.disabled = true;
      try {
        const fields = new URLSearchParams({
          cardType: button.dataset.cardType,
          entryID: button.dataset.entryId || "",
          runID: button.dataset.runId || "",
          boundaryEntryID: button.dataset.boundaryEntryId || "",
        });
        const response = await fetch(`${root.dataset.messageActionUrl}${encodeURIComponent(button.dataset.messageAction)}`, {
          method: "POST", body: fields,
        });
        if (!response.ok) throw new Error(await response.text());
        const result = await response.json();
        if (result.redirect) {
          window.location.assign(result.redirect);
          return;
        }
        if (!result.text) throw new Error("没有可复制的文本");
        if (!navigator.clipboard) throw new Error("浏览器不支持复制");
        await navigator.clipboard.writeText(result.text);
        button.textContent = "已复制";
        window.setTimeout(() => { button.textContent = `${button.dataset.actionIcon} ${button.dataset.actionLabel}`; }, 1200);
      } catch (error) {
        status.textContent = error.message || "复制失败";
      } finally {
        button.disabled = false;
      }
    });
    stop.addEventListener("click", async () => {
      const response = await fetch(stop.dataset.stop, { method: "POST" });
      if (!response.ok) status.textContent = await response.text();
    });
    model.addEventListener("change", filterEfforts);
    filterEfforts();
    closeSuggestions();
    setRunning(runview.dataset.running === "true");
    renderMessageSkills();
    renderMessageActions();
  }

  document.addEventListener("DOMContentLoaded", initialize);
  document.addEventListener("htmx:afterSettle", initialize);
  initialize();
})();
