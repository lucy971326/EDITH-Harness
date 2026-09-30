import { useRef, useState } from "react";
import { ConnectionIndicator } from "../components/connection-indicator";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  Hint,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  PanelLeft,
  Folder,
  FolderOpen,
  Plus,
  ChevronRight,
  Archive,
  Trash2,
  Pencil,
  GitBranch,
  MoreHorizontal,
} from "../icons";
import { formatRPCError, type ConnectionStatus } from "../client/rpc";
import { groupSessions } from "../state/projects";
import type { PendingApproval } from "../../../contracts/approvals";
import type { SessionActivity, SessionView } from "../../../contracts/harness.ts";
import type { NavigationEntry } from "./navigation";

export function Sidebar({
  connection,
  connectionDetail,
  backendBusy,
  sessions,
  activities,
  pendingApprovals,
  activityError,
  listError,
  selectedID,
  navigation,
  activePath,
  onNavigate,
  onClose,
  onOpenProject,
  onReload,
  onSelect,
  onCreate,
  onReconnect,
  onArchiveSession,
  onRenameSession,
  onForkLatest,
  onDeleteSession,
  onDeleteProject,
}: {
  connection: ConnectionStatus;
  connectionDetail?: string;
  backendBusy: boolean;
  sessions: SessionView[] | null;
  activities: SessionActivity[];
  pendingApprovals: PendingApproval[];
  activityError: string;
  listError: string;
  selectedID: string | null;
  navigation: NavigationEntry[];
  activePath: string;
  onNavigate: (path: string) => void;
  onClose: () => void;
  onOpenProject: () => void;
  onReload: () => void;
  onSelect: (sessionID: string) => void;
  onCreate: (workspace: string) => void;
  onReconnect: () => void;
  onArchiveSession: (sessionID: string) => void;
  onRenameSession: (sessionID: string, title: string) => Promise<void>;
  onForkLatest: (sessionID: string) => Promise<void>;
  onDeleteSession: (sessionID: string) => void;
  onDeleteProject: (workspace: string) => void;
}) {
  const [editingID, setEditingID] = useState<string | null>(null);
  const [titleDraft, setTitleDraft] = useState("");
  const [rowError, setRowError] = useState<{ id: string; text: string } | null>(null);
  const renamePending = useRef(false);
  const renameFocus = useRef<string | null>(null);
  const activityByID = new Map(activities.map((item) => [item.sessionID, item]));
  const waiting = new Set(pendingApprovals.map((item) => item.sessionID));
  function sessionStatus(id: string) {
    const state = activityByID.get(id);
    if (waiting.has(id)) return { kind: "waiting", label: "等待审批" };
    if (state?.running) return { kind: "running", label: "正在运行" };
    if (state && state.latestResultSeq > state.readResultSeq) return { kind: "unread", label: "有未读结果" };
    return null;
  }
  const connected = connection === "connected";
  const projects = sessions ? groupSessions(sessions) : [];

  function beginRename(item: SessionView) {
    renameFocus.current = item.sessionID;
    setEditingID(item.sessionID);
    setTitleDraft(item.title);
    setRowError(null);
    requestAnimationFrame(() => document.getElementById(`rename-${item.sessionID}`)?.focus());
  }

  async function saveRename(sessionID: string) {
    if (renamePending.current) return;
    renamePending.current = true;
    try {
      await onRenameSession(sessionID, titleDraft);
      setEditingID(null);
      setRowError(null);
    } catch (error) {
      setRowError({ id: sessionID, text: formatRPCError(error, "重命名失败") });
      document.getElementById(`rename-${sessionID}`)?.focus();
    } finally {
      renamePending.current = false;
    }
  }

  async function forkLatest(sessionID: string) {
    setRowError(null);
    try {
      await onForkLatest(sessionID);
    } catch (error) {
      setRowError({ id: sessionID, text: formatRPCError(error, "分叉失败") });
    }
  }

  function navigationItem(entry: NavigationEntry) {
    const Icon = entry.icon;
    const active = entry.kind === "page" && activePath === entry.path;
    return <Button key={entry.id} variant="ghost" className={`navigation-item ${entry.kind === "action" ? "ui-key" : ""} ${active ? "selected" : ""}`}
      data-kind={entry.kind}
      aria-current={active ? "page" : undefined}
      disabled={entry.kind === "action" && entry.disabled}
      onClick={() => {
        if (entry.kind === "action") entry.run();
        else onNavigate(entry.path);
        if (window.innerWidth < 760) onClose();
      }}>
      <Icon /><span>{entry.label}</span>
    </Button>;
  }

  return (
    <>
      <button
        className="sidebar-scrim"
        aria-label="关闭项目侧栏"
        onClick={onClose}
      />
      <aside className="sidebar">
        <div className="brand-row">
          <span className="brand"><img className="brand-mark" src="/edith-icon.svg" alt="" />EDITH</span>
          <span className="sidebar-workspace-label"><Folder />工作区</span>
          <ConnectionIndicator status={connection} detail={connectionDetail} onReconnect={onReconnect} />
          <Hint text="收起项目侧栏"><Button
            variant="ghost"
            size="icon"
            aria-label="收起项目侧栏"
            onClick={onClose}
          >
            <PanelLeft />
          </Button></Hint>
        </div>
        <nav className="feature-navigation" aria-label="功能导航">
          {navigation.filter((entry) => entry.placement === "primary").map(navigationItem)}
        </nav>
        <div className="sidebar-heading"><span>项目</span>
        <Tooltip>
          <TooltipTrigger asChild>
            <span>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="打开项目"
                disabled={!connected || backendBusy}
                onClick={onOpenProject}
              >
                <FolderOpen />
              </Button>
            </span>
          </TooltipTrigger>
          <TooltipContent>{connected ? "打开项目" : "尚未连接后台"}</TooltipContent>
        </Tooltip>
        </div>
        <nav className="project-list" aria-label="项目与会话">
          {activityError && <p className="metadata sidebar-empty" role="status">{activityError}</p>}
          {connection !== "connected" && sessions === null && !listError && (
            <p className="metadata sidebar-empty">
              {connection === "connecting"
                ? "正在连接后台…"
                : "尚未连接后台。"}
            </p>
          )}
          {connected && sessions === null && !listError && (
            <p className="metadata sidebar-empty">正在加载项目…</p>
          )}
          {listError && (
            <div className="sidebar-empty">
              <p className="metadata">{listError}</p>
              <Button
                variant="ghost"
                size="sm"
                disabled={!connected}
                onClick={onReload}
              >
                重新加载列表
              </Button>
            </div>
          )}
          {sessions && sessions.length === 0 && !listError && (
            <p className="metadata sidebar-empty">
              还没有项目。打开一个目录开始。
            </p>
          )}
          {projects.map((project) => (
            <Collapsible
              defaultOpen
              key={project.workspace}
              className="project-group"
            >
              <div className="project-heading">
                <Hint text={project.workspace}><CollapsibleTrigger className="project-trigger ui-focus">
                  <ChevronRight className="disclosure-chevron" />
                  <Folder />
                  <span>{project.name}</span>
                </CollapsibleTrigger></Hint>
                <Hint text="永久删除项目"><Button variant="ghost" size="icon-sm" className="sidebar-row-action"
                  aria-label={`永久删除项目 ${project.name}`} disabled={!connected}
                  onClick={() => onDeleteProject(project.workspace)}><Trash2 /></Button></Hint>
                <Hint text="新建会话"><Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={`在 ${project.name} 新建会话`}
                  disabled={!connected || backendBusy}
                  onClick={() => onCreate(project.workspace)}
                >
                  <Plus />
                </Button></Hint>
              </div>
              <CollapsibleContent className="project-sessions">
                {project.sessions.map((item) => {
                  const state = activityByID.get(item.sessionID);
                  const status = connected ? sessionStatus(item.sessionID) : null;
                  return (
                  <div key={item.sessionID} className="session-item">
                    <div
                      data-unread={!!state && state.latestResultSeq > state.readResultSeq}
                      className={`session-row ${item.sessionID === selectedID && activePath === "/" ? "selected" : ""}`}
                    >
                      {editingID === item.sessionID ? <input
                        id={`rename-${item.sessionID}`}
                        className="session-rename-input ui-focus"
                        aria-label="会话名称"
                        autoFocus
                        value={titleDraft}
                        disabled={!connected}
                        onChange={(event) => { setTitleDraft(event.target.value); setRowError(null); }}
                        onBlur={() => { if (!renamePending.current) { setEditingID(null); setRowError(null); } }}
                        onKeyDown={(event) => {
                          if (event.key === "Escape" && !renamePending.current) {
                            event.stopPropagation();
                            setEditingID(null);
                            setRowError(null);
                          }
                          if (event.key === "Enter" && !event.nativeEvent.isComposing && event.keyCode !== 229) {
                            event.preventDefault();
                            event.stopPropagation();
                            void saveRename(item.sessionID);
                          }
                        }}
                      /> : <Hint text={item.title}><button className="session-link ui-focus"
                        aria-current={item.sessionID === selectedID && activePath === "/" ? "page" : undefined}
                        onClick={() => onSelect(item.sessionID)}><span>{item.title}</span></button></Hint>}
                      {status && <Tooltip><TooltipTrigger asChild>
                        <span className="session-indicator-target ui-focus" tabIndex={0} role="img" aria-label={status.label}>
                          <span className={`session-indicator session-indicator-${status.kind}`} aria-hidden="true" />
                        </span>
                      </TooltipTrigger><TooltipContent>{status.label}</TooltipContent></Tooltip>}
                      {editingID !== item.sessionID && <div className="session-actions">
                        <Hint text="归档会话"><Button variant="ghost" size="icon-sm" className="sidebar-row-action"
                          aria-label={`归档会话 ${item.title}`}
                          disabled={!connected} onClick={() => onArchiveSession(item.sessionID)}><Archive /></Button></Hint>
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild><Button variant="ghost" size="icon-sm" className="sidebar-row-action"
                            aria-label={`更多会话操作 ${item.title}`} disabled={!connected}><MoreHorizontal /></Button></DropdownMenuTrigger>
                          <DropdownMenuContent align="end" onCloseAutoFocus={(event) => {
                            if (renameFocus.current === item.sessionID) {
                              event.preventDefault();
                              renameFocus.current = null;
                            }
                          }}>
                            <DropdownMenuItem onSelect={() => beginRename(item)}><Pencil />重命名</DropdownMenuItem>
                            <DropdownMenuItem onSelect={() => void forkLatest(item.sessionID)}>
                              <GitBranch />分叉会话
                            </DropdownMenuItem>
                            <DropdownMenuItem variant="destructive" onSelect={() => onDeleteSession(item.sessionID)}><Trash2 />永久删除</DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </div>}
                    </div>
                    {rowError?.id === item.sessionID && <p className="session-row-error" role="alert">{rowError.text}</p>}
                  </div>
                ); })}
              </CollapsibleContent>
            </Collapsible>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <nav aria-label="应用导航">{navigation.filter((entry) => entry.placement === "footer").map(navigationItem)}</nav>

        </div>
      </aside>
    </>
  );
}
