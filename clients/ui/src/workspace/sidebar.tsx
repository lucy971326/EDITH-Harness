import { ConnectionIndicator } from "../components/connection-indicator";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
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
} from "../icons";
import type { ConnectionStatus } from "../client/rpc";
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
  onDeleteSession: (sessionID: string) => void;
  onDeleteProject: (workspace: string) => void;
}) {
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
      <aside className="sidebar workspace-panel">
        <div className="brand-row">
          <span className="brand"><img className="brand-mark" src="/edith-icon.svg" alt="" />EDITH</span>
          <ConnectionIndicator status={connection} detail={connectionDetail} onReconnect={onReconnect} />
          <Button
            variant="ghost"
            size="icon"
            aria-label="收起项目侧栏" title="收起项目侧栏"
            onClick={onClose}
          >
            <PanelLeft />
          </Button>
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
                <CollapsibleTrigger className="project-trigger ui-focus">
                  <ChevronRight className="disclosure-chevron" />
                  <Folder />
                  <span title={project.workspace}>{project.name}</span>
                </CollapsibleTrigger>
                <Button variant="ghost" size="icon-sm" className="sidebar-row-action"
                  title="永久删除项目" aria-label={`永久删除项目 ${project.name}`} disabled={!connected}
                  onClick={() => onDeleteProject(project.workspace)}><Trash2 /></Button>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  title="新建会话" aria-label={`在 ${project.name} 新建会话`}
                  disabled={!connected || backendBusy}
                  onClick={() => onCreate(project.workspace)}
                >
                  <Plus />
                </Button>
              </div>
              <CollapsibleContent className="project-sessions">
                {project.sessions.map((item) => {
                  const state = activityByID.get(item.sessionID);
                  const status = connected ? sessionStatus(item.sessionID) : null;
                  return (
                  <div
                    key={item.sessionID}
                    data-unread={!!state && state.latestResultSeq > state.readResultSeq}
                    className={`session-row ${item.sessionID === selectedID && activePath === "/" ? "selected" : ""}`}
                  >
                    <button className="session-link ui-focus" title={item.title}
                      aria-current={item.sessionID === selectedID && activePath === "/" ? "page" : undefined}
                      onClick={() => onSelect(item.sessionID)}><span>{item.title}</span></button>
                    {status && <Tooltip><TooltipTrigger asChild>
                      <span className="session-indicator-target ui-focus" tabIndex={0} role="img" aria-label={status.label}>
                        <span className={`session-indicator session-indicator-${status.kind}`} aria-hidden="true" />
                      </span>
                    </TooltipTrigger><TooltipContent>{status.label}</TooltipContent></Tooltip>}
                    <div className="session-actions">
                    <Button variant="ghost" size="icon-sm" className="sidebar-row-action"
                        title="归档会话" aria-label={`归档会话 ${item.title}`}
                        disabled={!connected} onClick={() => onArchiveSession(item.sessionID)}><Archive /></Button>
                      <Button variant="ghost" size="icon-sm" className="sidebar-row-action"
                        title="永久删除会话" aria-label={`永久删除会话 ${item.title}`}
                        disabled={!connected} onClick={() => onDeleteSession(item.sessionID)}><Trash2 /></Button>
                    </div>
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
