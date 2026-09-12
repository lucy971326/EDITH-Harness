import assert from "node:assert/strict";
import { test } from "node:test";
import {
  applyRunEvent,
  chatMessages,
  latestUsage,
  runLabel,
} from "../src/state/chat.ts";
import type { Entry, RunEvent, Snapshot } from "../../contracts/run.ts";

const empty = (): Snapshot => ({
  entries: [],
  runs: [],
  seqEpoch: "epoch",
  updateSeq: 0,
});
const entry = (
  id: string,
  seq: number,
  role: "user" | "assistant",
  text: string,
  afterSeq?: number,
): Entry => ({
  id,
  seq,
  message: {
    role,
    runID: "run",
    blocks: [{ kind: "text", text }],
    ...(afterSeq === undefined ? {} : { afterSeq }),
  },
});
function update(state: Snapshot, fields: Partial<RunEvent>): Snapshot {
  const next = applyRunEvent(state, {
    sessionID: "session",
    runID: "run",
    kind: "message",
    seqEpoch: "epoch",
    updateSeq: state.updateSeq + 1,
    ...fields,
  });
  assert.ok(next);
  return next;
}

test("live Chinese deltas and final snapshot have the same content, identity and status", () => {
  const user = entry("user", 1, "user", "你好");
  const answer = entry("answer", 2, "assistant", "你好世界", 1);
  let live = update(empty(), { entry: user, afterEntrySeq: 1 });
  live = update(live, { kind: "run-started", afterEntrySeq: 1 });
  live = update(live, {
    kind: "message-started",
    entryID: "answer",
    afterEntrySeq: 1,
  });
  live = update(live, {
    kind: "text-delta",
    entryID: "answer",
    blockSeq: 1,
    text: "你好",
  });
  const half = live;
  live = update(live, {
    kind: "text-delta",
    entryID: "answer",
    blockSeq: 1,
    text: "世界",
  });
  live = update(live, { kind: "message", entry: answer });
  live = update(live, { kind: "run-ended", status: "success" });
  const restored: Snapshot = {
    entries: [user, answer],
    runs: [{ runID: "run", afterEntrySeq: 1, status: "success" }],
    updateSeq: 7,
    seqEpoch: "epoch",
  };
  assert.deepEqual(chatMessages(live), chatMessages(restored));
  assert.equal(live.runs[0].status, restored.runs[0].status);
  assert.equal(
    chatMessages(half)[1].message.blocks[0].text,
    "你好",
    "no mutation of older snapshots",
  );
  assert.equal(live.runs[0].drafts!.length, 0);
});

test("snapshot may omit empty draft; first delta still creates its original ID and anchor", () => {
  const state: Snapshot = {
    ...empty(),
    updateSeq: 3,
    runs: [{ runID: "run", status: "running", afterEntrySeq: 1 }],
  };
  const next = update(state, {
    kind: "reasoning-delta",
    entryID: "answer",
    afterEntrySeq: 4,
    blockSeq: 2,
    text: "思考",
  });
  assert.equal(next.runs[0].drafts![0].entryID, "answer");
  assert.equal(next.runs[0].drafts![0].afterEntrySeq, 4);
  assert.equal(next.runs[0].drafts![0].blocks[1].kind, "reasoning");
});

test("duplicate updates do not append text; gaps and epoch changes request recovery", () => {
  const state = empty();
  const event: RunEvent = {
    sessionID: "s",
    runID: "run",
    kind: "run-started",
    seqEpoch: "epoch",
    updateSeq: 1,
  };
  const next = applyRunEvent(state, event)!;
  assert.equal(applyRunEvent(next, event), next);
  assert.equal(applyRunEvent(next, { ...event, updateSeq: 3 }), null);
  assert.equal(
    applyRunEvent(next, { ...event, seqEpoch: "new", updateSeq: 2 }),
    null,
  );
});

test("usage event updates the same snapshot used after reconnect", () => {
  let state = update(empty(), { kind: "run-started", afterEntrySeq: 0 });
  state = update(state, {
    kind: "usage",
    usage: { inputTokens: 12, cacheReadTokens: 19, contextWindow: 1000 },
  });
  assert.deepEqual(latestUsage(state), {
    inputTokens: 12,
    cacheReadTokens: 19,
    contextWindow: 1000,
  });
});

test("late delta cannot turn a durable entry into a draft", () => {
  let state = update(empty(), {
    entry: entry("answer", 1, "assistant", "done"),
  });
  state = update(state, {
    kind: "text-delta",
    entryID: "answer",
    blockSeq: 1,
    text: "late",
  });
  assert.equal(state.runs[0].drafts!.length, 0);
  assert.equal(state.entries[0].message.blocks[0].text, "done");
});

test("Steer does not move older output; new output follows the consumed input", () => {
  let state = update(empty(), {
    entry: entry("user", 1, "user", "first"),
    afterEntrySeq: 1,
  });
  state = update(state, {
    kind: "text-delta",
    entryID: "old",
    afterEntrySeq: 1,
    blockSeq: 1,
    text: "old",
  });
  state = update(state, { entry: entry("steer", 2, "user", "change") });
  assert.deepEqual(
    chatMessages(state).map((item) => item.id),
    ["user", "old", "steer"],
  );
  state = update(state, { entry: entry("old", 3, "assistant", "old", 1) });
  state = update(state, {
    kind: "text-delta",
    entryID: "new",
    afterEntrySeq: 2,
    blockSeq: 1,
    text: "new",
  });
  assert.deepEqual(
    chatMessages(state).map((item) => item.id),
    ["user", "old", "steer", "new"],
  );
  state = update(state, { entry: entry("new", 4, "assistant", "new", 2) });
  assert.deepEqual(
    chatMessages(state).map((item) => item.id),
    ["user", "old", "steer", "new"],
  );
});

test("tool call/result identities and block order survive both paths", () => {
  const call = entry("call", 1, "assistant", "working");
  call.message.blocks.push({
    kind: "tool-call",
    tool: { id: "tool-1", name: "read", args: "{}" },
  });
  const result: Entry = {
    id: "result",
    seq: 2,
    message: {
      runID: "run",
      role: "tool",
      blocks: [
        {
          kind: "tool-result",
          result: { id: "tool-1", name: "read", content: "file" },
        },
      ],
    },
  };
  let state = update(empty(), { entry: call });
  state = update(state, {
    kind: "tool-started",
    entryID: "call",
    blockSeq: 2,
    tool: { id: "tool-1", name: "read", isError: false },
  });
  state = update(state, { entry: result });
  assert.deepEqual(state.entries, [call, result]);
  state = update(state, { entry: entry("final", 3, "assistant", "answer", 1) });
  assert.deepEqual(
    chatMessages(state).map((item) => item.id),
    ["call", "result", "final"],
  );
});

test("long tool history matches each result without rescanning earlier calls", () => {
  const state = empty();
  const count = 200;
  let toolReads = 0;
  for (let index = 0; index < count; index++) {
    const call = entry(`call-${index}`, index * 2 + 1, "assistant", "working");
    call.message.blocks.push({
      kind: "tool-call",
      tool: {
        get id() {
          toolReads++;
          return `tool-${index}`;
        },
        name: "read",
        args: "{}",
      },
    });
    state.entries.push(call, {
      id: `result-${index}`,
      seq: index * 2 + 2,
      message: {
        role: "tool",
        runID: "run",
        blocks: [
          {
            kind: "tool-result",
            result: { id: `tool-${index}`, name: "read", content: "file" },
          },
        ],
      },
    });
  }
  const messages = chatMessages(state);
  assert.deepEqual(
    messages.map((item) => item.id),
    state.entries.map((item) => item.id),
  );
  for (let index = 0; index < count; index++) {
    assert.equal(
      messages[index * 2 + 1].position,
      messages[index * 2].position,
    );
  }
  assert.ok(toolReads <= count * 4, `repeated tool scans: ${toolReads}`);
});

test("failed/cancelled partial content stays explicitly incomplete; legacy status is unknown", () => {
  for (const status of ["failed", "cancelled", "interrupted"] as const) {
    const partial = entry("half", 2, "assistant", "半句", 1);
    partial.message.incomplete = true;
    let state = update(empty(), { entry: partial, afterEntrySeq: 1 });
    state = update(state, { kind: "run-ended", status });
    assert.equal(chatMessages(state)[0].message.incomplete, true);
    assert.equal(state.runs[0].status, status);
  }
  assert.equal(runLabel(undefined), "状态未记录");
  const legacy = {
    ...empty(),
    entries: [entry("a", 1, "assistant", "old"), entry("b", 2, "user", "next")],
  };
  assert.deepEqual(
    chatMessages(legacy).map((item) => item.id),
    ["a", "b"],
  );
});
