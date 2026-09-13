import assert from "node:assert/strict";
import { test } from "node:test";
import { chatTurns, processGroups } from "../src/state/chat-process.ts";
import { applyRunEvent } from "../src/state/chat.ts";
import type { Block, Entry, Snapshot } from "../../contracts/run.ts";

const text = (value: string): Block => ({ kind: "text", text: value });
function entry(
  seq: number,
  role: Entry["message"]["role"],
  blocks: Block[],
  afterSeq?: number,
): Entry {
  return {
    id: String(seq),
    seq,
    message: { runID: "r", role, blocks, afterSeq },
  };
}
function snapshot(
  entries: Entry[],
  status: Snapshot["runs"][number]["status"] = "success",
): Snapshot {
  return {
    entries,
    runs: [{ runID: "r", status, afterEntrySeq: 1 }],
    updateSeq: 0,
    seqEpoch: "e",
  };
}
const user = entry(1, "user", [text("问题")]);
const call: Block = {
  kind: "tool-call",
  tool: { id: "t", name: "read", args: "{}" },
};

test("normal answer moves only text outside; reasoning remains in process", () => {
  const s = snapshot([
    user,
    entry(
      2,
      "assistant",
      [{ kind: "reasoning", text: "思考" }, text("答案")],
      1,
    ),
  ]);
  const turn = chatTurns(s)[0];
  assert.equal(turn.answer?.text, "答案");
  assert.deepEqual(
    turn.items.map((item) => item.text),
    ["思考"],
  );
  assert.equal(turn.prompt?.id, "1");
  assert.deepEqual(s.entries[1].message.blocks.length, 2);
});

test("last assistant only: no backward search through tools, summary or reasoning", () => {
  for (const blocks of [
    [call],
    [{ kind: "summary", text: "压缩" }],
    [{ kind: "reasoning", text: "思考" }],
    [],
  ]) {
    const s = snapshot([
      user,
      entry(2, "assistant", [text("旧说明")], 1),
      entry(3, "assistant", blocks, 1),
    ]);
    assert.equal(chatTurns(s)[0].answer, undefined);
  }
});

test("Steer followed by empty output cannot promote pre-Steer answer, even when persisted later", () => {
  for (const oldSeq of [2, 4]) {
    const s = snapshot([
      user,
      entry(oldSeq, "assistant", [text("旧说明")], 1),
      entry(3, "user", [text("改变方向")]),
    ]);
    const turn = chatTurns(s)[0];
    assert.equal(turn.answer, undefined);
    assert.equal(
      turn.items.find((item) => item.kind === "steer")?.text,
      "改变方向",
    );
    s.entries.push(entry(5, "assistant", [text("新答案")], 3));
    assert.equal(chatTurns(s)[0].answer?.text, "新答案");
  }
});

test("failure, cancellation, interruption and incomplete never become final", () => {
  for (const status of [
    "running",
    "failed",
    "cancelled",
    "interrupted",
  ] as const) {
    const turn = chatTurns(
      snapshot([user, entry(2, "assistant", [text("正文")], 1)], status),
    )[0];
    assert.equal(turn.answer, undefined);
    assert.equal(turn.items[0].text, "正文");
  }
  const s = snapshot([user, entry(2, "assistant", [text("半截")], 1)]);
  s.entries[1].message.incomplete = true;
  assert.equal(chatTurns(s)[0].answer, undefined);
});

test("legacy history without run state still renders its complete final Markdown", () => {
  const s = snapshot([
    user,
    entry(2, "assistant", [text("## 标题\n\n- **内容**")], 1),
  ]);
  s.runs = [];

  const turn = chatTurns(s)[0];
  assert.equal(turn.run, undefined);
  assert.equal(turn.answer?.text, "## 标题\n\n- **内容**");
  assert.equal(turn.items.length, 0);
});

test("tool result fills its call exactly once, all four statuses derive from snapshot", () => {
  const s = snapshot([user, entry(2, "assistant", [call], 1)], "running");
  assert.equal(chatTurns(s)[0].items[0].status, "等待结果");
  s.runs[0].status = "interrupted";
  assert.equal(chatTurns(s)[0].items[0].status, "结果未记录");
  s.entries.push(
    entry(3, "tool", [
      {
        kind: "tool-result",
        result: { id: "t", name: "read", content: "结果" },
      },
    ]),
  );
  assert.equal(chatTurns(s)[0].items.length, 1);
  assert.equal(chatTurns(s)[0].items[0].status, "已完成");
  s.entries[2].message.blocks[0].result!.isError = true;
  assert.equal(chatTurns(s)[0].items[0].status, "异常");
});

test("only consecutive tools share a group", () => {
  const s = snapshot(
    [
      user,
      entry(
        2,
        "assistant",
        [
          call,
          { kind: "reasoning", text: "想想" },
          { ...call, tool: { id: "t2", name: "bash", args: "{}" } },
          text("接下来"),
          { ...call, tool: { id: "t3", name: "read", args: "{}" } },
        ],
        1,
      ),
    ],
    "running",
  );
  const groups = processGroups(chatTurns(s)[0].items);
  assert.equal(groups.length, 5);
  assert.ok(Array.isArray(groups[0]) && groups[0].length === 1);
  assert.equal(Array.isArray(groups[1]) ? undefined : groups[1].kind, "reasoning");
  assert.ok(Array.isArray(groups[2]) && groups[2].length === 1);
});

test("draft, durable entry and fresh snapshot share the same presentation", () => {
  const s = snapshot([user], "running");
  const live = applyRunEvent(s, {
    sessionID: "s",
    runID: "r",
    kind: "text-delta",
    entryID: "2",
    blockSeq: 1,
    text: "你好",
    afterEntrySeq: 1,
    updateSeq: 1,
    seqEpoch: "e",
  })!;
  assert.equal(chatTurns(live)[0].items[0].text, "你好");
  const durable = entry(2, "assistant", [text("你好")], 1);
  const saved = applyRunEvent(live, {
    sessionID: "s",
    runID: "r",
    kind: "message",
    entry: durable,
    updateSeq: 2,
    seqEpoch: "e",
  })!;
  const done = applyRunEvent(saved, {
    sessionID: "s",
    runID: "r",
    kind: "run-ended",
    status: "success",
    updateSeq: 3,
    seqEpoch: "e",
  })!;
  assert.deepEqual(
    chatTurns(done),
    chatTurns({ ...done, entries: [user, durable] }),
  );
  assert.equal(chatTurns(done)[0].items.length, 0);
});

test("collaboration, compact and orphan results remain visible as detail records", () => {
  const s = snapshot([
    user,
    entry(2, "collaboration", [text("孩子回报")]),
    entry(3, "assistant", [{ kind: "summary", text: "摘要" }]),
    entry(4, "tool", [
      {
        kind: "tool-result",
        result: { id: "missing", name: "read", content: "孤立" },
      },
    ]),
  ]);
  const turn = chatTurns(s)[0];
  assert.equal(turn.answer, undefined);
  assert.equal(turn.items[0].kind, "collaboration");
  assert.deepEqual(
    turn.items.map((item) => item.text),
    ["孩子回报", "摘要", "孤立"],
  );
});
