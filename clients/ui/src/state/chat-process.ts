import type { Block, RunState, Snapshot } from "../../../contracts/run.ts";
import { chatMessages, type ChatMessage } from "./chat.ts";

// 只读展示结构；身份和内容来自 Snapshot，不另存聊天账本。
export interface ProcessItem {
  id: string;
  kind:
    | "text"
    | "steer"
    | "tool"
    | "subagent"
    | "detail"
    | "reasoning"
    | "collaboration"
    | "image";
  title: string;
  text: string;
  preview?: string;
  status?: string;
  media?: NonNullable<Block["media"]>;
  taskID?: string;
  taskName?: string;
}
export interface ChatTurn {
  id: string;
  standalone: boolean;
  run?: RunState;
  prompt?: ChatMessage;
  items: ProcessItem[];
  answer?: { id: string; text: string };
}

function shortText(value: string): string | undefined {
  const text = value.replace(/\s+/g, " ").replace(/^\*\*|\*\*$/g, "").trim();
  return text ? text.slice(0, 96) : undefined;
}

function commandFromArgs(args: string): string | undefined {
  try {
    const input = JSON.parse(args) as Record<string, unknown>;
    const command = input?.cmd ?? input?.command;
    return typeof command === "string" ? command.trim() : undefined;
  } catch {
    return undefined;
  }
}

function toolPreview(name: string, args: string): string | undefined {
  if (!["exec_command", "bash", "read", "write", "edit", "apply_patch", "subagent_spawn"].includes(name))
    return undefined;
  try {
    const input = JSON.parse(args) as Record<string, unknown>;
    if (!input || typeof input !== "object" || Array.isArray(input)) return undefined;
    if (name === "apply_patch" && typeof input.patch === "string") {
      const file = input.patch.match(/^\*\*\* (?:Add|Update|Delete) File: (.+)$/m)?.[1];
      return file ? shortText(file.split(/[\\/]/).pop() ?? file) : undefined;
    }
    if (name === "subagent_spawn")
      return typeof input.taskName === "string" ? shortText(input.taskName) : undefined;
    if (name === "exec_command" || name === "bash") {
      const command = commandFromArgs(args);
      if (!command) return undefined;
      const line = command.split(/\r?\n/, 1)[0].trim();
      // 折叠行会直接显示命令；疑似凭据留在展开详情中。
      if (/(?:token|secret|password|api[_-]?key|authorization|bearer|cookie|credential|-----BEGIN)/i.test(line))
        return undefined;
      return shortText(line);
    }
    const path = input.path ?? input.file_path ?? input.filePath;
    return typeof path === "string" ? shortText(path) : undefined;
  } catch {
    return undefined;
  }
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
    const completed = run?.status === "success";
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

    const activeDraftID = run?.status === "running"
      ? run.drafts?.at(-1)?.entryID
      : undefined;
    const items: ProcessItem[] = [];
    for (const item of messages) {
      if (item === prompt) continue;
      for (const [index, block] of item.message.blocks.entries()) {
        if (block.kind === "pending") continue;
        if (answer?.id === item.id && block.kind === "text") continue;
        const itemID = `${item.id}:${index}`;
        if (block.kind === "reasoning") {
          // 草稿覆盖整条消息；后续正文开始后，前面的思考已不再活跃。
          // 只投影当前末尾的思考文字，落账原文和续接数据仍保留在 Snapshot。
          if (
            item.draft &&
            item.id === activeDraftID &&
            index === item.message.blocks.length - 1 &&
            block.text?.trim()
          ) {
            items.push({
              id: itemID,
              kind: "reasoning",
              title: "思考中",
              text: block.text,
              preview: shortText(block.text),
              status: "生成中",
            });
          }
          continue;
        }
        if (block.tool) {
          const result = results.get(`${id}:${block.tool.id}`);
          const child = subagentFromTool(
            block.tool.name,
            block.tool.args,
            result?.content,
          );
          items.push({
            id: itemID,
            kind: child ? "subagent" : "tool",
            title: block.tool.name,
            preview: toolPreview(block.tool.name, block.tool.args),
            status: result
              ? result.isError
                ? "异常"
                : "已完成"
              : run?.status === "running"
                ? "等待结果"
                : run
                  ? "结果未记录"
                  : "状态未记录",
            text: block.tool.name === "exec_command" || block.tool.name === "bash"
              ? `$ ${commandFromArgs(block.tool.args) ?? block.tool.args}\n\n${result?.content ?? "尚无结果记录"}`
              : `参数\n${block.tool.args}\n\n结果\n${result?.content ?? "尚无结果记录"}`,
            taskID: child?.taskID,
            taskName: child?.taskName,
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
                : role === "collaboration"
                  ? "collaboration"
                  : detail
                    ? "detail"
                    : role === "user"
                      ? "steer"
                      : "text",
            title:
              block.kind === "summary"
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
            taskID:
              role === "collaboration" ? item.message.sourceTaskID : undefined,
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

function subagentFromTool(name: string, args: string, result?: string) {
  if (name !== "subagent_spawn" || !result) return undefined;
  try {
    const input = JSON.parse(args) as { taskName?: unknown };
    const output = JSON.parse(result) as { taskID?: unknown };
    if (typeof output.taskID !== "string" || output.taskID === "")
      return undefined;
    return {
      taskID: output.taskID,
      taskName:
        typeof input.taskName === "string" && input.taskName.trim()
          ? input.taskName.trim()
          : "Subagent",
    };
  } catch {
    return undefined;
  }
}

// 连续终端命令组成一个可展开的操作组；单条命令也保留这一级。
export function processGroups(
  items: ProcessItem[],
): (ProcessItem | ProcessItem[])[] {
  const groups: (ProcessItem | ProcessItem[])[] = [];
  let commands: ProcessItem[] = [];
  const flush = () => {
    if (commands.length > 0) groups.push(commands);
    commands = [];
  };
  for (const item of items) {
    if (item.kind === "tool" && (item.title === "exec_command" || item.title === "bash" || item.title === "write_stdin"))
      commands.push(item);
    else {
      flush();
      groups.push(item);
    }
  }
  flush();
  return groups;
}
