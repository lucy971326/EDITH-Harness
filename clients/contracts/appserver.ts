export interface InitializeResult { protocolVersion: number }

export interface ServerMethods {
  initialize: { params: { protocolVersion: number }; result: InitializeResult };
  'server/unsubscribe': { params: { subscriptionID: string }; result: Record<string, never> };
}
