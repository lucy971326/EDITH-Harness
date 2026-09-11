export interface InitializeResult { protocolVersion: number }

export interface SelectWorkspaceResult {
  canceled: boolean;
  workspace: string;
}

export interface ServerMethods {
  initialize: { params: { protocolVersion: number }; result: InitializeResult };
  'server/unsubscribe': { params: { subscriptionID: string }; result: Record<string, never> };
  'workspace/select': { params: Record<string, never>; result: SelectWorkspaceResult };
}
