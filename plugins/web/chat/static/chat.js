(() => {
  function initialize() {
    const root = document.getElementById("chat-root");
    const composer = document.getElementById("composer");
    const runview = root?.querySelector("[data-runview]");
    if (!root || !composer || !runview || root.dataset.ready === "true") return;
    root.dataset.ready = "true";

    const status = document.getElementById("chat-status");
    const model = document.getElementById("model");
    const effort = document.getElementById("reasoning-effort");
    const mode = document.getElementById("live-mode");
    const send = document.getElementById("send-button");
    const liveSend = document.getElementById("live-send");
    const stop = document.getElementById("stop-button");
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
        composer.elements.text.value = "";
        composer.querySelectorAll('input[type="file"]').forEach((input) => { input.value = ""; });
      }
    });

    runview.addEventListener("runview:updated", (event) => {
      setRunning(event.detail.running);
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
    setRunning(runview.dataset.running === "true");
    renderMessageActions();
  }

  document.addEventListener("DOMContentLoaded", initialize);
  document.addEventListener("htmx:afterSettle", initialize);
  initialize();
})();
