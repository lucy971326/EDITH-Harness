// 引用只属于前端草稿；发送时变成普通用户文本，账本无需新增类型。
export type ContextReference =
  | { kind: "file" | "directory"; path: string }
  | {
      kind: "selection";
      path: string;
      startLine: number;
      startColumn: number;
      endLine: number;
      endColumn: number;
      content: string;
    }
  | {
      kind: "assistant-selection";
      entryID: string;
      startOffset: number;
      endOffset: number;
      content: string;
      comment?: string;
    };

export interface ReferenceAttachment {
  id: string;
  reference: ContextReference;
}

const referenceV1Start = "\n\n引用上下文（harness-context-v1）：\n";
const referenceV2Start = "\n\n引用上下文（harness-context-v2）：\n";
const referenceEnd = "\n（上下文引用结束）";

export function encodeReferences(text: string, references: ContextReference[]): string {
  if (!references.length) return text;
  // JSON 转义正文中的换行、引号和边界文字；模型与历史使用同一份原文。
  const start = references.some((reference) => reference.kind === "assistant-selection")
    ? referenceV2Start : referenceV1Start;
  return text + start + JSON.stringify(references) + referenceEnd;
}

export function decodeReferences(text: string): { text: string; references: ContextReference[] } {
  const unchanged = { text, references: [] as ContextReference[] };
  if (!text.endsWith(referenceEnd)) return unchanged;
  const v2 = text.lastIndexOf(referenceV2Start);
  const v1 = text.lastIndexOf(referenceV1Start);
  const start = Math.max(v1, v2);
  if (start < 0) return unchanged;
  try {
    const marker = start === v2 ? referenceV2Start : referenceV1Start;
    const raw = text.slice(start + marker.length, -referenceEnd.length);
    const references: unknown = JSON.parse(raw);
    const valid = start === v2 ? isReference : isV1Reference;
    if (!Array.isArray(references) || !references.length || !references.every(valid))
      return unchanged;
    return { text: text.slice(0, start), references };
  } catch {
    // 未知版本、残缺或普通手写内容原样显示，不能为了美化吞掉用户消息。
    return unchanged;
  }
}

function isReference(value: unknown): value is ContextReference {
  if (!value || typeof value !== "object") return false;
  const item = value as Record<string, unknown>;
  if (item.kind === "assistant-selection") {
    if (typeof item.entryID !== "string" || !item.entryID || item.entryID.includes("\0") ||
        typeof item.content !== "string" || !item.content)
      return false;
    if (!Number.isSafeInteger(item.startOffset) || !Number.isSafeInteger(item.endOffset) ||
        (item.startOffset as number) < 0 || (item.endOffset as number) <= (item.startOffset as number))
      return false;
    return Object.keys(item).every((key) =>
      ["kind", "entryID", "startOffset", "endOffset", "content", "comment"].includes(key)) &&
      (item.comment === undefined || (typeof item.comment === "string" && !!item.comment.trim()));
  }
  if (typeof item.path !== "string" || !item.path || item.path.includes("\0")) return false;
  if (item.kind === "file" || item.kind === "directory")
    return Object.keys(item).every((key) => key === "kind" || key === "path");
  if (item.kind !== "selection" || typeof item.content !== "string" || !item.content) return false;
  const positions = [item.startLine, item.startColumn, item.endLine, item.endColumn];
  if (!positions.every((position) => typeof position === "number" && Number.isSafeInteger(position) && position > 0))
    return false;
  const selection = item as unknown as Extract<ContextReference, { kind: "selection" }>;
  if (selection.endLine < selection.startLine ||
      (selection.endLine === selection.startLine && selection.endColumn <= selection.startColumn))
    return false;
  return Object.keys(item).every((key) =>
    ["kind", "path", "startLine", "startColumn", "endLine", "endColumn", "content"].includes(key));
}

function isV1Reference(value: unknown): value is ContextReference {
  return isReference(value) && value.kind !== "assistant-selection";
}

export function addReference(current: ReferenceAttachment[], reference: ContextReference): ReferenceAttachment[] {
  if (current.some((item) => sameReference(item.reference, reference))) return current;
  return [...current, { id: crypto.randomUUID(), reference }];
}

function sameReference(left: ContextReference, right: ContextReference): boolean {
  if (left.kind !== right.kind) return false;
  if (left.kind === "assistant-selection" && right.kind === "assistant-selection")
    return left.entryID === right.entryID && left.startOffset === right.startOffset &&
      left.endOffset === right.endOffset && left.content === right.content;
  if (left.kind === "assistant-selection" || right.kind === "assistant-selection") return false;
  if (left.path !== right.path) return false;
  if (left.kind !== "selection" || right.kind !== "selection") return true;
  return left.startLine === right.startLine && left.startColumn === right.startColumn &&
    left.endLine === right.endLine && left.endColumn === right.endColumn && left.content === right.content;
}

// 确认按附件身份清理；删除后重新添加的同一选区也属于新编辑，应当留下。
export function clearSubmittedReferences(current: ReferenceAttachment[], submitted: ReferenceAttachment[]): ReferenceAttachment[] {
  const ids = new Set(submitted.map((item) => item.id));
  return current.filter((item) => !ids.has(item.id));
}

export function referencePath(workspace: string, path: string): string {
  const normalize = (value: string) => {
    const slashes = value.replace(/\\/g, "/");
    const prefix = slashes.startsWith("//") ? "//" : slashes.startsWith("/") ? "/" : "";
    const parts: string[] = [];
    for (const part of slashes.split("/")) {
      if (!part || part === ".") continue;
      if (part === ".." && parts.length && parts.at(-1) !== "..") parts.pop();
      else parts.push(part);
    }
    return prefix + parts.join("/");
  };
  const root = normalize(workspace);
  const target = normalize(path);
  const windows = /^[a-z]:\//i.test(root) || root.startsWith("//");
  const comparedRoot = windows ? root.toLowerCase() : root;
  const comparedTarget = windows ? target.toLowerCase() : target;
  if (comparedTarget === comparedRoot) return ".";
  const prefix = comparedRoot.endsWith("/") ? comparedRoot : comparedRoot + "/";
  if (comparedTarget.startsWith(prefix)) return target.slice(prefix.length);
  return target;
}

export function referenceLabel(reference: ContextReference): string {
  if (reference.kind === "assistant-selection") return "引用";
  if (reference.kind !== "selection") return reference.path;
  // Monaco 的结束位置不包含该字符；终点为下一行列 1 时，该行不属于选区。
  const lastLine = reference.endColumn === 1 && reference.endLine > reference.startLine
    ? reference.endLine - 1 : reference.endLine;
  return `${reference.path}:${reference.startLine}${lastLine > reference.startLine ? `–${lastLine}` : ""}`;
}
