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
  Settings,
  Circle,
  RefreshCw,
} from "./icons";
import type { ConnectionStatus } from "./client/rpc";
import { groupSessions } from "./state/projects";
import type { SessionView } from "../../contracts/harness.ts";

export function Sidebar({
  connection,
  backendBusy,
  sessions,
  listError,
  selectedID,
  settingsOpen,
  onClose,
  onOpenProject,
  onReload,
  onSelect,
  onCreate,
  onOpenSettings,
  onReconnect,
}: {
  connection: ConnectionStatus;
  backendBusy: boolean;
  sessions: SessionView[] | null;
  listError: string;
  selectedID: string | null;
  settingsOpen: boolean;
  onClose: () => void;
  onOpenProject: () => void;
  onReload: () => void;
  onSelect: (sessionID: string) => void;
  onCreate: (workspace: string) => void;
  onOpenSettings: () => void;
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

  return (
    <>
      <button
        className="sidebar-scrim"
        aria-label="关闭项目侧栏"
        onClick={onClose}
      />
      <aside className="sidebar">
        <div className="brand-row">
          <span className="brand">Harness</span>
          <Button
            variant="ghost"
            size="icon"
            aria-label="收起项目侧栏"
            onClick={onClose}
          >
            <PanelLeft />
          </Button>
        </div>
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="open-project-wrap">
              <Button
                variant="ghost"
                className="open-project"
                disabled={!connected || backendBusy}
                onClick={onOpenProject}
              >
                <FolderOpen />
                打开项目
                <Plus className="ml-auto" />
              </Button>
            </span>
          </TooltipTrigger>
          {!connected && <TooltipContent>尚未连接后台</TooltipContent>}
        </Tooltip>
        <div className="sidebar-heading">项目</div>
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
              <CollapsibleContent>
                {project.sessions.map((item) => (
                  <button
                    key={item.sessionID}
                    className={`session-row ${item.sessionID === selectedID && !settingsOpen ? "selected" : ""}`}
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
          <Button
            variant="ghost"
            className={settingsOpen ? "selected" : ""}
            onClick={onOpenSettings}
          >
            <Settings />
            设置
          </Button>
          <div className="local-caption">
            <Circle />
            本机工作空间<span>{connectionLabel}</span>
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
