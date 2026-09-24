import { lazy, memo, Suspense, useEffect, useRef, useState } from "react";
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
import type { ContextReference } from "./context-references";
import { useEditorFiles } from "./editor/use-files";
import { FileTree } from "./editor/file-tree";
import { fileName, needsCloseConfirmation, statusLabel } from "./editor/files";
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
  onAddReference,
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
  onAddReference: (reference: ContextReference) => void;
}) {
  const files = useEditorFiles(client, workspace);
  const {
    project,
    treeRevision,
    editFile,
    saveFile,
    reloadConflict,
    watchDirectory,
  } = files;
  const handledReviewRequestID = useRef(0);
  const handledSubagentRequestID = useRef(0);
  const terminalSequence = useRef(0);
  const treePane = useRef<HTMLElement>(null);
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

  useEffect(() => {
    try {
      localStorage.setItem("harness-web:file-tree-width", String(treeWidth));
    } catch {
      /* 本机偏好不可写时仅保留本页状态。 */
    }
  }, [treeWidth]);

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
    setActiveTabID(project?.activePath ?? "");
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
    openSubagent(subagentRequest.parentSessionID, subagentRequest.taskID, []);
  }, [subagentRequest?.requestID, sessionID]);

  function openFile(path: string, target?: FileLocation) {
    if (!project || !client?.connected) return;
    setActiveTabID(path);
    setLocation(target);
    void files.openFile(path);
  }

  function requestClose(path: string) {
    const file = project?.files.get(path);
    if (!file) return;
    if (needsCloseConfirmation(file)) {
      setClosingPath(path);
      return;
    }
    closeFile(path);
  }

  function closeFile(path: string) {
    files.closeFile(path);
    if (activeTabID === path)
      setActiveTabID(
        project?.activePath ||
          (reviewRunIDs[0] ? reviewTabID(reviewRunIDs[0]) : ""),
      );
    setClosingPath("");
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
                      onAddReference={onAddReference}
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
            files.selectFile(tab.id === emptyFileTabID ? "" : tab.id);
            setLocation(undefined);
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
    previous.agents === next.agents &&
    previous.onAddReference === next.onAddReference,
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
