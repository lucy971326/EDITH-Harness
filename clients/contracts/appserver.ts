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

export interface ServerMethods {
  initialize: { params: { protocolVersion: number }; result: InitializeResult };
  'server/unsubscribe': { params: { subscriptionID: string }; result: Record<string, never> };
  'workspace/select': { params: Record<string, never>; result: SelectWorkspaceResult };
  'model/list': { params: Record<string, never>; result: { models: ModelChoice[] } };
  'agent/list': { params: Record<string, never>; result: AgentListResult };
  'agent/save': { params: AgentSaveParams; result: { agent: AgentView } };
  'agent/delete': { params: { agentID: string }; result: Record<string, never> };
}
