import type { SessionView } from "../../../contracts/harness.ts";

export type ProjectGroup = {
  workspace: string;
  name: string;
  sessions: SessionView[];
};

export function workspaceName(workspace: string): string {
  const trimmed = workspace.replace(/[\\/]+$/, "");
  const slash = Math.max(trimmed.lastIndexOf("/"), trimmed.lastIndexOf("\\"));
  if (slash < 0) return trimmed || workspace;
  return trimmed.slice(slash + 1) || workspace;
}

export function groupSessions(sessions: SessionView[]): ProjectGroup[] {
  const byWorkspace = new Map<string, SessionView[]>();
  for (const session of sessions) {
    const items = byWorkspace.get(session.settings.workspace) ?? [];
    items.push(session);
    byWorkspace.set(session.settings.workspace, items);
  }
  const groups: ProjectGroup[] = [];
  for (const [workspace, items] of byWorkspace) {
    items.sort((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt));
    groups.push({ workspace, name: workspaceName(workspace), sessions: items });
  }
  groups.sort((a, b) => a.name.localeCompare(b.name, "zh-CN"));
  return groups;
}

export function sessionInList(sessions: SessionView[], sessionID: string): boolean {
  return sessions.some((item) => item.sessionID === sessionID);
}
