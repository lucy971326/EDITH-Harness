import type { PendingApproval, PermissionModeChoice, ApprovalSettings, ApprovalSettingsView } from "./approvals.ts";
export interface InitializeResult { protocolVersion: number }

// 手工对应 kernel/llm.ModelChoice；只有目录数据，没有 Provider 密钥。
export interface ModelChoice {
  id: string;
  contextWindow: number;
  vision: boolean;
  reasoningEfforts: string[];
}

export interface SelectWorkspaceResult {
  canceled: boolean;
  workspace: string;
}

export interface AgentView {
  id: string;
  name: string;
  kind: string;
  systemPrompt: string;
  tools: string[];
  inUse: boolean;
}

export interface AgentKindChoice { kind: string; description: string }
export interface AgentToolChoice { name: string; description: string }

export interface AgentListResult {
  agents: AgentView[];
  kinds: AgentKindChoice[];
  tools: AgentToolChoice[];
}

export interface AgentSaveParams {
  id?: string;
  name: string;
  kind: string;
  systemPrompt: string;
  tools: string[];
}

export interface SkillView {
  name: string;
  description: string;
  scope: 'system' | 'user' | 'workspace';
}

export interface CommandView {
  name: string;
  description: string;
}

export interface FileEntry {
  fileName: string;
  isDirectory: boolean;
  isFile: boolean;
}

export interface PathMatch {
  path: string;
  kind: 'file' | 'directory';
}
export interface PathSearchResult {
  entries: PathMatch[];
  truncated: boolean;
}

export interface FileMetadata {
  isDirectory: boolean;
  isFile: boolean;
  isSymlink: boolean;
  createdAtMs: number;
  modifiedAtMs: number;
}

export interface FileChangedEvent { changedPaths: string[] }
export interface FileChangedNotification { subscriptionID: string; event: FileChangedEvent }

export interface CommandExecTerminalSize { rows: number; cols: number }
export interface CommandExecOutputDeltaNotification {
  processId: string;
  stream: 'stdout';
  deltaBase64: string;
  capReached: boolean;
}

export interface ServerMethods {
  'approval/settings/read': { params: Record<string, never>; result: ApprovalSettingsView };
  'approval/settings/update': { params: ApprovalSettings; result: ApprovalSettingsView };
  'approval/subscribe': { params: Record<string, never>; result: { subscriptionID: string; pending: PendingApproval[] } };
  'permissions/modes': { params: Record<string, never>; result: { modes: PermissionModeChoice[] } };
  'approval/respond': { params: { requestID: string; decision: { approved: boolean; reason: string } }; result: Record<string, never> };
  initialize: { params: { protocolVersion: number }; result: InitializeResult };
  'server/unsubscribe': { params: { subscriptionID: string }; result: Record<string, never> };
  'workspace/select': { params: Record<string, never>; result: SelectWorkspaceResult };
  'model/list': { params: Record<string, never>; result: { models: ModelChoice[] } };
  'agent/list': { params: Record<string, never>; result: AgentListResult };
  'agent/save': { params: AgentSaveParams; result: { agent: AgentView } };
  'agent/delete': { params: { agentID: string }; result: Record<string, never> };
  'skill/list': { params: { sessionID: string }; result: { skills: SkillView[] } };
  'command/list': { params: Record<string, never>; result: { commands: CommandView[] } };
  'command/call': { params: { sessionID: string; name: string }; result: Record<string, never> };
  'fs/readFile': { params: { path: string }; result: { dataBase64: string; hash: string } };
  'fs/writeFile': { params: { path: string; dataBase64: string; expectedHash: string }; result: { hash: string } };
  'fs/readDirectory': { params: { path: string }; result: { entries: FileEntry[] } };
  'fs/searchPaths': { params: { workspace: string; query: string }; result: PathSearchResult };
  'fs/getMetadata': { params: { path: string }; result: FileMetadata };
  'fs/watch': { params: { path: string }; result: { subscriptionID: string } };
  'command/exec': {
    params: { processId: string; cwd: string; size: CommandExecTerminalSize };
    result: { exitCode: number };
  };
  'command/exec/write': {
    params: { processId: string; deltaBase64?: string; closeStdin?: boolean };
    result: Record<string, never>;
  };
  'command/exec/resize': {
    params: { processId: string; size: CommandExecTerminalSize };
    result: Record<string, never>;
  };
  'command/exec/terminate': {
    params: { processId: string };
    result: Record<string, never>;
  };
}
