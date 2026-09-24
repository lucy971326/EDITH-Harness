import { useCallback, useEffect, useRef, useState } from "react";
import { RPCError, type RPCClient } from "../client/rpc";
import {
  applyExternalFile,
  applySavedFile,
  decodeFile,
  encodeFile,
  fileConflictCode,
  fileName,
  projectEditorState,
  samePath,
  type EditorFile,
  type ProjectEditorState,
} from "./files";

type Watch = { kind: "file" | "directory"; subscriptionID: string };

// 工作区草稿留在本页；文件读写、自动保存和监听在这里共同收尾。
export function useEditorFiles(
  client: RPCClient | null,
  workspace: string | null,
) {
  const projects = useRef(new Map<string, ProjectEditorState>());
  const watches = useRef(new Map<string, Watch>());
  const timers = useRef(new Map<EditorFile, ReturnType<typeof setTimeout>>());
  const saving = useRef(new Map<EditorFile, Promise<void>>());
  const visibleWorkspace = useRef(workspace);
  const mounted = useRef(true);
  visibleWorkspace.current = workspace;
  const [, setRevision] = useState(0);
  const [treeRevision, setTreeRevision] = useState(0);
  const project = workspace
    ? projectEditorState(projects.current, workspace)
    : undefined;

  function render() {
    if (mounted.current) setRevision((value) => value + 1);
  }
  function owns(file: EditorFile): boolean {
    return mounted.current && project?.files.get(file.path) === file;
  }

  const watch = useCallback(
    async (path: string, kind: Watch["kind"]) => {
      if (
        !client?.connected ||
        !workspace ||
        visibleWorkspace.current !== workspace ||
        watches.current.has(path)
      )
        return;
      const target: Watch = { kind, subscriptionID: "" };
      watches.current.set(path, target);
      try {
        await client.watchFile(path, (subscriptionID) => {
          if (!client.connected || watches.current.get(path) !== target) {
            void client.unsubscribe(subscriptionID).catch(() => {});
            return;
          }
          target.subscriptionID = subscriptionID;
        });
      } catch {
        if (watches.current.get(path) === target) watches.current.delete(path);
      }
    },
    [client, workspace],
  );

  async function refreshFromDisk(file: EditorFile) {
    while (client?.connected && owns(file)) {
      // 等写入完成再读，避免把自己的保存通知识别成外部冲突。
      await saving.current.get(file);
      if (!client.connected || !owns(file)) return;
      const hash = file.hash;
      try {
        const result = await client.readFile(file.path);
        if (!owns(file)) return;
        if (saving.current.has(file) || file.hash !== hash) continue;
        applyExternalFile(file, decodeFile(result.dataBase64), result.hash);
      } catch (error) {
        if (!owns(file)) return;
        if (saving.current.has(file) || file.hash !== hash) continue;
        file.status =
          error instanceof RPCError && error.code === -32004
            ? "missing"
            : "error";
        file.error =
          file.status === "missing"
            ? "文件已被删除；草稿仍保留在本页。"
            : error instanceof Error
              ? error.message
              : "重新读取失败";
      }
      render();
      return;
    }
  }

  function scheduleSave(file: EditorFile) {
    const previous = timers.current.get(file);
    if (previous) clearTimeout(previous);
    timers.current.set(
      file,
      setTimeout(() => {
        timers.current.delete(file);
        void saveFile(file.path);
      }, 700),
    );
  }

  async function saveFile(path: string, overwriteHash?: string) {
    const file = project?.files.get(path);
    if (!file || !client?.connected || saving.current.has(file)) return;
    if (!file.hash || file.status === "missing" || file.status === "loading")
      return;
    if (file.status === "conflict" && !overwriteHash) return;
    if (file.content === file.savedContent && !overwriteHash) return;
    const submitted = file.content;
    file.status = "saving";
    delete file.error;
    render();
    const pending = (async () => {
      try {
        const result = await client.writeFile(
          path,
          encodeFile(submitted),
          overwriteHash ?? file.hash,
        );
        if (owns(file)) applySavedFile(file, submitted, result.hash);
      } catch (error) {
        if (!owns(file)) return;
        if (error instanceof RPCError && error.code === fileConflictCode) {
          // 写入已被拒绝，直接读磁盘；不能等待正在执行的保存自身。
          file.status = "dirty";
          try {
            const result = await client.readFile(path);
            if (owns(file))
              applyExternalFile(
                file,
                decodeFile(result.dataBase64),
                result.hash,
              );
          } catch (readError) {
            if (!owns(file)) return;
            file.status =
              readError instanceof RPCError && readError.code === -32004
                ? "missing"
                : "error";
            file.error =
              file.status === "missing"
                ? "文件已被删除；草稿仍保留在本页。"
                : readError instanceof Error
                  ? readError.message
                  : "重新读取失败";
          }
        } else {
          file.status = "error";
          file.error = error instanceof Error ? error.message : "保存失败";
        }
      } finally {
        saving.current.delete(file);
        if (owns(file)) {
          if (file.status === "dirty") scheduleSave(file);
          render();
        }
      }
    })();
    saving.current.set(file, pending);
    await pending;
  }

  useEffect(() => {
    if (!client?.connected || !workspace) return;
    client.onFileChanged = ({ subscriptionID, event }) => {
      const target = [...watches.current.values()].find(
        (item) => item.subscriptionID === subscriptionID,
      );
      if (!target) return;
      if (target.kind === "directory") setTreeRevision((value) => value + 1);
      for (const file of project!.files.values()) {
        if (event.changedPaths.some((path) => samePath(path, file.path)))
          void refreshFromDisk(file);
      }
    };
    void watch(workspace, "directory");
    for (const file of project!.files.values()) {
      void watch(file.path, "file");
      if (
        file.content !== file.savedContent &&
        file.status !== "conflict" &&
        file.status !== "missing"
      )
        scheduleSave(file);
    }
    return () => {
      client.onFileChanged = null;
      for (const target of watches.current.values()) {
        if (target.subscriptionID && client.connected)
          void client.unsubscribe(target.subscriptionID).catch(() => {});
      }
      watches.current.clear();
    };
  }, [client, workspace, watch]);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      for (const timer of timers.current.values()) clearTimeout(timer);
      timers.current.clear();
    };
  }, []);

  async function openFile(path: string) {
    if (!project || !client?.connected) return;
    project.activePath = path;
    if (project.files.has(path)) {
      render();
      return;
    }
    const file: EditorFile = {
      path,
      name: fileName(path),
      content: "",
      savedContent: "",
      hash: "",
      status: "loading",
    };
    project.files.set(path, file);
    project.order.push(path);
    render();
    try {
      const result = await client.readFile(path);
      if (!owns(file)) return;
      file.content = file.savedContent = decodeFile(result.dataBase64);
      file.hash = result.hash;
      file.status = "saved";
      void watch(path, "file");
    } catch (error) {
      if (!owns(file)) return;
      file.status = "error";
      file.error = error instanceof Error ? error.message : "文件读取失败";
    }
    render();
  }

  function editFile(path: string, content: string) {
    const file = project?.files.get(path);
    if (!file) return;
    file.content = content;
    if (file.status !== "conflict")
      file.status = content === file.savedContent ? "saved" : "dirty";
    delete file.error;
    scheduleSave(file);
    render();
  }

  function closeFile(path: string) {
    const file = project?.files.get(path);
    if (!file || !project) return;
    const index = project.order.indexOf(path);
    project.files.delete(path);
    project.order = project.order.filter((item) => item !== path);
    if (project.activePath === path)
      project.activePath =
        project.order[Math.min(index, project.order.length - 1)] ?? "";
    const timer = timers.current.get(file);
    if (timer) clearTimeout(timer);
    timers.current.delete(file);
    const target = watches.current.get(path);
    watches.current.delete(path);
    if (target?.subscriptionID && client?.connected)
      void client.unsubscribe(target.subscriptionID).catch(() => {});
    render();
  }

  function reloadConflict(path: string) {
    const file = project?.files.get(path);
    if (!file || file.diskContent == null || !file.diskHash) return;
    file.content = file.savedContent = file.diskContent;
    file.hash = file.diskHash;
    file.status = "saved";
    delete file.diskContent;
    delete file.diskHash;
    delete file.error;
    render();
  }

  return {
    project,
    treeRevision,
    openFile,
    editFile,
    saveFile,
    closeFile,
    reloadConflict,
    watchDirectory: useCallback(
      (path: string) => {
        void watch(path, "directory");
      },
      [watch],
    ),
    selectFile(path: string) {
      if (project) {
        project.activePath = path;
        render();
      }
    },
  };
}
