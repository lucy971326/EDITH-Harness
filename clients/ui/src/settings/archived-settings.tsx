import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Hint } from "@/components/ui/tooltip";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { SessionView } from "../../../contracts/harness.ts";
import { groupSessions } from "../state/projects";
import { ArchiveRestore, Trash2 } from "../icons";
import { SettingsHeader, SettingsListToolbar, SettingsEmpty } from "./settings-primitives";

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
    <SettingsHeader title="已归档会话" description="查看历史会话，恢复后可继续聊天。" />
    <SettingsListToolbar query={query} onQueryChange={setQuery} placeholder="搜索会话或项目">
      <Select value={selected?.workspace || "all"} onValueChange={(value) => setWorkspace(value === "all" ? null : value)}>
        <SelectTrigger aria-label="筛选项目"><SelectValue /></SelectTrigger>
        <SelectContent><SelectItem value="all">全部项目</SelectItem>
          {projects.filter((group) => group.workspace).map((group) => <SelectItem key={group.workspace} value={group.workspace}>{group.name}</SelectItem>)}
        </SelectContent>
      </Select>
    </SettingsListToolbar>
    {error && <p className="inline-notice" role="alert">{error}</p>}
    {!sessions && !error && <p className="metadata">正在加载…</p>}
    {sessions && !groups.length && <SettingsEmpty>{query ? "没有匹配的会话" : "还没有已归档会话"}</SettingsEmpty>}
    {groups.map((group) => <section className="archived-group" key={group.workspace}>
      <header><Hint text={group.workspace}><h3>{group.name}<span className="settings-count">{group.sessions.length}</span></h3></Hint>
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
  </div>;
}
