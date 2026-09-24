// 手工维护，与 internal/conversations/types.go 和 internal/appserver/harness_types.go 同步修改。
// TS 只约束调用方写法；必填、长度和时间格式仍由服务端校验。
import type { RunDiffFile, RunDiffSummary, Snapshot } from "./run.js";

// 创建：结果使用共享的 SessionResult。
export interface CreateParams {
  workspace: string;
}

// 列表。
export type ListParams = Record<string, never>;

export interface ListResult {
  sessions: SessionView[];
}

// 查询、快照、订阅和停止共用的会话定位参数。
export interface SessionIDParams {
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

export type PermissionMode = "read_only" | "ask_for_approval" | "approve_for_me" | "full_access";

export interface SessionSettings {
  agentID: string;
  model: string;
  reasoningEffort: string;
  workspace: string;
  permissionMode: PermissionMode;
}

export interface UpdateSettingsParams {
  sessionID: string;
  agentID: string;
  model: string;
  reasoningEffort: string;
  permissionMode?: PermissionMode;
}

export interface ForkParams {
  sessionID: string;
  runID: string;
  boundaryEntryID: string;
}

export interface ImageInput {
  mime: "image/png" | "image/jpeg" | "image/webp";
  data: string;
}

export interface SendParams {
  sessionID: string;
  text?: string;
  images?: ImageInput[];
  expectedRunID?: string; // 非空时只插入指定 Run；结束或换轮返回 -32009。
}

export interface SubscribeResult {
  subscriptionID: string;
  snapshot: Snapshot;
}

export interface ReadRunDiffParams {
  sessionID: string;
  runID: string;
  path: string;
}
export interface RevertRunDiffParams {
  sessionID: string;
  runID: string;
  path: string;
  expectedRevision: number;
}

export type SubagentStatus =
  | "pending"
  | "running"
  | "completed"
  | "failed"
  | "cancelled"
  | "interrupted";

export interface SubagentInfo {
  taskID: string;
  taskName: string;
  description: string;
  agentID: string;
  model: string;
  reasoningEffort: string;
  workspace: string;
  status: SubagentStatus;
  turn: number;
  currentRunID?: string;
  error?: string;
}

export interface SubagentParams {
  parentSessionID: string;
  taskID: string;
}
export interface SubagentListParams {
  parentSessionID: string;
}
export interface SubagentListResult {
  tasks: SubagentInfo[];
}
export interface SubagentSubscribeResult {
  subscriptionID: string;
  task: SubagentInfo;
  snapshot: Snapshot;
  childSessionID: string;
}
export interface SubagentSendParams extends SubagentParams {
  text?: string;
  images?: ImageInput[];
}
export interface SubagentSettingsParams extends SubagentParams {
  model: string;
  reasoningEffort: string;
}
export interface ReadSubagentRunDiffParams extends SubagentParams {
  runID: string;
  path: string;
}
export interface RevertSubagentRunDiffParams extends ReadSubagentRunDiffParams {
  expectedRevision: number;
}

export interface Methods {
  "harness/session/create": { params: CreateParams; result: SessionResult };
  "harness/session/list": { params: ListParams; result: ListResult };
  "harness/session/get": { params: SessionIDParams; result: SessionResult };
  "harness/session/settings/update": {
    params: UpdateSettingsParams;
    result: SessionResult;
  };
  "harness/session/fork": { params: ForkParams; result: SessionResult };
  "harness/session/send": {
    params: SendParams;
    result: { mode: "started" | "steered" };
  };
  "harness/session/snapshot": { params: SessionIDParams; result: Snapshot };
  "harness/session/subscribe": {
    params: SessionIDParams;
    result: SubscribeResult;
  };
  "harness/session/stop": {
    params: SessionIDParams;
    result: Record<string, never>;
  };
  "harness/run/diff/read": { params: ReadRunDiffParams; result: RunDiffFile };
  "harness/run/diff/revertFile": {
    params: RevertRunDiffParams;
    result: RunDiffSummary;
  };
  "harness/subagent/list": {
    params: SubagentListParams;
    result: SubagentListResult;
  };
  "harness/subagent/subscribe": {
    params: SubagentParams;
    result: SubagentSubscribeResult;
  };
  "harness/subagent/send": {
    params: SubagentSendParams;
    result: { mode: "started" | "steered"; turn: number; runID: string };
  };
  "harness/subagent/settings/update": {
    params: SubagentSettingsParams;
    result: { task: SubagentInfo };
  };
  "harness/subagent/stop": {
    params: SubagentParams;
    result: Record<string, never>;
  };
  "harness/subagent/run/diff/read": {
    params: ReadSubagentRunDiffParams;
    result: RunDiffFile;
  };
  "harness/subagent/run/diff/revertFile": {
    params: RevertSubagentRunDiffParams;
    result: RunDiffSummary;
  };
}
