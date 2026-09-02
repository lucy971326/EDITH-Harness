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

  function assistantCard(segment) {
    let body = "";
    for (const step of [...segment.steps.values()].sort((left, right) => left.seq - right.seq)) {
      for (const block of [...step.blocks.values()].sort((left, right) => left.seq - right.seq)) {
        if (block.kind === "reasoning" && block.text) {
          body += `<details class="mb-3 text-xs text-[var(--color-subtle)]"><summary>思考过程</summary><pre class="mt-2 whitespace-pre-wrap font-sans">${escapeHTML(block.text)}</pre></details>`;
        } else if (block.kind === "text" && block.text) {
          body += `<p class="mb-3 whitespace-pre-wrap text-sm leading-6">${escapeHTML(block.text)}</p>`;
        } else if (block.kind === "tool-call") {
          body += toolBlock(block);
        }
      }
    }
    return `<article data-run-id="${escapeHTML(segment.runID)}" data-segment-id="${escapeHTML(segment.id)}" class="max-w-3xl rounded-xl border border-[var(--color-border)] bg-white px-4 py-3">${body || '<span class="text-sm text-[var(--color-subtle)]">思考中…</span>'}</article>`;
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
    let historyRequest = null;

    function runFor(id, afterEntrySeq = 0) {
      let run = state.runs.get(id);
      if (!run) {
        run = { id, afterEntrySeq, segments: new Map(), pendingResults: new Map() };
        state.runs.set(id, run);
      } else if (!run.afterEntrySeq && afterEntrySeq) {
        run.afterEntrySeq = afterEntrySeq;
      }
      return run;
    }

    function userBoundary(run, maxSeq = Number.POSITIVE_INFINITY) {
      let boundary = null;
      for (const entry of state.entries.values()) {
        if (entry.message?.role !== "user" || entry.message.runID !== run.id || entry.seq > maxSeq) continue;
        if (!boundary || entry.seq > boundary.seq) boundary = entry;
      }
      return boundary;
    }

    function segmentFor(run, maxSeq = Number.POSITIVE_INFINITY) {
      const boundary = userBoundary(run, maxSeq);
      const boundaryID = boundary?.id || `seq-${boundary?.seq || run.afterEntrySeq || 0}`;
      const boundarySeq = boundary?.seq || run.afterEntrySeq || 0;
      const id = `${run.id}:${boundaryID}`;
      let segment = run.segments.get(id);
      if (!segment) {
        segment = { id, runID: run.id, boundarySeq, steps: new Map() };
        run.segments.set(id, segment);
      }
      return segment;
    }

    function stepFor(segment, stepSeq) {
      const seq = Number(stepSeq) || 1;
      let step = segment.steps.get(seq);
      if (!step) {
        step = { seq, blocks: new Map() };
        segment.steps.set(seq, step);
      }
      return step;
    }

    function mergeBlock(step, seq, block) {
      const blockSeq = Number(seq) || 1;
      const previous = step.blocks.get(blockSeq);
      const merged = { ...block, seq: blockSeq };
      if (previous?.status) merged.status = previous.status;
      if (previous?.result && !merged.result) merged.result = previous.result;
      step.blocks.set(blockSeq, merged);
      return merged;
    }

    function blockFor(step, seq, kind) {
      const blockSeq = Number(seq) || 1;
      let block = step.blocks.get(blockSeq);
      if (!block) {
        block = { seq: blockSeq, kind, text: "" };
        step.blocks.set(blockSeq, block);
      }
      return block;
    }

    function attachResult(run, result, stepSeq, blockSeq) {
      if (!result?.id) return;
      for (const segment of run.segments.values()) {
        for (const step of segment.steps.values()) {
          for (const block of step.blocks.values()) {
            if (block.kind === "tool-call" && block.tool?.id === result.id) {
              block.result = result;
              return;
            }
          }
        }
      }
      if (stepSeq || blockSeq) {
        const segment = segmentFor(run);
        const step = stepFor(segment, stepSeq);
        const block = blockFor(step, blockSeq, "tool-call");
        block.tool = block.tool || { id: result.id, name: result.name };
        block.result = result;
        return;
      }
      run.pendingResults.set(result.id, result);
    }

    function registerAssistant(run, entry, event) {
      const maxSeq = Number(entry.seq) || Number.POSITIVE_INFINITY;
      const segment = segmentFor(run, maxSeq);
      let stepSeq = Number(event.stepSeq) || 0;
      if (!stepSeq) {
        for (const candidate of segment.steps.values()) stepSeq = Math.max(stepSeq, candidate.seq);
        stepSeq++;
      }
      const step = stepFor(segment, stepSeq);
      for (const [index, block] of (entry.message.blocks || []).entries()) {
        const merged = mergeBlock(step, index + 1, { ...block });
        if (merged.kind === "tool-call" && merged.tool?.id) {
          const result = run.pendingResults.get(merged.tool.id);
          if (result) {
            merged.result = result;
            run.pendingResults.delete(merged.tool.id);
          }
        }
      }
    }

    function applyEntry(entry, event = {}) {
      if (!entry?.id || state.entries.has(entry.id)) return;
      state.entries.set(entry.id, entry);
      const message = entry.message || {};
      if (!message.runID || (message.role !== "assistant" && message.role !== "tool")) return;
      const run = runFor(message.runID, event.afterEntrySeq || Math.max(0, entry.seq - 1));
      if (message.role === "assistant") {
        registerAssistant(run, entry, event);
        return;
      }
      const result = (message.blocks || []).find((block) => block.kind === "tool-result")?.result;
      attachResult(run, result, event.stepSeq, event.blockSeq);
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
        const segment = segmentFor(run);
        const step = stepFor(segment, event.stepSeq);
        const kind = event.kind === "text-delta" ? "text" : "reasoning";
        const block = blockFor(step, event.blockSeq, kind);
        if (block.kind === kind) block.text = (block.text || "") + (event.text || "");
        else step.blocks.set(Number(event.blockSeq) || 1, { seq: Number(event.blockSeq) || 1, kind, text: event.text || "" });
        return;
      }
      if ((event.kind === "tool-started" || event.kind === "tool-finished") && event.tool && run) {
        let target = null;
        for (const segment of run.segments.values()) {
          for (const step of segment.steps.values()) {
            for (const block of step.blocks.values()) {
              if (block.kind === "tool-call" && block.tool?.id === event.tool.id) target = block;
            }
          }
        }
        if (!target) {
          const segment = segmentFor(run);
          const step = stepFor(segment, event.stepSeq);
          target = blockFor(step, event.blockSeq, "tool-call");
          target.tool = { id: event.tool.id, name: event.tool.name };
        }
        target.status = event.kind === "tool-started" ? "执行中" : (event.tool.isError ? "失败" : "完成");
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
        for (const segment of run.segments.values()) {
          items.push({ kind: "segment", seq: segment.boundarySeq, runID: run.id, segment });
        }
      }
      return items.sort((left, right) => {
        const leftSeq = left.seq * 2 + (left.kind === "segment" ? 1 : 0);
        const rightSeq = right.seq * 2 + (right.kind === "segment" ? 1 : 0);
        return leftSeq - rightSeq;
      });
    }

    function paint() {
      thread.innerHTML = orderedItems().map((item) => item.kind === "entry" ? userCard(item.entry) : assistantCard(item.segment)).join("");
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
      if (historyRequest) return historyRequest;
      hydrating = true;
      historyRequest = (async () => {
        try {
          const response = await fetch(root.dataset.history);
          if (response.ok) apply(await response.json());
        } finally {
          hydrating = false;
          for (const event of queuedEvents) apply(event);
          queuedEvents = [];
          historyRequest = null;
        }
      })();
      return historyRequest;
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
