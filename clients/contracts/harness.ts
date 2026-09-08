// 手工维护，与 products/harness/types.go 和 methods.go 同步修改。
// TS 只约束调用方写法；必填、长度和时间格式仍由服务端校验。

// 创建：结果使用共享的 SessionResult。
export interface CreateParams {
  workspace: string;
}

// 列表。
export type ListParams = Record<string, never>;

export interface ListResult {
  sessions: SessionView[];
}

// 查询：结果使用共享的 SessionResult。
export interface GetParams {
  sessionID: string;
}

// 共享结果与会话数据。
export interface SessionResult {
  session: SessionView;
}

export interface SessionView {
  sessionID: string;
  title: string;
  createdAt: string; // RFC 3339，不是 Date 对象。
  settings: SessionSettings;
}

export interface SessionSettings {
  agentID: string;
  model: string;
  reasoningEffort: string;
  workspace: string;
}

export interface Methods {
  'harness/session/create': { params: CreateParams; result: SessionResult };
  'harness/session/list': { params: ListParams; result: ListResult };
  'harness/session/get': { params: GetParams; result: SessionResult };
}
