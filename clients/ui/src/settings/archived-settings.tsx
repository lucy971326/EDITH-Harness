import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Hint } from "@/components/ui/tooltip";
import { Input } from "@/components/ui/input";
import type { SessionView } from "../../../contracts/harness.ts";
import { groupSessions } from "../state/projects";
import { Archive, ArchiveRestore, Folder, Folders, Search, Trash2 } from "../icons";

export function ArchivedSettingsPanel({ sessions, error, onView, onRestore, onDeleteSession, onDeleteProject }: {
  sessions: SessionView[] | null;
  error: string;
  onView: (sessionID: string) => void;
  onRestore: (sessionID: string) => void;
  onDeleteSession: (sessionID: string) => void;
  onDeleteProject: (workspace: string) => void;
}) {
  const [query, setQuery] = useState("");
  const [workspace, setWorkspace] = useState<string | null>(null);
  const projects = groupSessions(sessions ?? []);
  const selected = projects.find((group) => group.workspace === workspace);
  const normalized = query.trim().toLocaleLowerCase();
  const groups = groupSessions((sessions ?? []).filter((item) =>
    (!selected || item.settings.workspace === selected.workspace) &&
    (!normalized || item.title.toLocaleLowerCase().includes(normalized) ||
    item.settings.workspace.toLocaleLowerCase().includes(normalized)),
  ));
  return <div className="archived-settings-page">
    <header className="settings-heading"><h2>已归档会话</h2></header>
    <div className="settings-two-pane">
    <aside className="settings-subnav" aria-label="归档项目">
      <h3>项目</h3><div className="settings-subnav-list">
        <Button variant="ghost" className="settings-subnav-item" aria-pressed={!selected} onClick={() => setWorkspace(null)}>
          <Folders /><span className="settings-subnav-copy"><span>全部项目</span><span className="settings-subnav-meta">{sessions?.length ?? 0} 个会话</span></span>
        </Button>
        {projects.map((group) => <Hint key={group.workspace} text={group.workspace}><Button variant="ghost" className="settings-subnav-item"
          aria-pressed={selected?.workspace === group.workspace} onClick={() => setWorkspace(group.workspace)}>
          <Folder /><span className="settings-subnav-copy"><span className="settings-subnav-name">{group.name}</span>
            <span className="settings-subnav-meta">{group.sessions.length} 个会话</span></span>
        </Button></Hint>)}
      </div>
    </aside>
    <div className="settings-detail-pane">
    <div className="settings-detail-title-row"><div className="settings-identity"><span className="settings-identity-icon">{selected ? <Folder /> : <Archive />}</span>
      <h3>{selected?.name ?? "全部归档"}</h3></div></div>
    <div className="archived-search"><Search aria-hidden="true" />
      <Input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索会话或项目" aria-label="搜索已归档会话" />
    </div>
    {error && <p className="inline-notice" role="alert">{error}</p>}
    {!sessions && !error && <p className="metadata">正在加载…</p>}
    {sessions && !groups.length && <p className="metadata">{query ? "没有匹配的会话" : "还没有已归档会话"}</p>}
    {groups.map((group) => <section className="archived-group" key={group.workspace}>
      <header><div><h3>{group.name}</h3><p className="metadata">{group.workspace}</p></div>
        <Hint text="永久删除项目"><Button variant="ghost" size="icon-sm" className="settings-delete" aria-label={`永久删除项目 ${group.workspace}`} onClick={() => onDeleteProject(group.workspace)}><Trash2 /></Button></Hint>
      </header>
      <div className="archived-list">
        {group.sessions.map((item) => <div className="archived-row" key={item.sessionID}>
          <Hint text={item.title}><button className="archived-session-link ui-focus" onClick={() => onView(item.sessionID)}>
            <strong>{item.title}</strong><span className="metadata">{new Date(item.archivedAt ?? item.createdAt).toLocaleString()}</span>
          </button></Hint>
          <Button variant="ghost" size="sm" onClick={() => onRestore(item.sessionID)}><ArchiveRestore />恢复</Button>
          <Hint text="永久删除会话"><Button variant="ghost" size="icon-sm" className="settings-delete" aria-label={`永久删除会话 ${item.title}`} onClick={() => onDeleteSession(item.sessionID)}><Trash2 /></Button></Hint>
        </div>)}
      </div>
    </section>)}
    </div></div>
  </div>;
}
