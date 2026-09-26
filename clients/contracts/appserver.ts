import type { PendingApproval, PermissionModeChoice, ApprovalSettings, ApprovalSettingsView } from "./approvals.ts";
export interface InitializeResult { protocolVersion: number }

// 手工对应 internal/llm.ModelChoice；只有目录数据，没有 Provider 密钥。
export interface ModelChoice {
  id: string;
  provider: string;
  contextWindow: number;
  vision: boolean;
  reasoningEfforts: string[];
}

export interface ProviderSettings {
  id: string;
  protocol: 'deepseek' | 'google' | 'openai-chat' | 'openai-responses' | 'anthropic';
  baseURL: string;
  hasAPIKey: boolean;
}
export interface ReasoningSettings {
  name: string;
  mode: 'off' | 'enabled' | 'adaptive';
  effort: string;
  budgetTokens: number;
}
export interface ModelSettings {
  key: string;
  provider: string;
  id: string;
  contextWindow: number;
  vision: boolean;
  reasoning: ReasoningSettings[];
}
export interface ModelSettingsView {
  providers: ProviderSettings[];
  models: ModelSettings[];
  presets: ModelSettings[];
  providerRevision: string;
  modelRevision: string;
}
export interface SaveProviderSettings {
  id: string;
  protocol: ProviderSettings['protocol'];
  baseURL: string;
  apiKey: string;
  clearAPIKey: boolean;
  revision: string;
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

export interface HookConfig {
  name: string;
  flow: string;
  enabled: boolean;
  tools: string[];
  command: string;
  args: string[];
  timeoutSeconds: number;
}

export interface HookSource {
  hooks: HookConfig[];
  hash: string;
  error?: string;
}

export interface HookView {
  flows: string[];
  global: HookSource;
  project: HookSource;
  workspace: string;
  trusted: boolean;
  lastError: string;
}

export interface MCPServerView {
  name: string;
  scope: 'global' | 'project';
  source: string;
  type: string;
  enabled: boolean;
  overridden: boolean;
  hasCommand: boolean;
  hasURL: boolean;
  hasCWD: boolean;
  argCount: number;
  envKeys: string[];
  headerKeys: string[];
  includeTools: string[];
  excludeTools: string[];
  status: 'saved' | 'connected' | 'failed' | 'invalid' | 'disabled';
  error?: string;
  tools: string[];
}

export interface MCPSettingsView {
  revision: string;
  global: MCPServerView[];
  project: MCPServerView[];
  globalError?: string;
  projectError?: string;
  globalPath: string;
  projectPaths: string[];
}

export interface MCPSaveInput {
  name: string;
  revision: string;
  create: boolean;
  type?: string;
  enabled?: boolean;
  command?: string;
  args?: string[];
  env?: Record<string, string | null>;
  cwd?: string;
  url?: string;
  headers?: Record<string, string | null>;
  includeTools?: string[];
  excludeTools?: string[];
}

export interface ServerMethods {
	'mcp/read': { params: { workspace: string }; result: MCPSettingsView };
	'mcp/save': { params: MCPSaveInput; result: MCPSettingsView };
	'mcp/delete': { params: { name: string; revision: string }; result: MCPSettingsView };
	'mcp/retry': { params: { name: string }; result: MCPSettingsView };
	'mcp/resetInvalid': { params: { revision: string }; result: MCPSettingsView };
  'hooks/read': { params: { workspace: string }; result: HookView };
  'hooks/save': { params: { scope: 'global' | 'project'; workspace: string; hash: string; hooks: HookConfig[] }; result: HookView };
  'hooks/trust': { params: { workspace: string; hash: string }; result: HookView };
  'approval/settings/read': { params: Record<string, never>; result: ApprovalSettingsView };
  'approval/settings/update': { params: ApprovalSettings; result: ApprovalSettingsView };
  'approval/subscribe': { params: Record<string, never>; result: { subscriptionID: string; pending: PendingApproval[] } };
  'permissions/modes': { params: Record<string, never>; result: { modes: PermissionModeChoice[] } };
  'approval/respond': { params: { requestID: string; decision: { approved: boolean; reason: string } }; result: Record<string, never> };
  initialize: { params: { protocolVersion: number }; result: InitializeResult };
  'server/unsubscribe': { params: { subscriptionID: string }; result: Record<string, never> };
  'workspace/select': { params: Record<string, never>; result: SelectWorkspaceResult };
  'model/list': { params: Record<string, never>; result: { models: ModelChoice[] } };
  'model/config/read': { params: Record<string, never>; result: ModelSettingsView };
  'model/provider/save': { params: SaveProviderSettings; result: ModelSettingsView };
  'model/provider/delete': { params: { id: string; providerRevision: string; modelRevision: string }; result: ModelSettingsView };
  'model/definition/save': { params: { model: ModelSettings; revision: string }; result: ModelSettingsView };
  'model/definition/delete': { params: { key: string; revision: string }; result: ModelSettingsView };
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
