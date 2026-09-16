import {
  lazy,
  memo,
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
import { Bot, FileText, GitCompareArrows, PanelRight } from "./icons";
import { Terminal } from "./icons";
import type { FileLocation } from "./editor/links";
import type { RunState } from "../../contracts/run.ts";
import type { RunDiffSummary } from "../../contracts/run.ts";
import type { AgentView, ModelChoice } from "../../contracts/appserver.ts";
import type { SubagentInfo } from "../../contracts/harness.ts";

const CodeEditor = lazy(() =>
  import("./editor/code-editor").then((module) => ({
    default: module.CodeEditor,
  })),
);

const ReviewView = lazy(() =>
  import("./review/review-view").then((module) => ({
    default: module.ReviewView,
  })),
);

const TerminalView = lazy(() =>
  import("./terminal/terminal-view").then((module) => ({
    default: module.TerminalView,
  })),
);

const SubagentView = lazy(() =>
  import("./subagent/subagent-view").then((module) => ({
    default: module.SubagentView,
  })),
);

const SubagentReviewView = lazy(() =>
  import("./subagent/subagent-review-view").then((module) => ({
    default: module.SubagentReviewView,
  })),
);

export interface ReviewOpenRequest {
  requestID: number;
  sessionID: string;
  runID: string;
}

export interface SubagentOpenRequest {
  requestID: number;
  parentSessionID: string;
  taskID: string;
}

interface WatchTarget {
  path: string;
  kind: "file" | "directory";
  subscriptionID: string;
}

interface TerminalTab {
  processID: string;
  title: string;
  workspace: string;
}

interface SubagentTab {
  id: string;
  parentSessionID: string;
  taskID: string;
  title: string;
  ancestors: string[];
  status?: string;
}

interface SubagentReviewTab {
  id: string;
  parentSessionID: string;
  taskID: string;
  title: string;
  summary: RunDiffSummary;
  workspace: string;
  runActive: boolean;
}

const emptyFileTabID = "file:empty";

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

function WorkspaceTabsComponent({
  workspace,
  sessionID,
  sessionTitle,
  runs,
  runActive,
  client,
  openRequest,
  reviewRequest,
  subagentRequest,
  models,
  agents,
  onHide,
}: {
  workspace: string | null;
  sessionID: string | null;
  sessionTitle: string | null;
  runs: RunState[];
  runActive: boolean;
  client: RPCClient | null;
  openRequest?: FileLocation & { requestID: number; workspace: string };
  reviewRequest?: ReviewOpenRequest;
  subagentRequest?: SubagentOpenRequest;
  models: ModelChoice[] | null;
  agents: AgentView[] | null;
  onHide: () => void;
}) {
  const projects = useRef(new Map<string, ProjectEditorState>());
  const watches = useRef(new Map<string, WatchTarget>());
  const watchPaths = useRef(new Map<string, string>());
  const saveTimers = useRef(new Map<string, ReturnType<typeof setTimeout>>());
  const saving = useRef(new Set<string>());
  const handledReviewRequestID = useRef(0);
  const handledSubagentRequestID = useRef(0);
  const terminalSequence = useRef(0);
  const treePane = useRef<HTMLElement>(null);
  const [, setRevision] = useState(0);
  const [treeRevision, setTreeRevision] = useState(0);
  const [treeOpen, setTreeOpen] = useState(true);
  const [treeWidth, setTreeWidth] = useState(storedTreeWidth);
  const [closingPath, setClosingPath] = useState("");
  const [location, setLocation] = useState<FileLocation>();
  const [reviewRunIDs, setReviewRunIDs] = useState<string[]>([]);
  const [terminals, setTerminals] = useState<TerminalTab[]>([]);
  const [subagentTabs, setSubagentTabs] = useState<SubagentTab[]>([]);
  const [subagentReviews, setSubagentReviews] = useState<SubagentReviewTab[]>(
    [],
  );
  const [activeTabID, setActiveTabID] = useState("");

  const project = workspace ? projects.current.get(workspace) : undefined;
  const activeFile = project?.files.get(project.activePath);
  const reviewTabID = (runID: string) => `review:${sessionID}:${runID}`;
  const activeReviewRunID = reviewRunIDs.find(
    (runID) => reviewTabID(runID) === activeTabID,
  );
  const activeReview = runs.find(
    (run) => run.runID === activeReviewRunID,
  )?.diff;
  const activeSubagentReview = subagentReviews.find(
    (item) => item.id === activeTabID,
  );

  function createTerminal() {
    if (!workspace || !client?.connected) return;
    const count = ++terminalSequence.current;
    const terminal = {
      processID: crypto.randomUUID(),
      title: count === 1 ? "bash" : `bash ${count}`,
      workspace,
    };
    setTerminals((current) => [...current, terminal]);
    setActiveTabID(terminal.processID);
  }

  function currentProject(): ProjectEditorState | null {
    if (!workspace) return null;
    return projectEditorState(projects.current, workspace);
  }

  function render() {
    setRevision((value) => value + 1);
  }

  function openSubagent(
    parentSessionID: string,
    taskID: string,
    ancestors: string[],
  ) {
    const id = `subagent:${parentSessionID}:${taskID}`;
    setSubagentTabs((current) =>
      current.some((item) => item.id === id)
        ? current
        : [
            ...current,
            {
              id,
              parentSessionID,
              taskID,
              title: "Subagent",
              ancestors,
            },
          ],
    );
    setActiveTabID(id);
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
      if (!workspace || !client?.connected || watchPaths.current.has(path))
        return;
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
    if (!openRequest || !workspace || openRequest.workspace !== workspace)
      return;
    setLocation(openRequest);
    void openFile(openRequest.path, openRequest);
  }, [openRequest?.requestID, workspace, client]);

  useEffect(() => {
    setReviewRunIDs([]);
    setSubagentTabs([]);
    setSubagentReviews([]);
    setActiveTabID(currentProject()?.activePath ?? "");
  }, [sessionID, workspace]);

  useEffect(() => {
    if (!client) setTerminals([]);
  }, [client]);

  useEffect(() => {
    if (!reviewRequest || reviewRequest.sessionID !== sessionID) return;
    if (reviewRequest.requestID === handledReviewRequestID.current) return;
    const run = runs.find((item) => item.runID === reviewRequest.runID);
    if (!run?.diff) return;
    handledReviewRequestID.current = reviewRequest.requestID;
    setReviewRunIDs((current) =>
      current.includes(run.runID) ? current : [...current, run.runID],
    );
    setActiveTabID(reviewTabID(run.runID));
  }, [reviewRequest?.requestID, sessionID, runs]);

  useEffect(() => {
    if (!subagentRequest || subagentRequest.parentSessionID !== sessionID)
      return;
    if (subagentRequest.requestID === handledSubagentRequestID.current) return;
    handledSubagentRequestID.current = subagentRequest.requestID;
    openSubagent(
      subagentRequest.parentSessionID,
      subagentRequest.taskID,
      [],
    );
  }, [subagentRequest?.requestID, sessionID]);

  async function openFile(path: string, target?: FileLocation) {
    const state = currentProject();
    if (!state || !client?.connected) return;
    if (state.files.has(path)) {
      state.activePath = path;
      setActiveTabID(path);
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
    setActiveTabID(path);
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
    if (!file.hash || file.status === "missing" || file.status === "loading")
      return;
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
    if (activeTabID === path)
      setActiveTabID(
        state.activePath ||
          (reviewRunIDs[0] ? reviewTabID(reviewRunIDs[0]) : ""),
      );
    const timer = saveTimers.current.get(path);
    if (timer) clearTimeout(timer);
    saveTimers.current.delete(path);
    const subscriptionID = watchPaths.current.get(path);
    watchPaths.current.delete(path);
    if (subscriptionID && subscriptionID !== "pending") {
      watches.current.delete(subscriptionID);
      if (client?.connected)
        void client.unsubscribe(subscriptionID).catch(() => {});
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

  const tabs: AuxiliaryTab[] = [
    ...(project?.order.map((path) => {
      const file = project.files.get(path)!;
      return {
        id: path,
        kind: "file",
        title: file.name,
        contextPath: path,
        dirty: file.content !== file.savedContent || file.status === "conflict",
      };
    }) ?? []),
    ...reviewRunIDs.map((runID) => ({
      id: reviewTabID(runID),
      kind: "review",
      title: "审查更改",
      contextPath: runID,
    })),
    ...terminals.map((terminal) => ({
      id: terminal.processID,
      kind: "terminal",
      title: terminal.title,
      contextPath: terminal.workspace,
    })),
    ...subagentTabs.map((tab) => ({
      id: tab.id,
      kind: "subagent",
      title: [sessionTitle ?? "主会话", ...tab.ancestors, tab.title].join(
        " › ",
      ),
      contextPath: tab.taskID,
      status: tab.status,
    })),
    ...subagentReviews.map((tab) => ({
      id: tab.id,
      kind: "subagent-review",
      title: "审查更改",
      contextPath: tab.summary.runID,
    })),
  ];
  const visibleTabs: AuxiliaryTab[] =
    workspace && (project?.order.length ?? 0) === 0
      ? [
          {
            id: emptyFileTabID,
            kind: "file",
            title: "查看文件",
            closable: false,
          },
          ...tabs,
        ]
      : tabs;

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
              size="icon-xs"
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
              <aside
                ref={treePane}
                className="file-tree-pane"
                style={{ width: treeWidth }}
              >
                <ResizeHandle
                  label="调整文件树宽度"
                  value={treeWidth}
                  min={180}
                  max={520}
                  growToward="left"
                  onResize={(width) => {
                    if (treePane.current)
                      treePane.current.style.width = `${width}px`;
                  }}
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
    {
      kind: "review",
      label: "审查",
      icon: GitCompareArrows,
      onCreate: () => {
        const latest = [...runs].reverse().find((run) => run.diff)?.runID;
        if (!latest) return;
        setReviewRunIDs((current) =>
          current.includes(latest) ? current : [...current, latest],
        );
        setActiveTabID(reviewTabID(latest));
      },
      render: () =>
        sessionID && activeReview ? (
          <Suspense
            fallback={<div className="editor-loading">正在加载审查…</div>}
          >
            <ReviewView
              key={`${sessionID}:${activeReview.runID}`}
              sessionID={sessionID}
              workspace={workspace}
              initialSummary={activeReview}
              client={client}
              runActive={runActive}
            />
          </Suspense>
        ) : (
          <EmptyEditor
            title="没有可审查的更改"
            detail="Agent 的文件修改会显示在这里。"
          />
        ),
    },
    {
      kind: "terminal",
      label: "终端",
      icon: Terminal,
      keepMounted: true,
      onCreate: createTerminal,
      render: (tab) => {
        const terminal = terminals.find((item) => item.processID === tab?.id);
        return terminal && client ? (
          <Suspense
            fallback={<div className="editor-loading">正在加载终端…</div>}
          >
            <TerminalView
              processID={terminal.processID}
              workspace={terminal.workspace}
              active={activeTabID === terminal.processID}
              client={client}
            />
          </Suspense>
        ) : null;
      },
    },
    {
      kind: "subagent",
      label: "Subagent",
      icon: Bot,
      keepMounted: true,
      render: (tab) => {
        const child = subagentTabs.find((item) => item.id === tab?.id);
        return child ? (
          <Suspense
            fallback={<div className="editor-loading">正在加载子任务…</div>}
          >
            <SubagentView
              parentSessionID={child.parentSessionID}
              taskID={child.taskID}
              client={client}
              models={models}
              agents={agents}
              active={activeTabID === child.id}
              onTask={(task: SubagentInfo) =>
                setSubagentTabs((current) =>
                  current.map((item) =>
                    item.id === child.id
                      ? { ...item, title: task.taskName, status: task.status }
                      : item,
                  ),
                )
              }
              onOpenSubagent={(parentSessionID, taskID) =>
                openSubagent(parentSessionID, taskID, [
                  ...child.ancestors,
                  child.title,
                ])
              }
              onOpenFile={(target) => void openFile(target.path, target)}
              onOpenDiff={(_runID, summary, childRunActive) => {
                const id = `subagent-review:${child.taskID}:${summary.runID}`;
                setSubagentReviews((current) => {
                  const existing = current.find((item) => item.id === id);
                  if (existing)
                    return current.map((item) =>
                      item.id === id
                        ? { ...item, summary, runActive: childRunActive }
                        : item,
                    );
                  return [
                    ...current,
                    {
                      id,
                      parentSessionID: child.parentSessionID,
                      taskID: child.taskID,
                      title: child.title,
                      summary,
                      workspace: workspace ?? "",
                      runActive: childRunActive,
                    },
                  ];
                });
                setActiveTabID(id);
              }}
              onDiffUpdate={(runID, summary, childRunActive) => {
                const id = `subagent-review:${child.taskID}:${runID}`;
                setSubagentReviews((current) => {
                  const existing = current.find((item) => item.id === id);
                  if (
                    !existing ||
                    (existing.summary.revision === summary.revision &&
                      existing.runActive === childRunActive)
                  )
                    return current;
                  return current.map((item) =>
                    item.id === id
                      ? { ...item, summary, runActive: childRunActive }
                      : item,
                  );
                });
              }}
            />
          </Suspense>
        ) : null;
      },
    },
    {
      kind: "subagent-review",
      label: "Subagent 审查",
      icon: GitCompareArrows,
      render: () =>
        activeSubagentReview ? (
          <Suspense
            fallback={<div className="editor-loading">正在加载审查…</div>}
          >
            <SubagentReviewView
              key={`${activeSubagentReview.parentSessionID}:${activeSubagentReview.taskID}:${activeSubagentReview.summary.runID}`}
              parentSessionID={activeSubagentReview.parentSessionID}
              taskID={activeSubagentReview.taskID}
              workspace={activeSubagentReview.workspace}
              initialSummary={activeSubagentReview.summary}
              initialRunActive={activeSubagentReview.runActive}
              client={client}
            />
          </Suspense>
        ) : null,
    },
  ];

  return (
    <div className="workspace-tabs">
      <AuxiliaryPanel
        tabs={visibleTabs}
        activeTabID={activeTabID || project?.activePath || emptyFileTabID}
        views={views}
        onActivateTab={(tab) => {
          setActiveTabID(tab.id);
          if (project && tab.kind === "file") {
            project.activePath = tab.id === emptyFileTabID ? "" : tab.id;
            setLocation(undefined);
            render();
          }
        }}
        onCloseTab={(tab) => {
          if (tab.kind === "file") {
            requestClose(tab.id);
            return;
          }
          if (tab.kind === "terminal") {
            const index = tabs.findIndex((item) => item.id === tab.id);
            if (client?.connected)
              void client.terminateTerminal(tab.id).catch(() => {});
            setTerminals((current) =>
              current.filter((item) => item.processID !== tab.id),
            );
            if (activeTabID === tab.id)
              setActiveTabID(tabs[index - 1]?.id ?? tabs[index + 1]?.id ?? "");
            return;
          }
          const index = tabs.findIndex((item) => item.id === tab.id);
          if (tab.kind === "subagent") {
            setSubagentTabs((current) =>
              current.filter((item) => item.id !== tab.id),
            );
          } else if (tab.kind === "subagent-review") {
            setSubagentReviews((current) =>
              current.filter((item) => item.id !== tab.id),
            );
          } else {
            setReviewRunIDs((current) =>
              current.filter((runID) => reviewTabID(runID) !== tab.id),
            );
          }
          if (activeTabID === tab.id)
            setActiveTabID(tabs[index - 1]?.id ?? tabs[index + 1]?.id ?? "");
        }}
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

function sameRunView(previous: RunState[], next: RunState[]): boolean {
  if (previous.length !== next.length) return false;
  return previous.every((run, index) => {
    const candidate = next[index];
    return (
      run.runID === candidate.runID &&
      run.status === candidate.status &&
      run.diff?.revision === candidate.diff?.revision
    );
  });
}

// 聊天文字增量不改变辅助区；避免流式输出带着 Monaco、终端和隐藏标签重绘。
export const WorkspaceTabs = memo(
  WorkspaceTabsComponent,
  (previous, next) =>
    previous.workspace === next.workspace &&
    previous.sessionID === next.sessionID &&
    previous.sessionTitle === next.sessionTitle &&
    sameRunView(previous.runs, next.runs) &&
    previous.runActive === next.runActive &&
    previous.client === next.client &&
    previous.openRequest === next.openRequest &&
    previous.reviewRequest === next.reviewRequest &&
    previous.subagentRequest === next.subagentRequest &&
    previous.models === next.models &&
    previous.agents === next.agents,
);

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
