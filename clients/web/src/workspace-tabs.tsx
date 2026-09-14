import {
  lazy,
  Suspense,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import { Button } from "@/components/ui/button";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { ResizeHandle } from "@/components/resize-handle";
import {
  AuxiliaryPanel,
  type AuxiliaryTab,
  type AuxiliaryView,
} from "./auxiliary/auxiliary-panel";
import type { RPCClient } from "./client/rpc";
import { RPCError } from "./client/rpc";
import { FileTree } from "./editor/file-tree";
import {
  decodeFile,
  encodeFile,
  applyExternalFile,
  applySavedFile,
  fileConflictCode,
  fileName,
  samePath,
  needsCloseConfirmation,
  projectEditorState,
  statusLabel,
  type EditorFile,
  type ProjectEditorState,
} from "./editor/files";
import { FileText, PanelRight } from "./icons";
import type { FileLocation } from "./editor/links";

const CodeEditor = lazy(() =>
  import("./editor/code-editor").then((module) => ({
    default: module.CodeEditor,
  })),
);

interface WatchTarget {
  path: string;
  kind: "file" | "directory";
  subscriptionID: string;
}

function storedTreeWidth(): number {
  try {
    return Math.max(
      180,
      Math.min(
        520,
        Number(localStorage.getItem("harness-web:file-tree-width")) || 280,
      ),
    );
  } catch {
    return 280;
  }
}

export function WorkspaceTabs({
  workspace,
  client,
  openRequest,
  onHide,
}: {
  workspace: string | null;
  client: RPCClient | null;
  openRequest?: FileLocation & { requestID: number; workspace: string };
  onHide: () => void;
}) {
  const projects = useRef(new Map<string, ProjectEditorState>());
  const watches = useRef(new Map<string, WatchTarget>());
  const watchPaths = useRef(new Map<string, string>());
  const saveTimers = useRef(new Map<string, ReturnType<typeof setTimeout>>());
  const saving = useRef(new Set<string>());
  const [, setRevision] = useState(0);
  const [treeRevision, setTreeRevision] = useState(0);
  const [treeOpen, setTreeOpen] = useState(true);
  const [treeWidth, setTreeWidth] = useState(storedTreeWidth);
  const [closingPath, setClosingPath] = useState("");
  const [location, setLocation] = useState<FileLocation>();

  const project = workspace ? projects.current.get(workspace) : undefined;
  const activeFile = project?.files.get(project.activePath);

  function currentProject(): ProjectEditorState | null {
    if (!workspace) return null;
    return projectEditorState(projects.current, workspace);
  }

  function render() {
    setRevision((value) => value + 1);
  }

  function updateFile(path: string, update: (file: EditorFile) => void) {
    const file = currentProject()?.files.get(path);
    if (!file) return;
    update(file);
    render();
  }

  const clearWatches = useCallback((targetClient: RPCClient | null) => {
    const subscriptions = [...watches.current.values()];
    watches.current.clear();
    watchPaths.current.clear();
    if (!targetClient?.connected) return;
    for (const watch of subscriptions) {
      void targetClient.unsubscribe(watch.subscriptionID).catch(() => {});
    }
  }, []);

  const watch = useCallback(
    async (path: string, kind: WatchTarget["kind"]) => {
      if (!workspace || !client?.connected || watchPaths.current.has(path)) return;
      watchPaths.current.set(path, "pending");
      try {
        await client.watchFile(path, (subscriptionID) => {
          if (!client.connected || watchPaths.current.get(path) !== "pending") {
            void client.unsubscribe(subscriptionID).catch(() => {});
            return;
          }
          watchPaths.current.set(path, subscriptionID);
          watches.current.set(subscriptionID, { path, kind, subscriptionID });
        });
      } catch {
        watchPaths.current.delete(path);
      }
    },
    [client, workspace],
  );
  const watchDirectory = useCallback(
    (path: string) => {
      void watch(path, "directory");
    },
    [watch],
  );

  const refreshFromDisk = useCallback(
    async (path: string) => {
      if (!client?.connected) return;
      try {
        const result = await client.readFile(path);
        const content = decodeFile(result.dataBase64);
        updateFile(path, (file) => {
          if (file.status === "saving") {
            setTimeout(() => void refreshFromDisk(path), 120);
            return;
          }
          applyExternalFile(file, content, result.hash);
        });
      } catch (error) {
        updateFile(path, (file) => {
          if (error instanceof RPCError && error.code === -32004) {
            file.status = "missing";
            file.error = "文件已被删除；草稿仍保留在本页。";
            return;
          }
          file.status = "error";
          file.error = error instanceof Error ? error.message : "重新读取失败";
        });
      }
    },
    [client, workspace],
  );

  useEffect(() => {
    clearWatches(client);
    if (!client?.connected || !workspace) return;

    client.onFileChanged = ({ subscriptionID, event }) => {
      const target = watches.current.get(subscriptionID);
      if (!target) return;
      if (target.kind === "directory") setTreeRevision((value) => value + 1);
      const openFiles = currentProject()?.files;
      if (!openFiles) return;
      for (const changedPath of event.changedPaths) {
        for (const path of openFiles.keys()) {
          if (samePath(path, changedPath)) void refreshFromDisk(path);
        }
      }
    };
    void watch(workspace, "directory");
    for (const path of currentProject()?.order ?? []) {
      void watch(path, "file");
      const file = currentProject()?.files.get(path);
      if (
        file &&
        file.content !== file.savedContent &&
        file.status !== "conflict" &&
        file.status !== "missing"
      )
        scheduleSave(path);
    }

    return () => {
      client.onFileChanged = null;
      clearWatches(client);
    };
  }, [client, workspace, clearWatches, refreshFromDisk, watch]);

  useEffect(() => {
    try {
      localStorage.setItem("harness-web:file-tree-width", String(treeWidth));
    } catch {
      /* 本机偏好不可写时仅保留本页状态。 */
    }
  }, [treeWidth]);

  useEffect(
    () => () => {
      for (const timer of saveTimers.current.values()) clearTimeout(timer);
    },
    [],
  );

  useEffect(() => {
    if (!openRequest || !workspace || openRequest.workspace !== workspace) return;
    setLocation(openRequest);
    void openFile(openRequest.path, openRequest);
  }, [openRequest?.requestID, workspace, client]);

  async function openFile(path: string, target?: FileLocation) {
    const state = currentProject();
    if (!state || !client?.connected) return;
    if (state.files.has(path)) {
      state.activePath = path;
      setLocation(target);
      render();
      return;
    }
    state.files.set(path, {
      path,
      name: fileName(path),
      content: "",
      savedContent: "",
      hash: "",
      status: "loading",
    });
    state.order.push(path);
    state.activePath = path;
    setLocation(target);
    render();
    try {
      const result = await client.readFile(path);
      const content = decodeFile(result.dataBase64);
      updateFile(path, (file) => {
        file.content = content;
        file.savedContent = content;
        file.hash = result.hash;
        file.status = "saved";
      });
      void watch(path, "file");
    } catch (error) {
      updateFile(path, (file) => {
        file.status = "error";
        file.error = error instanceof Error ? error.message : "文件读取失败";
      });
    }
  }

  function scheduleSave(path: string) {
    const previous = saveTimers.current.get(path);
    if (previous) clearTimeout(previous);
    saveTimers.current.set(
      path,
      setTimeout(() => {
        saveTimers.current.delete(path);
        void saveFile(path);
      }, 700),
    );
  }

  function editFile(path: string, content: string) {
    updateFile(path, (file) => {
      file.content = content;
      if (file.status !== "conflict")
        file.status = content === file.savedContent ? "saved" : "dirty";
      delete file.error;
    });
    scheduleSave(path);
  }

  async function saveFile(path: string, overwriteHash?: string) {
    const file = currentProject()?.files.get(path);
    if (!file || !client?.connected || saving.current.has(path)) return;
    if (!file.hash || file.status === "missing" || file.status === "loading") return;
    if (file.content === file.savedContent && !overwriteHash) return;

    const submittedContent = file.content;
    const expectedHash = overwriteHash ?? file.hash;
    saving.current.add(path);
    file.status = "saving";
    delete file.error;
    render();
    try {
      const result = await client.writeFile(
        path,
        encodeFile(submittedContent),
        expectedHash,
      );
      updateFile(path, (current) => {
        applySavedFile(current, submittedContent, result.hash);
      });
    } catch (error) {
      if (error instanceof RPCError && error.code === fileConflictCode) {
        await refreshFromDisk(path);
      } else {
        updateFile(path, (current) => {
          current.status = "error";
          current.error = error instanceof Error ? error.message : "保存失败";
        });
      }
    } finally {
      saving.current.delete(path);
      const current = currentProject()?.files.get(path);
      if (current?.status === "dirty") scheduleSave(path);
    }
  }

  function requestClose(path: string) {
    const file = currentProject()?.files.get(path);
    if (!file) return;
    if (needsCloseConfirmation(file)) {
      setClosingPath(path);
      return;
    }
    closeFile(path);
  }

  function closeFile(path: string) {
    const state = currentProject();
    if (!state) return;
    const index = state.order.indexOf(path);
    state.files.delete(path);
    state.order = state.order.filter((item) => item !== path);
    if (state.activePath === path)
      state.activePath =
        state.order[Math.min(index, state.order.length - 1)] ?? "";
    const timer = saveTimers.current.get(path);
    if (timer) clearTimeout(timer);
    saveTimers.current.delete(path);
    const subscriptionID = watchPaths.current.get(path);
    watchPaths.current.delete(path);
    if (subscriptionID && subscriptionID !== "pending") {
      watches.current.delete(subscriptionID);
      if (client?.connected) void client.unsubscribe(subscriptionID).catch(() => {});
    }
    setClosingPath("");
    render();
  }

  function reloadConflict(path: string) {
    updateFile(path, (file) => {
      if (file.diskContent == null || !file.diskHash) return;
      file.content = file.diskContent;
      file.savedContent = file.diskContent;
      file.hash = file.diskHash;
      file.status = "saved";
      delete file.diskContent;
      delete file.diskHash;
      delete file.error;
    });
  }

  const tabs: AuxiliaryTab[] =
    project?.order.map((path) => {
      const file = project.files.get(path)!;
      return {
        id: path,
        kind: "file",
        title: file.name,
        contextPath: path,
        dirty:
          file.content !== file.savedContent || file.status === "conflict",
      };
    }) ?? [];

  const views: AuxiliaryView[] = [
    {
      kind: "file",
      label: "文件",
      icon: FileText,
      onCreate: () => setTreeOpen(true),
      render: () => (
        <div className="editor-view">
          <div className="editor-toolbar" data-status={activeFile?.status}>
            <span title={activeFile?.path ?? workspace ?? ""}>
              {editorBreadcrumb(workspace, activeFile?.path)}
            </span>
            {activeFile && <strong>{statusLabel(activeFile)}</strong>}
            <Button
              variant="ghost"
              size="icon"
              aria-label={treeOpen ? "收起文件树" : "展开文件树"}
              onClick={() => setTreeOpen(!treeOpen)}
            >
              <PanelRight />
            </Button>
          </div>

          <div className="editor-workspace">
            <section className="editor-pane" aria-label="代码编辑器">
              {!workspace && (
                <EmptyEditor
                  title="未选择项目"
                  detail="先从左侧打开或选择一个项目。"
                />
              )}
              {workspace && !activeFile && (
                <EmptyEditor
                  title="选择一个文件"
                  detail="从右侧文件树打开已有文件。"
                />
              )}
              {activeFile?.status === "loading" && (
                <EmptyEditor title="正在读取文件…" detail={activeFile.path} />
              )}
              {activeFile?.status === "error" && !activeFile.hash && (
                <EmptyEditor
                  title="无法打开文件"
                  detail={activeFile.error ?? "读取失败"}
                />
              )}
              {activeFile?.hash && (
                <>
                  {activeFile.status === "conflict" && (
                    <div className="editor-conflict" role="alert">
                      <span>磁盘内容已变化，本地草稿已保留。</span>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => reloadConflict(activeFile.path)}
                      >
                        重新加载
                      </Button>
                      <Button
                        size="sm"
                        onClick={() =>
                          void saveFile(activeFile.path, activeFile.diskHash)
                        }
                      >
                        覆盖磁盘
                      </Button>
                    </div>
                  )}
                  {(activeFile.status === "missing" ||
                    activeFile.status === "error") && (
                    <div className="editor-conflict" role="alert">
                      <span>{activeFile.error}</span>
                      {activeFile.status === "error" && (
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => void saveFile(activeFile.path)}
                        >
                          重试保存
                        </Button>
                      )}
                    </div>
                  )}
                  <Suspense
                    fallback={
                      <div className="editor-loading">正在加载编辑器…</div>
                    }
                  >
                    <CodeEditor
                      file={activeFile}
                      location={location}
                      onChange={(content) => editFile(activeFile.path, content)}
                      onSave={() => void saveFile(activeFile.path)}
                    />
                  </Suspense>
                </>
              )}
            </section>

            {treeOpen && workspace && (
              <aside className="file-tree-pane" style={{ width: treeWidth }}>
                <ResizeHandle
                  label="调整文件树宽度"
                  value={treeWidth}
                  min={180}
                  max={520}
                  growToward="left"
                  onChange={setTreeWidth}
                />
                <FileTree
                  key={workspace}
                  workspace={workspace}
                  client={client}
                  revision={treeRevision}
                  activePath={project?.activePath}
                  onOpenFile={(path) => void openFile(path)}
                  onOpenDirectory={watchDirectory}
                />
              </aside>
            )}
          </div>
        </div>
      ),
    },
  ];

  return (
    <div className="workspace-tabs">
      <AuxiliaryPanel
        tabs={tabs}
        activeTabID={project?.activePath ?? ""}
        views={views}
        onActivateTab={(tab) => {
          if (!project || tab.kind !== "file") return;
          project.activePath = tab.id;
          setLocation(undefined);
          render();
        }}
        onCloseTab={(tab) => requestClose(tab.id)}
        onHide={onHide}
      />

      <AlertDialog
        open={!!closingPath}
        onOpenChange={(open) => !open && setClosingPath("")}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>关闭未保存的文件？</AlertDialogTitle>
            <AlertDialogDescription>
              本地草稿将从当前页面移除，此操作无法撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>继续编辑</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => closeFile(closingPath)}
            >
              放弃并关闭
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function EmptyEditor({ title, detail }: { title: string; detail: string }) {
  return (
    <div className="workspace-chooser">
      <FileText />
      <strong>{title}</strong>
      <span>{detail}</span>
    </div>
  );
}

function editorBreadcrumb(workspace: string | null, path?: string): string {
  if (!workspace) return "未选择项目";
  const project = fileName(workspace);
  if (!path) return project;

  const prefix = workspace.replace(/[\\/]$/, "");
  const insideWorkspace =
    path.toLowerCase().startsWith(prefix.toLowerCase()) &&
    /[\\/]/.test(path.charAt(prefix.length));
  const relative = path.slice(prefix.length).replace(/^[\\/]/, "");
  if (!insideWorkspace || !relative) return path;
  return `${project}>${relative.replace(/[\\/]/g, ">")}`;
}
