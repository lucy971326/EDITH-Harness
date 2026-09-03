(() => {
  function escapeHTML(value) {
    if (value == null) return "";
    return String(value).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/\"/g, "&quot;");
  }

  const markdownTags = [
    "a", "blockquote", "br", "code", "del", "em", "h1", "h2", "h3", "h4", "h5", "h6", "hr", "img",
    "input", "li", "ol", "p", "pre", "strong", "table", "tbody", "td", "th", "thead", "tr", "ul",
  ];
  const markdownAttributes = ["alt", "checked", "colspan", "disabled", "href", "rowspan", "src", "start", "title", "type"];

  function safeURL(value, allowMailto = true) {
    if (typeof value !== "string" || !value.trim()) return "";
    try {
      const url = new URL(value, window.location.href);
      const allowed = allowMailto ? ["http:", "https:", "mailto:"] : ["http:", "https:"];
      return allowed.includes(url.protocol) ? url.href : "";
    } catch {
      return "";
    }
  }

  function plainTextMarkdown(value) {
    return escapeHTML(value).replace(/\n/g, "<br>");
  }

  function renderMarkdown(value) {
    const text = String(value || "");
    if (!window.marked || !window.DOMPurify) return plainTextMarkdown(text);
    const parsed = window.marked.parse(text, { breaks: true, gfm: true });
    const clean = window.DOMPurify.sanitize(parsed, {
      ALLOWED_ATTR: markdownAttributes,
      ALLOWED_TAGS: markdownTags,
      ALLOW_DATA_ATTR: false,
      ALLOW_ARIA_ATTR: false,
    });
    const template = document.createElement("template");
    template.innerHTML = clean;
    for (const image of template.content.querySelectorAll("img")) {
      const label = image.getAttribute("alt") || "图片";
      const href = safeURL(image.getAttribute("src"), false);
      const parentLink = image.closest("a");
      const replacement = document.createElement(href && !parentLink ? "a" : "span");
      replacement.textContent = `图片：${label}`;
      replacement.className = "ui-markdown-image-link";
      if (href && replacement.tagName === "A") replacement.setAttribute("href", href);
      image.replaceWith(replacement);
    }
    for (const link of template.content.querySelectorAll("a")) {
      const href = safeURL(link.getAttribute("href"));
      if (!href) {
        link.replaceWith(...link.childNodes);
        continue;
      }
      link.setAttribute("href", href);
      link.setAttribute("target", "_blank");
      link.setAttribute("rel", "noopener noreferrer");
    }
    return template.innerHTML;
  }

  function number(value, fallback = 1) {
    const result = Number(value);
    return Number.isFinite(result) && result > 0 ? result : fallback;
  }

  function sortedSteps(segment) {
    return [...segment.steps.values()].sort((left, right) => left.seq - right.seq);
  }

  function sortedBlocks(step) {
    return [...step.blocks.values()].sort((left, right) => left.seq - right.seq);
  }

  function textBlocks(blocks, kind = "text") {
    return (blocks || [])
      .filter((block) => block.kind === kind)
      .map((block) => block.text || "")
      .join("");
  }

  function hasToolCall(step) {
    return [...step.blocks.values()].some((block) => block.kind === "tool-call");
  }

  function finalStep(segment) {
    const steps = sortedSteps(segment);
    for (let index = steps.length - 1; index >= 0; index--) {
      if (!hasToolCall(steps[index]) && steps[index].durable) return steps[index];
    }
    return null;
  }

  function formatArguments(args) {
    if (!args) return "";
    try {
      return JSON.stringify(JSON.parse(args), null, 2);
    } catch {
      return args;
    }
  }

  function toolStatus(block) {
    const result = block.result;
    if (result) return result.isError ? "失败" : "完成";
    return block.status || "执行中";
  }

  function toolStatusClass(status) {
    if (status === "失败") return "ui-workflow-tool-status-danger";
    if (status === "执行中") return "ui-workflow-tool-status-running";
    return "";
  }

  function toolPreview(args) {
    if (!args) return "";
    try {
      const value = JSON.parse(args);
      if (value && typeof value === "object" && !Array.isArray(value)) {
        for (const key of ["command", "path", "query", "url"]) {
          if (typeof value[key] === "string" && value[key].trim()) return value[key];
        }
      }
    } catch {
      // 非 JSON 参数按原文展示。
    }
    return String(args);
  }

  function truncate(value, limit = 88) {
    const text = String(value || "").replace(/\s+/g, " ").trim();
    return text.length > limit ? `${text.slice(0, limit - 1)}…` : text;
  }

  function toolBlock(segment, step, block) {
    const tool = block.tool || {};
    const result = block.result;
    const status = toolStatus(block);
    const toolName = tool.name || result?.name || "工具";
    const key = `tool:${segment.runID}:${segment.boundarySeq}:${step.seq}:${block.seq}:${tool.id || result?.id || ""}`;
    const args = formatArguments(tool.args);
    const output = result?.content || "";
    const preview = truncate(toolPreview(tool.args));
    return `<details class="ui-workflow-tool" data-disclosure-key="${escapeHTML(key)}">
  <summary class="ui-workflow-tool-summary">
    <span class="ui-workflow-tool-name">${escapeHTML(toolName)}</span>
    ${preview ? `<span class="ui-workflow-tool-preview ui-text-code">${escapeHTML(preview)}</span>` : ""}
    <span class="ui-workflow-tool-status ${toolStatusClass(status)}">${escapeHTML(status)}</span>
  </summary>
  <div class="ui-workflow-tool-content">
    ${args ? `<div class="ui-workflow-field"><span class="ui-workflow-field-label ui-text-meta">参数</span><pre class="ui-workflow-output ui-text-code">${escapeHTML(args)}</pre></div>` : ""}
    ${output ? `<div class="ui-workflow-field"><span class="ui-workflow-field-label ui-text-meta">结果</span><pre class="ui-workflow-output ui-text-code ${result?.isError ? "ui-workflow-output-error" : ""}">${escapeHTML(output)}</pre></div>` : ""}
  </div>
</details>`;
  }

  function workflowContent(segment) {
    const answer = finalStep(segment);
    let content = "";
    let toolCount = 0;
    for (const step of sortedSteps(segment)) {
      for (const block of sortedBlocks(step)) {
        if (step === answer && block.kind === "text") continue;
        if ((block.kind === "reasoning" || block.kind === "text") && block.text) {
          content += `<p class="ui-workflow-note ${block.kind === "reasoning" ? "ui-workflow-note-reasoning" : ""}">${escapeHTML(block.text)}</p>`;
        } else if (block.kind === "tool-call") {
          toolCount++;
          content += toolBlock(segment, step, block);
        }
      }
    }
    return { answer, content, toolCount };
  }

  function latestRunningTool(segment) {
    let latest = null;
    for (const step of sortedSteps(segment)) {
      for (const block of sortedBlocks(step)) {
        if (block.kind === "tool-call" && block.status === "执行中") latest = block;
      }
    }
    return latest;
  }

  function workflowSummary(run, segment, live, hasContent, toolCount) {
    if (live) {
      const tool = latestRunningTool(segment);
      const name = tool?.tool?.name;
      return name ? `正在工作 · ${name}` : "正在工作";
    }
    const latest = !run.latestSegment || run.latestSegment === segment.id;
    if (latest && run.status === "failed") return "未完成 · 查看过程";
    if (latest && run.status === "cancelled") return "已停止 · 查看过程";
    if (!hasContent) return "";
    return toolCount ? `已完成 · ${toolCount} 次工具调用` : "已完成 · 工作过程";
  }

  function workflowStatusClass(run, segment, live) {
    if (live) return "ui-workflow-status-running";
    if (run.latestSegment === segment.id && (run.status === "failed" || run.status === "cancelled")) return "ui-workflow-status-danger";
    return "";
  }

  function actionBar(actions, target) {
    if (target.cardType === "assistant" && !target.boundaryEntryID) return "";
    const matching = actions.filter((action) => (action.targets || []).includes(target.cardType));
    if (!matching.length) return "";
    const fields = [
      `data-card-type="${escapeHTML(target.cardType)}"`,
      target.entryID ? `data-entry-id="${escapeHTML(target.entryID)}"` : "",
      target.runID ? `data-run-id="${escapeHTML(target.runID)}"` : "",
      target.boundaryEntryID ? `data-boundary-entry-id="${escapeHTML(target.boundaryEntryID)}"` : "",
    ].join(" ");
    return `<div class="ui-message-actions">${matching.map((action) => `<button type="button" class="message-action ui-button-secondary ui-message-action" data-message-action="${escapeHTML(action.id)}" data-action-icon="${escapeHTML(action.icon)}" data-action-label="${escapeHTML(action.name)}" ${fields}>${escapeHTML(action.icon)} ${escapeHTML(action.name)}</button>`).join("")}</div>`;
  }

  function assistantCard(run, segment, actions, live) {
    const parts = workflowContent(segment);
    const answerText = parts.answer ? textBlocks(sortedBlocks(parts.answer)) : "";
    const summary = workflowSummary(run, segment, live, Boolean(parts.content), parts.toolCount);
    const showWorkflow = Boolean(summary);
    const error = run.latestSegment === segment.id && run.status === "failed" && run.error ? `<p class="ui-workflow-error ui-text-body">${escapeHTML(run.error)}</p>` : "";
    const workflow = showWorkflow ? `<details class="ui-workflow" data-disclosure-key="workflow:${escapeHTML(segment.id)}">
  <summary class="ui-workflow-summary ${workflowStatusClass(run, segment, live)}">${escapeHTML(summary)}</summary>
  <div class="ui-workflow-body">${parts.content || `<p class="ui-workflow-empty ui-text-meta">暂无可展开内容</p>`}${error}</div>
</details>` : "";
    const answer = answerText ? `<div class="ui-message-answer ui-text-body ui-markdown">${renderMarkdown(answerText)}</div>` : "";
    const actionsHTML = live || !answerText ? "" : actionBar(actions, {
      cardType: "assistant", runID: segment.runID, boundaryEntryID: segment.boundaryEntryID,
    });
    if (!workflow && !answer && !actionsHTML) return "";
    return `<article data-run-id="${escapeHTML(segment.runID)}" data-segment-id="${escapeHTML(segment.id)}" class="ui-message-assistant">${workflow}${answer}${actionsHTML}</article>`;
  }

  function userCard(entry, actions) {
    const actionsHTML = actionBar(actions, { cardType: "user", entryID: entry.id });
    return `<article class="ui-message-user ui-text-body ui-markdown">${renderMarkdown(textBlocks(entry.message.blocks))}${actionsHTML}</article>`;
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
    let messageActions = [];
    try {
      messageActions = JSON.parse(root.dataset.messageActions || "[]");
    } catch { /* invalid action definitions leave the action bar empty */ }
    const state = { entries: new Map(), runs: new Map(), running: false, activeRunID: "" };
    const expanded = new Set();
    let hydrating = false;
    let queuedEvents = [];
    let historyRequest = null;

    function runFor(id, afterEntrySeq = 0) {
      let run = state.runs.get(id);
      if (!run) {
        run = { id, afterEntrySeq, status: "", error: "", latestSegment: "", segments: new Map(), pendingResults: new Map() };
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
        if (!boundary || boundary.seq < entry.seq) boundary = entry;
      }
      return boundary;
    }

    function segmentFor(run, maxSeq = Number.POSITIVE_INFINITY) {
      const boundary = userBoundary(run, maxSeq);
      const boundarySeq = boundary?.seq || run.afterEntrySeq || 0;
      const id = `${run.id}:${boundarySeq}`;
      let segment = run.segments.get(id);
      if (!segment) {
        segment = { id, runID: run.id, boundarySeq, boundaryEntryID: boundary?.id || "", steps: new Map() };
        run.segments.set(id, segment);
      } else if (!segment.boundaryEntryID && boundary?.id) {
        segment.boundaryEntryID = boundary.id;
      }
      if (!run.latestSegment || run.segments.get(run.latestSegment)?.boundarySeq <= boundarySeq) run.latestSegment = id;
      return segment;
    }

    function stepFor(segment, stepSeq) {
      const seq = number(stepSeq);
      let step = segment.steps.get(seq);
      if (!step) {
        step = { seq, durable: false, blocks: new Map() };
        segment.steps.set(seq, step);
      }
      return step;
    }

    function mergeBlock(step, seq, block) {
      const blockSeq = number(seq);
      const previous = step.blocks.get(blockSeq);
      const merged = { ...block, seq: blockSeq };
      if (previous?.status) merged.status = previous.status;
      if (previous?.result && !merged.result) merged.result = previous.result;
      step.blocks.set(blockSeq, merged);
      return merged;
    }

    function blockFor(step, seq, kind) {
      const blockSeq = number(seq);
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

    function registerAssistant(run, entry, event, durable) {
      const maxSeq = Number(entry.seq) || Number.POSITIVE_INFINITY;
      const segment = segmentFor(run, maxSeq);
      let stepSeq = number(event.stepSeq, 0);
      if (!stepSeq) {
        for (const candidateSegment of run.segments.values()) {
          for (const candidate of candidateSegment.steps.values()) stepSeq = Math.max(stepSeq, candidate.seq);
        }
        stepSeq++;
      }
      const step = stepFor(segment, stepSeq);
      step.durable = durable || step.durable;
      for (const [index, block] of (entry.message.blocks || []).entries()) {
        const merged = mergeBlock(step, index + 1, { ...block, durable });
        if (merged.kind === "tool-call" && merged.tool?.id) {
          const result = run.pendingResults.get(merged.tool.id);
          if (result) {
            merged.result = result;
            run.pendingResults.delete(merged.tool.id);
          }
        }
      }
    }

    function applyEntry(entry, event = {}, durable = false) {
      if (!entry?.id || state.entries.has(entry.id)) return;
      state.entries.set(entry.id, entry);
      const message = entry.message || {};
      if (!message.runID) return;
      if (message.role === "user" && !state.runs.has(message.runID)) return;
      const run = runFor(message.runID, event.afterEntrySeq || Math.max(0, entry.seq - 1));
      if (message.role === "user") {
        segmentFor(run, entry.seq);
        return;
      }
      if (message.role === "assistant") {
        registerAssistant(run, entry, event, durable);
        return;
      }
      if (message.role !== "tool") return;
      const result = (message.blocks || []).find((block) => block.kind === "tool-result")?.result;
      attachResult(run, result, event.stepSeq, event.blockSeq);
    }

    function applyEvent(event) {
      if (event.kind === "message" && event.entry) {
        applyEntry(event.entry, event, true);
        return;
      }
      if (event.kind === "run-started") {
        state.activeRunID = event.runID;
        state.running = true;
        const run = runFor(event.runID, event.afterEntrySeq);
        run.status = "running";
        segmentFor(run);
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
        if (block.durable) return;
        if (block.kind === kind) block.text = (block.text || "") + (event.text || "");
        else step.blocks.set(number(event.blockSeq), { seq: number(event.blockSeq), kind, text: event.text || "" });
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
        if (run) {
          run.status = event.status || "success";
          run.error = event.error || "";
        }
        if (event.runID === state.activeRunID) state.activeRunID = "";
        state.running = false;
      }
    }

    function apply(input) {
      if (input.entries) {
        state.entries.clear();
        state.runs.clear();
        state.running = (input.runs || []).length > 0;
        state.activeRunID = input.runs?.[0]?.runID || "";
        for (const run of input.runs || []) {
          const current = runFor(run.runID, run.afterEntrySeq);
          current.status = "running";
        }
        for (const entry of input.entries) applyEntry(entry, {}, true);
        for (const run of state.runs.values()) {
          if (run.status === "running" && run.segments.size === 0) segmentFor(run);
        }
      } else applyEvent(input);
      paint();
    }

    function orderedItems() {
      const items = [];
      for (const entry of state.entries.values()) {
        if (entry.message?.role === "user") items.push({ kind: "entry", seq: entry.seq, entry });
      }
      for (const run of state.runs.values()) {
        for (const segment of run.segments.values()) items.push({ kind: "segment", seq: segment.boundarySeq, run, segment });
      }
      return items.sort((left, right) => {
        const leftSeq = left.seq * 2 + (left.kind === "segment" ? 1 : 0);
        const rightSeq = right.seq * 2 + (right.kind === "segment" ? 1 : 0);
        return leftSeq - rightSeq;
      });
    }

    function rememberExpanded() {
      for (const detail of thread.querySelectorAll("details[data-disclosure-key]")) {
        if (detail.open) expanded.add(detail.dataset.disclosureKey);
        else expanded.delete(detail.dataset.disclosureKey);
      }
    }

    function paint() {
      rememberExpanded();
      thread.innerHTML = orderedItems().map((item) => {
        if (item.kind === "entry") return userCard(item.entry, messageActions);
        const live = state.running && item.run.id === state.activeRunID && item.run.latestSegment === item.segment.id;
        return assistantCard(item.run, item.segment, messageActions, live);
      }).join("");
      for (const detail of thread.querySelectorAll("details[data-disclosure-key]")) {
        detail.open = expanded.has(detail.dataset.disclosureKey);
      }
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
    thread.addEventListener("click", async (event) => {
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

    document.addEventListener("htmx:sseBeforeMessage", (event) => {
      if (!root.contains(event.target)) return;
      if (event.detail.type !== "run") return;
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
