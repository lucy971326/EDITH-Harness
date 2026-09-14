export const fileConflictCode = -32009;

export type FileStatus =
  | "loading"
  | "saved"
  | "dirty"
  | "saving"
  | "conflict"
  | "missing"
  | "error";

export interface EditorFile {
  path: string;
  name: string;
  content: string;
  savedContent: string;
  hash: string;
  status: FileStatus;
  error?: string;
  diskContent?: string;
  diskHash?: string;
}

export interface ProjectEditorState {
  files: Map<string, EditorFile>;
  order: string[];
  activePath: string;
}

export function projectEditorState(
  projects: Map<string, ProjectEditorState>,
  workspace: string,
): ProjectEditorState {
  let project = projects.get(workspace);
  if (!project) {
    project = { files: new Map(), order: [], activePath: "" };
    projects.set(workspace, project);
  }
  return project;
}

export function decodeFile(dataBase64: string): string {
  const binary = atob(dataBase64);
  const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  } catch {
    throw new Error("该文件不是有效的 UTF-8 文本，无法安全编辑。");
  }
}

export function encodeFile(content: string): string {
  const bytes = new TextEncoder().encode(content);
  let binary = "";
  for (let offset = 0; offset < bytes.length; offset += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + 0x8000));
  }
  return btoa(binary);
}

export function fileName(path: string): string {
  return path.split(/[\\/]/).filter(Boolean).at(-1) ?? path;
}

export function joinPath(parent: string, child: string): string {
  const separator = parent.includes("\\") ? "\\" : "/";
  return `${parent.replace(/[\\/]$/, "")}${separator}${child}`;
}

export function samePath(left: string, right: string): boolean {
  const normalize = (value: string) => value.replace(/\\/g, "/").toLowerCase();
  return normalize(left) === normalize(right);
}

export function statusLabel(file: EditorFile): string {
  if (file.status === "dirty") return "未保存";
  if (file.status === "saving") return "正在保存";
  if (file.status === "conflict") return "文件冲突";
  if (file.status === "missing") return "文件已删除";
  if (file.status === "error") return file.error ?? "保存失败";
  return "已保存";
}

export function applySavedFile(
  file: EditorFile,
  submittedContent: string,
  hash: string,
) {
  file.hash = hash;
  file.savedContent = submittedContent;
  delete file.diskContent;
  delete file.diskHash;
  file.status = file.content === submittedContent ? "saved" : "dirty";
}

export function applyExternalFile(
  file: EditorFile,
  content: string,
  hash: string,
): "unchanged" | "reloaded" | "conflict" {
  if (hash === file.hash) return "unchanged";
  if (file.content !== file.savedContent || file.status === "conflict") {
    file.status = "conflict";
    file.diskContent = content;
    file.diskHash = hash;
    file.error = "磁盘内容已变化";
    return "conflict";
  }
  file.content = content;
  file.savedContent = content;
  file.hash = hash;
  file.status = "saved";
  delete file.error;
  return "reloaded";
}

export function needsCloseConfirmation(file: EditorFile): boolean {
  return (
    file.content !== file.savedContent ||
    file.status === "saving" ||
    file.status === "conflict" ||
    file.status === "error"
  );
}
