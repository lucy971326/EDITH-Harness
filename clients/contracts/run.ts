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
  messageID?: string;
  sourceSessionID?: string;
  sourceRunID?: string;
  runID?: string;
  role: 'user' | 'assistant' | 'tool' | 'system' | 'collaboration';
  blocks: Block[];
}

export interface Entry {
  id: string;
  parentID?: string;
  seq: number;
  message: Message;
}

export interface RunState { runID: string; afterEntrySeq: number }
export interface Snapshot { entries: Entry[]; runs: RunState[] }

export interface RunEvent {
  sessionID: string;
  runID: string;
  kind: 'run-started' | 'text-delta' | 'reasoning-delta' | 'tool-started' | 'tool-finished' | 'message' | 'usage' | 'run-ended';
  afterEntrySeq?: number;
  stepSeq?: number;
  blockSeq?: number;
  text?: string;
  entry?: Entry;
  tool?: { id: string; name: string; isError: boolean };
  usage?: { inputTokens: number; cacheReadTokens: number; contextWindow: number };
  status?: 'success' | 'cancelled' | 'failed';
  error?: string;
}

export interface RunNotification { subscriptionID: string; event: RunEvent }
