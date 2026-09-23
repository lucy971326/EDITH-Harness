// 手工对应 kernel/session/types.go、kernel/runner/types.go 与产品 Snapshot。
export interface Block {
  kind: string;
  text?: string;
  tool?: { id: string; name: string; args: string };
  result?: { id: string; name: string; content: string; isError?: boolean };
  media?: { mime: string; data: string };
  error?: string;
}

export interface Message {
	userAuthored?: boolean;
  messageID?: string;
  sourceSessionID?: string;
  sourceTaskID?: string;
  sourceRunID?: string;
  runID?: string;
  role: "user" | "assistant" | "tool" | "system" | "collaboration";
  blocks: Block[];
  incomplete?: boolean;
  afterSeq?: number;
}

export interface Entry {
  id: string;
  parentID?: string;
  seq: number;
  message: Message;
}

export interface RunDraft {
  entryID: string;
  afterEntrySeq: number;
  blocks: Block[];
}

export interface RunState {
  runID: string;
  afterEntrySeq: number;
  status: "running" | "success" | "cancelled" | "failed" | "interrupted";
  error?: string;
  drafts?: RunDraft[];
  usage?: {
    inputTokens: number;
    cacheReadTokens: number;
    contextWindow: number;
  };
  diff?: RunDiffSummary;
}

export interface FileDiffSummary {
  path: string;
  operation: "add" | "update" | "delete";
  additions: number;
  deletions: number;
}

export interface RunDiffSummary {
  runID: string;
  revision: number;
  files: FileDiffSummary[];
}

export interface RunDiffFile {
  runID: string;
  revision: number;
  path: string;
  operation: "add" | "update" | "delete";
  oldContent: string | null;
  newContent: string | null;
}

export interface Snapshot {
  entries: Entry[];
  runs: RunState[];
  updateSeq: number;
  seqEpoch: string;
}

export interface RunEvent {
  sessionID: string;
  runID: string;
  kind:
    | "run-started"
    | "message-started"
    | "text-delta"
    | "reasoning-delta"
    | "tool-started"
    | "tool-finished"
    | "message"
    | "usage"
    | "run-ended"
    | "run-diff-updated"
    | "notice";
  entryID?: string;
  afterEntrySeq?: number;
  blockSeq?: number;
  text?: string;
  entry?: Entry;
  tool?: { id: string; name: string; isError: boolean };
  usage?: {
    inputTokens: number;
    cacheReadTokens: number;
    contextWindow: number;
  };
  status?: "running" | "success" | "cancelled" | "failed" | "interrupted";
  error?: string;
  diff?: RunDiffSummary | null;
  updateSeq: number;
  seqEpoch: string;
}

export interface RunNotification {
  subscriptionID: string;
  event: RunEvent;
}
