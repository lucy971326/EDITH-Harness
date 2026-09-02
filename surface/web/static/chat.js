(() => {
  function escapeHTML(value) {
    return String(value || "").replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  }

  function text(blocks, kind) {
    return (blocks || []).filter((block) => block.kind === kind).map((block) => block.text || "").join("");
  }

  function toolBlock(block) {
    const tool = block.tool || {};
    const result = block.result;
    const status = result ? (result.isError ? "失败" : "完成") : (block.status || "执行中");
    return `<details class="mb-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-muted)] px-3 py-2 text-xs"><summary class="cursor-pointer"><strong>${escapeHTML(tool.name || result?.name)}</strong><span class="ml-2 text-[var(--color-subtle)]">${escapeHTML(status)}</span></summary>${result?.content ? `<pre class="mt-2 whitespace-pre-wrap">${escapeHTML(result.content)}</pre>` : ""}</details>`;
  }

  function assistantCard(step, runID) {
    let body = "";
    for (const block of step.blocks || []) {
      if (block.kind === "reasoning" && block.text) {
        body += `<details class="mb-3 text-xs text-[var(--color-subtle)]"><summary>思考过程</summary><pre class="mt-2 whitespace-pre-wrap font-sans">${escapeHTML(block.text)}</pre></details>`;
      } else if (block.kind === "text" && block.text) {
        body += `<p class="mb-3 whitespace-pre-wrap text-sm leading-6">${escapeHTML(block.text)}</p>`;
      } else if (block.kind === "tool-call") {
        body += toolBlock(block);
      }
    }
    return `<article data-run-id="${escapeHTML(runID)}" class="max-w-3xl rounded-xl border border-[var(--color-border)] bg-white px-4 py-3">${body || '<span class="text-sm text-[var(--color-subtle)]">思考中…</span>'}</article>`;
  }

  function userCard(entry) {
    return `<article class="ml-auto max-w-[80%] rounded-xl bg-[var(--color-muted)] px-4 py-3 text-sm">${escapeHTML(text(entry.message.blocks, "text"))}</article>`;
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
    const state = { entries: new Map(), runs: new Map(), running: false, activeRunID: "" };
    let hydrating = false;
    let queuedEvents = [];

    function runFor(id, afterEntrySeq = 0) {
      let run = state.runs.get(id);
      if (!run) {
        run = { id, afterEntrySeq, nextStepSeq: 1, steps: new Map() };
        state.runs.set(id, run);
      }
      if (afterEntrySeq) run.afterEntrySeq = afterEntrySeq;
      return run;
    }

    function stepFor(run, stepSeq, afterEntrySeq) {
      const seq = Number(stepSeq || run.nextStepSeq++);
      run.nextStepSeq = Math.max(run.nextStepSeq, seq + 1);
      let step = run.steps.get(seq);
      if (!step) {
        step = { seq, afterEntrySeq: afterEntrySeq || run.afterEntrySeq, durableSeq: 0, blocks: [] };
        run.steps.set(seq, step);
      }
      if (afterEntrySeq) step.afterEntrySeq = afterEntrySeq;
      return step;
    }

    function applyEntry(entry, event = {}) {
      if (!entry?.id || state.entries.has(entry.id)) return;
      state.entries.set(entry.id, entry);
      const message = entry.message || {};
      if (!message.runID || (message.role !== "assistant" && message.role !== "tool")) return;
      const run = runFor(message.runID, event.afterEntrySeq || Math.max(0, entry.seq - 1));
      if (message.role === "assistant") {
        const step = stepFor(run, event.stepSeq, event.afterEntrySeq || run.afterEntrySeq);
        step.durableSeq = entry.seq;
        step.blocks = (message.blocks || []).map((block) => ({ ...block }));
        return;
      }
      const result = (message.blocks || []).find((block) => block.kind === "tool-result")?.result;
      if (!result) return;
      for (const step of run.steps.values()) {
        const block = step.blocks.find((candidate) => candidate.kind === "tool-call" && candidate.tool?.id === result.id);
        if (block) {
          block.result = result;
          return;
        }
      }
    }

    function applyEvent(event) {
      if (event.kind === "message" && event.entry) {
        applyEntry(event.entry, event);
        return;
      }
      if (event.kind === "run-started") {
        state.activeRunID = event.runID;
        state.running = true;
        runFor(event.runID, event.afterEntrySeq);
        return;
      }
      const runID = event.runID || state.activeRunID;
      const run = state.runs.get(runID);
      if (event.kind === "reasoning-delta" || event.kind === "text-delta") {
        if (!run) return;
        const step = stepFor(run, event.stepSeq, event.afterEntrySeq);
        const kind = event.kind === "text-delta" ? "text" : "reasoning";
        const index = Math.max(0, Number(event.blockSeq || 1) - 1);
        const old = step.blocks[index];
        if (old?.kind === kind) old.text = (old.text || "") + (event.text || "");
        else step.blocks[index] = { kind, text: event.text || "" };
        return;
      }
      if ((event.kind === "tool-started" || event.kind === "tool-finished") && event.tool && run) {
        const step = stepFor(run, event.stepSeq, event.afterEntrySeq);
        const block = step.blocks.find((candidate) => candidate.kind === "tool-call" && candidate.tool?.id === event.tool.id);
        if (block) block.status = event.kind === "tool-started" ? "执行中" : (event.tool.isError ? "失败" : "完成");
        return;
      }
      if (event.kind === "run-ended") {
        if (event.runID === state.activeRunID) state.activeRunID = "";
        state.running = false;
        if (event.status !== "success") status.textContent = event.status === "cancelled" ? "已停止" : event.error || "运行失败";
      }
    }

    function apply(input) {
      if (input.entries) {
        state.entries.clear();
        state.runs.clear();
        state.running = (input.runs || []).length > 0;
        state.activeRunID = input.runs?.[0]?.runID || "";
        for (const run of input.runs || []) runFor(run.runID, run.afterEntrySeq);
        for (const entry of input.entries) applyEntry(entry);
      } else applyEvent(input);
      paint();
    }

    function orderedItems() {
      const items = [];
      for (const entry of state.entries.values()) {
        if (entry.message?.role === "user") items.push({ kind: "entry", seq: entry.seq, entry });
      }
      for (const run of state.runs.values()) {
        for (const step of run.steps.values()) {
          const seq = step.durableSeq || step.afterEntrySeq + step.seq / 1000;
          items.push({ kind: "step", seq, runID: run.id, step });
        }
      }
      return items.sort((left, right) => left.seq - right.seq);
    }

    function paint() {
      thread.innerHTML = orderedItems().map((item) => item.kind === "entry" ? userCard(item.entry) : assistantCard(item.step, item.runID)).join("");
      thread.lastElementChild?.scrollIntoView({ block: "end" });
      setRunning(state.running);
    }

    function setRunning(value) {
      send.classList.toggle("hidden", value);
      send.disabled = value;
      mode.classList.toggle("hidden", !value);
      mode.disabled = !value;
      liveSend.classList.toggle("hidden", !value);
      liveSend.disabled = !value;
      stop.classList.toggle("hidden", !value);
      if (value && !status.textContent) status.textContent = "运行中：可选择插话或下一轮。";
      if (!value && status.textContent === "运行中：可选择插话或下一轮。") status.textContent = "";
    }

    function filterEfforts() {
      for (const option of effort.options) option.hidden = Boolean(option.dataset.model && option.dataset.model !== model.value);
      if (effort.selectedOptions[0]?.dataset.model && effort.selectedOptions[0].dataset.model !== model.value) effort.value = "";
    }

    async function loadHistory() {
      hydrating = true;
      const response = await fetch(root.dataset.history);
      if (response.ok) apply(await response.json());
      hydrating = false;
      for (const event of queuedEvents) apply(event);
      queuedEvents = [];
    }

    composer.addEventListener("submit", async (event) => {
      event.preventDefault();
      const fields = new FormData(composer, event.submitter);
      fields.set("mode", state.running ? mode.value : "run");
      const response = await fetch(composer.action, { method: "POST", body: new URLSearchParams(fields) });
      if (!response.ok) status.textContent = await response.text();
      else composer.elements.text.value = "";
    });
    stop.addEventListener("click", async () => {
      const response = await fetch(stop.dataset.stop, { method: "POST" });
      if (!response.ok) status.textContent = await response.text();
    });
    model.addEventListener("change", filterEfforts);
    filterEfforts();

    document.addEventListener("htmx:sseBeforeMessage", (event) => {
      if (!root.contains(event.target)) return;
      event.preventDefault();
      try {
        const item = JSON.parse(event.detail.data);
        if (hydrating) queuedEvents.push(item);
        else apply(item);
      } catch { /* malformed SSE is ignored */ }
    });
    document.addEventListener("htmx:sseOpen", (event) => {
      if (root.contains(event.target)) loadHistory();
    });
    loadHistory();
  }

  document.addEventListener("DOMContentLoaded", initialize);
  document.addEventListener("htmx:afterSettle", initialize);
  initialize();
})();
