import { useState } from "react";
import { Button } from "@/components/ui/button";
import type { SessionView } from "../../../contracts/harness.ts";
import { groupSessions } from "../state/projects";
import { ArchiveRestore, Search, Trash2 } from "../icons";

export function ArchivedSettingsPanel({ sessions, error, onView, onRestore, onDeleteSession, onDeleteProject }: {
  sessions: SessionView[] | null;
  error: string;
  onView: (sessionID: string) => void;
  onRestore: (sessionID: string) => void;
  onDeleteSession: (sessionID: string) => void;
  onDeleteProject: (workspace: string) => void;
}) {
  const [query, setQuery] = useState("");
  const normalized = query.trim().toLocaleLowerCase();
  const groups = groupSessions((sessions ?? []).filter((item) =>
    !normalized || item.title.toLocaleLowerCase().includes(normalized) ||
    item.settings.workspace.toLocaleLowerCase().includes(normalized),
  ));
  return <div className="archived-settings-page">
    <header className="settings-heading"><h2>已归档会话</h2></header>
    <label className="archived-search"><Search aria-hidden="true" />
      <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索会话或项目" aria-label="搜索已归档会话" />
    </label>
    {error && <p className="inline-notice" role="alert">{error}</p>}
    {!sessions && !error && <p className="metadata">正在加载…</p>}
    {sessions && !groups.length && <p className="metadata">{query ? "没有匹配的会话" : "还没有已归档会话"}</p>}
    {groups.map((group) => <section className="archived-group" key={group.workspace}>
      <header><div><h3>{group.name}</h3><p className="metadata" title={group.workspace}>{group.workspace}</p></div>
        <Button variant="ghost" size="sm" onClick={() => onDeleteProject(group.workspace)}><Trash2 />永久删除项目</Button>
      </header>
      <div className="archived-list">
        {group.sessions.map((item) => <div className="archived-row" key={item.sessionID}>
          <div><strong>{item.title}</strong><span className="metadata">{new Date(item.archivedAt ?? item.createdAt).toLocaleString()}</span></div>
          <Button variant="ghost" size="sm" onClick={() => onView(item.sessionID)}>查看</Button>
          <Button variant="ghost" size="sm" onClick={() => onRestore(item.sessionID)}><ArchiveRestore />恢复</Button>
          <Button variant="ghost" size="icon-sm" aria-label={`永久删除会话 ${item.title}`} onClick={() => onDeleteSession(item.sessionID)}><Trash2 /></Button>
        </div>)}
      </div>
    </section>)}
  </div>;
}
