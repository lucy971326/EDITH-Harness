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

export interface ServerMethods {
  initialize: { params: { protocolVersion: number }; result: InitializeResult };
  'server/unsubscribe': { params: { subscriptionID: string }; result: Record<string, never> };
  'workspace/select': { params: Record<string, never>; result: SelectWorkspaceResult };
  'model/list': { params: Record<string, never>; result: { models: ModelChoice[] } };
}
