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
  Circle,
  RefreshCw,
} from "../icons";
import type { ConnectionStatus } from "../client/rpc";
import { groupSessions } from "../state/projects";
import type { SessionView } from "../../../contracts/harness.ts";
import type { NavigationEntry } from "./navigation";

export function Sidebar({
  connection,
  backendBusy,
  sessions,
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
}: {
  connection: ConnectionStatus;
  backendBusy: boolean;
  sessions: SessionView[] | null;
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
}) {
  const connected = connection === "connected";
  const projects = sessions ? groupSessions(sessions) : [];
  const connectionLabel =
    connection === "connecting"
      ? "正在连接"
      : connection === "connected"
        ? "已连接"
        : "已断开";

  function navigationItem(entry: NavigationEntry) {
    const Icon = entry.icon;
    const active = entry.kind === "page" && activePath === entry.path;
    return <Button key={entry.id} variant="ghost" className={`navigation-item ${active ? "selected" : ""}`}
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
          <span className="brand"><span className="brand-mark" aria-hidden="true">H</span>Harness</span>
          <Button
            variant="ghost"
            size="icon"
            aria-label="收起项目侧栏"
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
                <CollapsibleTrigger className="project-trigger">
                  <ChevronRight className="disclosure-chevron" />
                  <Folder />
                  <span title={project.workspace}>{project.name}</span>
                </CollapsibleTrigger>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={`在 ${project.name} 新建会话`}
                  disabled={!connected || backendBusy}
                  onClick={() => onCreate(project.workspace)}
                >
                  <Plus />
                </Button>
              </div>
              <CollapsibleContent className="project-sessions">
                {project.sessions.map((item) => (
                  <button
                    key={item.sessionID}
                    className={`session-row ${item.sessionID === selectedID && activePath === "/" ? "selected" : ""}`}
                    aria-current={item.sessionID === selectedID && activePath === "/" ? "page" : undefined}
                    onClick={() => onSelect(item.sessionID)}
                  >
                    <span>{item.title}</span>
                  </button>
                ))}
              </CollapsibleContent>
            </Collapsible>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <nav aria-label="应用导航">{navigation.filter((entry) => entry.placement === "footer").map(navigationItem)}</nav>
          <div className="local-caption" data-connected={connected}>
            <Circle />
            本地工作台<span>{connectionLabel}</span>
          </div>
          {connection === "disconnected" && (
            <Button variant="ghost" size="sm" onClick={onReconnect}>
              <RefreshCw />
              重新连接
            </Button>
          )}
        </div>
      </aside>
    </>
  );
}
