import type { Block, RunState, Snapshot } from "../../../contracts/run.ts";
import { chatMessages, type ChatMessage } from "./chat.ts";

// 只读展示结构；身份和内容来自 Snapshot，不另存聊天账本。
export interface ProcessItem {
  id: string;
  kind:
    | "text"
    | "steer"
    | "tool"
    | "detail"
    | "reasoning"
    | "collaboration"
    | "image";
  title: string;
  text: string;
  status?: string;
  media?: NonNullable<Block["media"]>;
}
export interface ChatTurn {
  id: string;
  standalone: boolean;
  run?: RunState;
  prompt?: ChatMessage;
  items: ProcessItem[];
  answer?: { id: string; text: string };
}

export function chatTurns(snapshot: Snapshot): ChatTurn[] {
  const runs = new Map(snapshot.runs.map((run) => [run.runID, run]));
  const groups = new Map<string, ChatMessage[]>();
  const results = new Map<string, NonNullable<Block["result"]>>();
  const calls = new Set<string>();
  for (const item of chatMessages(snapshot)) {
    const id = item.message.runID ?? `entry:${item.id}`;
    const group = groups.get(id);
    if (group) group.push(item);
    else groups.set(id, [item]);
    for (const block of item.message.blocks) {
      if (block.result) results.set(`${id}:${block.result.id}`, block.result);
      if (block.tool) calls.add(`${id}:${block.tool.id}`);
    }
  }
  for (const run of snapshot.runs) {
    if (!groups.has(run.runID)) groups.set(run.runID, []);
  }

  return Array.from(groups, ([id, messages]) => {
    const run = runs.get(id);
    const prompt = messages.find((item) => item.message.role === "user");
    // 最终身份按落账次序判断，不能按显示锚点或正文措辞往前挑。
    let lastAssistant: ChatMessage | undefined;
    let lastInputSeq = 0;
    for (const item of messages) {
      const role = item.message.role;
      if (role === "user" || role === "collaboration")
        lastInputSeq = Math.max(lastInputSeq, item.seq ?? 0);
      if (
        role === "assistant" &&
        !item.draft &&
        (!lastAssistant || item.seq! > lastAssistant.seq!)
      )
        lastAssistant = item;
    }
    const candidate = lastAssistant;
    const answerText =
      candidate?.message.blocks
        .filter((block) => block.kind === "text")
        .map((block) => block.text ?? "")
        .join("\n\n") ?? "";
    // afterSeq 是已消费输入的锚点，防止插话后空输出借用旧回答。
    const answeredInput =
      candidate &&
      (candidate.message.afterSeq ?? candidate.seq ?? 0) >= lastInputSeq;
    // runs.json 出现前的旧历史没有运行状态；完整落账的最后一条助手正文仍是回答。
    const completed = run?.status === "success" || run === undefined;
    const answer =
      completed &&
      candidate &&
      answeredInput &&
      !candidate.message.incomplete &&
      answerText.trim() &&
      !candidate.message.blocks.some(
        (block) =>
          block.tool || block.kind === "tool-call" || block.kind === "summary",
      )
        ? { id: candidate.id, text: answerText }
        : undefined;

    const items: ProcessItem[] = [];
    for (const item of messages) {
      if (item === prompt) continue;
      for (const [index, block] of item.message.blocks.entries()) {
        if (block.kind === "pending") continue;
        if (answer?.id === item.id && block.kind === "text") continue;
        const itemID = `${item.id}:${index}`;
        if (block.tool) {
          const result = results.get(`${id}:${block.tool.id}`);
          items.push({
            id: itemID,
            kind: "tool",
            title: block.tool.name,
            status: result
              ? result.isError
                ? "异常"
                : "已完成"
              : run?.status === "running"
                ? "等待结果"
                : run
                  ? "结果未记录"
                  : "状态未记录",
            text: `参数\n${block.tool.args}\n\n结果\n${result?.content ?? "尚无结果记录"}`,
          });
        } else if (block.result) {
          // 已配对结果回填卡片；孤立结果原位保留。
          if (!calls.has(`${id}:${block.result.id}`))
            items.push({
              id: itemID,
              kind: "detail",
              title: `${block.result.name} · 独立工具结果`,
              text: block.result.content,
              status: block.result.isError ? "异常" : "已完成",
            });
        } else {
          const role = item.message.role;
          const detail =
            block.kind !== "text" ||
            role === "collaboration" ||
            role === "system";
          items.push({
            id: itemID,
            kind:
              block.kind === "image" && block.media
                ? "image"
                : block.kind === "reasoning"
                  ? "reasoning"
                  : role === "collaboration"
                    ? "collaboration"
                  : detail
                    ? "detail"
                    : role === "user"
                      ? "steer"
                      : "text",
            title:
              block.kind === "reasoning"
                ? "思考"
                : block.kind === "summary"
                  ? "上下文压缩"
                  : role === "collaboration"
                    ? "子任务回报"
                    : role === "system"
                      ? "系统记录"
                      : block.kind === "image"
                        ? "图片（第 5 步接入）"
                        : block.kind,
            text:
              block.text ??
              block.error ??
              (block.kind === "image" ? "图片" : JSON.stringify(block)),
            media: block.media,
            status: item.message.incomplete
              ? "未完成"
              : item.draft
                ? "生成中"
                : undefined,
          });
        }
      }
    }
    return {
      id,
      standalone: !messages[0]?.message.runID && !run,
      run,
      prompt,
      items,
      answer,
    };
  });
}

// 只合并真正连续的工具调用；其他条目保持账本里的同级顺序。
export function processGroups(
  items: ProcessItem[],
): (ProcessItem | ProcessItem[])[] {
  const groups: (ProcessItem | ProcessItem[])[] = [];
  for (const item of items) {
    if (item.kind === "tool") {
      const last = groups.at(-1);
      if (Array.isArray(last)) last.push(item);
      else groups.push([item]);
    } else groups.push(item);
  }
  return groups;
}
