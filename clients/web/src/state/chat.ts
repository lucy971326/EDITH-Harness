import type {
  Block,
  Entry,
  Message,
  RunEvent,
  RunState,
  Snapshot,
} from "../../../contracts/run.ts";

// 历史就是状态底稿；实时事件只修改这一份投影，不维护另一套聊天记录。
// null 表示边界断了，调用方重新订阅；不在前端排队猜顺序。
export function applyRunEvent(
  snapshot: Snapshot,
  event: RunEvent,
): Snapshot | null {
  if (event.seqEpoch !== snapshot.seqEpoch) return null;
  if (event.updateSeq <= snapshot.updateSeq) return snapshot;
  if (event.updateSeq !== snapshot.updateSeq + 1) return null;

  const next: Snapshot = {
    ...snapshot,
    updateSeq: event.updateSeq,
    entries: [...snapshot.entries],
    runs: snapshot.runs.map((run) => ({
      ...run,
      drafts: [...(run.drafts ?? [])],
    })),
  };
  let run = next.runs.find((item) => item.runID === event.runID);
  if (!run) {
    // Runner 先落首条用户输入、发 message，再发 run-started。
    if (event.kind !== "run-started" && event.kind !== "message") return null;
    run = {
      runID: event.runID,
      afterEntrySeq: event.afterEntrySeq ?? 0,
      status: "running",
      drafts: [],
    };
    next.runs.push(run);
  }

  switch (event.kind) {
    case "message-started":
    case "text-delta":
    case "reasoning-delta": {
      if (!event.entryID) return null;
      if (next.entries.some((entry) => entry.id === event.entryID)) break;
      if (run.status !== "running") break;
      const drafts = run.drafts!;
      const index = drafts.findIndex(
        (draft) => draft.entryID === event.entryID,
      );
      const previous = drafts[index];
      const draft = {
        entryID: event.entryID,
        afterEntrySeq:
          previous?.afterEntrySeq ?? event.afterEntrySeq ?? run.afterEntrySeq,
        blocks: [...(previous?.blocks ?? [])],
      };
      if (event.kind !== "message-started") {
        if (!event.blockSeq || event.blockSeq < 1) return null;
        const position = event.blockSeq - 1;
        // 块索引不改写；尚未到达的工具等块留空位，完整 Entry 会整体替换。
        while (draft.blocks.length <= position)
          draft.blocks.push({ kind: "pending" });
        const old = draft.blocks[position];
        draft.blocks[position] = {
          kind: event.kind === "text-delta" ? "text" : "reasoning",
          text: (old.text ?? "") + (event.text ?? ""),
        };
      }
      if (index < 0) drafts.push(draft);
      else drafts[index] = draft;
      break;
    }
    case "message": {
      if (!event.entry) return null;
      const index = next.entries.findIndex(
        (entry) => entry.id === event.entry!.id,
      );
      if (index < 0) next.entries.push(event.entry);
      else next.entries[index] = event.entry;
      next.entries.sort((a, b) => a.seq - b.seq);
      run.drafts = run.drafts!.filter(
        (draft) => draft.entryID !== event.entry!.id,
      );
      break;
    }
    case "run-ended":
      if (!event.status || event.status === "running") return null;
      run.status = event.status;
      run.error = event.error;
      // 半截正文由后台以 incomplete Entry 保存；没有落账的内容不能假装耐久。
      run.drafts = [];
      break;
    case "usage":
      if (!event.usage) return null;
      run.usage = event.usage;
      break;
    // 工具关联保存在 Entry.blocks 中。
    case "tool-started":
    case "tool-finished":
    case "run-started":
      break;
  }
  return next;
}

export interface ChatMessage {
  id: string;
  message: Message;
  draft: boolean;
  position: number;
  seq?: number;
}

// 只派生展示次序：用户按落账顺序，助手按生成开始的锚点；不重新分配身份。
export function chatMessages(snapshot: Snapshot): ChatMessage[] {
  const messages: ChatMessage[] = snapshot.entries.map((entry: Entry) => ({
    id: entry.id,
    message: entry.message,
    draft: false,
    position: entry.message.afterSeq ?? entry.seq,
    seq: entry.seq,
  }));
  const durableIDs = new Set<string>();
  const toolCalls = new Map<string, ChatMessage>();
  for (const item of messages) {
    durableIDs.add(item.id);
    for (const block of item.message.blocks) {
      if (block.tool && !toolCalls.has(block.tool.id)) {
        toolCalls.set(block.tool.id, item);
      }
    }
  }
  for (const item of messages) {
    const resultID = item.message.blocks.find((block) => block.result)?.result
      ?.id;
    if (!resultID) continue;
    const call = toolCalls.get(resultID);
    if (call) item.position = call.position;
  }
  for (const run of snapshot.runs) {
    for (const draft of run.drafts ?? []) {
      if (durableIDs.has(draft.entryID)) continue;
      messages.push({
        id: draft.entryID,
        message: {
          role: "assistant",
          runID: run.runID,
          afterSeq: draft.afterEntrySeq,
          blocks: draft.blocks,
        },
        draft: true,
        position: draft.afterEntrySeq,
      });
    }
  }
  return messages.sort(
    (a, b) =>
      a.position - b.position || (a.seq ?? Infinity) - (b.seq ?? Infinity),
  );
}

export function activeRun(snapshot: Snapshot | null): RunState | undefined {
  return snapshot?.runs.find((run) => run.status === "running");
}

export function latestUsage(snapshot: Snapshot | null): RunState["usage"] {
  if (!snapshot) return undefined;
  const running = activeRun(snapshot);
  if (running?.usage) return running.usage;
  for (let index = snapshot.runs.length - 1; index >= 0; index--) {
    if (snapshot.runs[index].usage) return snapshot.runs[index].usage;
  }
  return undefined;
}

export function runLabel(status?: RunState["status"]): string {
  switch (status) {
    case "running":
      return "运行中";
    case "success":
      return "已完成";
    case "cancelled":
      return "已停止";
    case "failed":
      return "运行失败";
    case "interrupted":
      return "运行已中断";
    default:
      return "状态未记录";
  }
}

export function blockText(block: Block): string {
  if (block.kind === "image") return "[图片]";
  if (block.kind === "tool-call")
    return `工具调用：${block.tool?.name ?? "未知工具"}`;
  if (block.kind === "tool-result")
    return `工具结果：${block.result?.name ?? "未知工具"}`;
  return block.text ?? "";
}
