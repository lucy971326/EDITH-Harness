(() => {
  function escapeHTML(value) {
    return String(value || "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;");
  }

  function text(blocks, kind) {
    return (blocks || []).filter((block) => block.kind === kind).map((block) => block.text || "").join("");
  }

  function paintMessage(message, tools) {
    const blocks = message.blocks || [];
    if (message.role === "tool") return "";
    if (message.role === "user") {
      return `<article class="ml-auto max-w-[80%] rounded-xl bg-[var(--color-muted)] px-4 py-3 text-sm">${escapeHTML(text(blocks, "text"))}</article>`;
    }
    let body = "";
    const reasoning = text(blocks, "reasoning");
    const answer = text(blocks, "text");
    if (reasoning) body += `<details class="mb-3 text-xs text-[var(--color-subtle)]"><summary>思考过程</summary><pre class="mt-2 whitespace-pre-wrap font-sans">${escapeHTML(reasoning)}</pre></details>`;
    if (answer) body += `<p class="whitespace-pre-wrap text-sm leading-6">${escapeHTML(answer)}</p>`;
    for (const block of blocks) {
      if (block.kind === "tool-call" && block.tool) {
        const state = tools.get(block.tool.id) || { name: block.tool.name, status: "等待" };
        body += `<div class="mt-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-muted)] px-3 py-2 text-xs"><strong>${escapeHTML(state.name)}</strong><span class="ml-2 text-[var(--color-subtle)]">${escapeHTML(state.status)}</span>${state.output ? `<pre class="mt-2 whitespace-pre-wrap">${escapeHTML(state.output)}</pre>` : ""}</div>`;
      }
    }
    return `<article class="max-w-3xl rounded-xl border border-[var(--color-border)] bg-white px-4 py-3">${body || '<span class="text-sm text-[var(--color-subtle)]">思考中…</span>'}</article>`;
  }

  function initialize() {
    const root = document.getElementById("chat-root");
    const composer = document.getElementById("composer");
    if (!root || !composer || root.dataset.ready === "true") return;
    root.dataset.ready = "true";

    const thread = document.getElementById("thread");
    const status = document.getElementById("chat-status");
    const model = document.getElementById("model");
    const effort = document.getElementById("reasoning-effort");
    const mode = document.getElementById("live-mode");
    const send = document.getElementById("send-button");
    const liveSend = document.getElementById("live-send");
    const stop = document.getElementById("stop-button");
    let messages = [];
    let live = null;
    let running = false;
    let eventVersion = 0;
    let historyTimer = null;
    const tools = new Map();

    function rebuildTools() {
	  const previous = new Map(tools);
      tools.clear();
	  for (const [id, state] of previous) tools.set(id, state);
      for (const message of messages) {
        for (const block of message.blocks || []) {
          if (block.kind === "tool-call" && block.tool) {
            const state = tools.get(block.tool.id) || { status: "等待" };
            state.name = block.tool.name;
            tools.set(block.tool.id, state);
          }
          if (block.kind === "tool-result" && block.result) {
            const state = tools.get(block.result.id) || { name: block.result.name, status: "完成" };
            state.status = block.result.isError ? "失败" : "完成";
            state.output = block.result.content;
            tools.set(block.result.id, state);
          }
        }
      }
    }

    function paint() {
      rebuildTools();
      thread.innerHTML = messages.map((message) => paintMessage(message, tools)).join("");
      thread.lastElementChild?.scrollIntoView({ block: "end" });
    }

    function setRunning(value) {
      running = value;
      send.classList.toggle("hidden", value);
      send.disabled = value;
      mode.classList.toggle("hidden", !value);
      mode.disabled = !value;
      liveSend.classList.toggle("hidden", !value);
      liveSend.disabled = !value;
      stop.classList.toggle("hidden", !value);
      status.textContent = value ? "运行中：可选择插话或下一轮。" : "";
    }

    function filterEfforts() {
      if (!model || !effort) return;
      for (const option of effort.options) {
        option.hidden = Boolean(option.dataset.model && option.dataset.model !== model.value);
      }
      if (effort.selectedOptions[0]?.dataset.model && effort.selectedOptions[0].dataset.model !== model.value) effort.value = "";
    }

    async function loadHistory() {
      const version = eventVersion;
      const response = await fetch(root.dataset.history);
      if (!response.ok) return;
      const history = await response.json();
      if (version !== eventVersion) {
        scheduleHistory();
        return;
      }
      messages = history;
      live = null;
      paint();
    }

    function scheduleHistory() {
      if (historyTimer !== null) window.clearTimeout(historyTimer);
      historyTimer = window.setTimeout(() => {
        historyTimer = null;
        loadHistory();
      }, 100);
    }

    composer.addEventListener("submit", async (event) => {
      event.preventDefault();
      const response = await fetch(composer.action, { method: "POST", body: new FormData(composer) });
      if (!response.ok) {
        status.textContent = await response.text();
        return;
      }
      composer.elements.text.value = "";
    });
    stop.addEventListener("click", async () => {
      const response = await fetch(stop.dataset.stop, { method: "POST" });
      if (!response.ok) status.textContent = await response.text();
    });
    model?.addEventListener("change", filterEfforts);
    filterEfforts();

    document.addEventListener("htmx:sseBeforeMessage", (event) => {
      if (!root.contains(event.target)) return;
      event.preventDefault();
      let item;
      try { item = JSON.parse(event.detail.data); } catch { return; }
      eventVersion += 1;
      if (item.kind === "run-started") {
        setRunning(true);
      } else if (item.kind === "reasoning-delta" || item.kind === "text-delta") {
        if (!live) {
          live = { role: "assistant", blocks: [] };
          messages.push(live);
        }
        const kind = item.kind === "text-delta" ? "text" : "reasoning";
        const last = live.blocks[live.blocks.length - 1];
        if (last?.kind === kind) last.text += item.text || "";
        else live.blocks.push({ kind, text: item.text || "" });
        paint();
      } else if (item.kind === "tool-started" && item.tool) {
        tools.set(item.tool.id, { name: item.tool.name, status: "执行中" });
        paint();
      } else if (item.kind === "tool-finished" && item.tool) {
        const state = tools.get(item.tool.id) || { name: item.tool.name };
        state.status = item.tool.isError ? "失败" : "完成";
        tools.set(item.tool.id, state);
        paint();
      } else if (item.kind === "message" && item.message) {
        if (item.message.role === "assistant" && live) {
          Object.assign(live, item.message);
          live = null;
        } else {
          messages.push(item.message);
        }
        const result = (item.message.blocks || []).find((block) => block.kind === "tool-result")?.result;
        if (result && tools.has(result.id)) {
          const state = tools.get(result.id);
          state.output = result.content;
          tools.set(result.id, state);
        }
        paint();
      } else if (item.kind === "run-ended") {
        setRunning(false);
        if (item.status !== "success") status.textContent = item.status === "cancelled" ? "已停止" : item.error || "运行失败";
      }
    });
    document.addEventListener("htmx:sseOpen", (event) => {
      if (root.contains(event.target)) scheduleHistory();
    });
    loadHistory();
  }

  document.addEventListener("DOMContentLoaded", initialize);
  document.addEventListener("htmx:afterSettle", initialize);
  initialize();
})();
