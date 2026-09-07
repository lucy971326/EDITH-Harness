// Generated from Go contracts. Do not edit.

export interface HarnessSessionListResult {
  sessions: SessionView[];
}
export interface SessionView {
  sessionID: string;
  title: string;
  createdAt: string;
  settings: SessionSettings;
}
export interface SessionSettings {
  agentID: string;
  model: string;
  reasoningEffort: string;
  workspace: string;
}
