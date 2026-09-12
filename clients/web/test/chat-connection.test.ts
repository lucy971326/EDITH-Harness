import assert from "node:assert/strict";
import { test } from "node:test";
import { setImmediate } from "node:timers/promises";
import { ChatConnection, reconnectDelays } from "../src/client/chat.ts";
import type { Snapshot } from "../../contracts/run.ts";

const snapshot = (text = "old"): Snapshot => ({
  entries: [
    {
      id: "u",
      seq: 1,
      message: { runID: "run", role: "user", blocks: [{ kind: "text", text }] },
    },
  ],
  runs: [{ runID: "run", status: "running", afterEntrySeq: 1 }],
  updateSeq: 2,
  seqEpoch: "epoch",
});
class Socket extends EventTarget {
  readyState = 0;
  requests: { id: string; method: string; params: any }[] = [];
  send(raw: string) {
    const request = JSON.parse(raw);
    this.requests.push(request);
    if (request.method === "initialize")
      queueMicrotask(() => this.reply(request.id, { protocolVersion: 1 }));
    if (request.method === "server/unsubscribe")
      queueMicrotask(() => this.reply(request.id, {}));
  }
  reply(id: string, result: unknown) {
    this.frame({ jsonrpc: "2.0", id, result });
  }
  frame(envelope: unknown) {
    this.dispatchEvent(
      new MessageEvent("message", { data: JSON.stringify(envelope) }),
    );
  }
  open() {
    this.readyState = 1;
    this.dispatchEvent(new Event("open"));
  }
  close() {
    this.readyState = 3;
    this.dispatchEvent(new Event("close"));
  }
}
function setup() {
  const sockets: Socket[] = [];
  const chat = new ChatConnection(
    "ws://local/rpc",
    () => {},
    () => {},
    () => {},
    () => {
      const socket = new Socket();
      sockets.push(socket);
      queueMicrotask(() => socket.open());
      return socket as unknown as WebSocket;
    },
  );
  chat.select("one");
  chat.connect();
  return { chat, sockets };
}
function subscription(socket: Socket, index = -1) {
  return socket.requests
    .filter((request) => request.method === "harness/session/subscribe")
    .at(index)!;
}

test("snapshot is adopted synchronously before the very next notification", async () => {
  const { chat, sockets } = setup();
  try {
    await setImmediate();
    const socket = sockets[0];
    assert.equal(chat.state.syncing, true);
    socket.reply(subscription(socket).id, {
      subscriptionID: "sub",
      snapshot: snapshot(),
    });
    socket.frame({
      jsonrpc: "2.0",
      method: "harness/run/event",
      params: {
        subscriptionID: "sub",
        event: {
          sessionID: "one",
          runID: "run",
          kind: "text-delta",
          entryID: "a",
          blockSeq: 1,
          afterEntrySeq: 1,
          text: "即时",
          updateSeq: 3,
          seqEpoch: "epoch",
        },
      },
    });
    assert.equal(
      chat.state.snapshot!.runs[0].drafts![0].blocks[0].text,
      "即时",
    );
    assert.equal(chat.state.syncing, false);
  } finally {
    chat.close();
  }
});

test("late old selection response is unsubscribed and cannot replace current history", async () => {
  const { chat, sockets } = setup();
  try {
    await setImmediate();
    const socket = sockets[0],
      old = subscription(socket);
    chat.select("two");
    const current = subscription(socket);
    socket.reply(current.id, {
      subscriptionID: "current",
      snapshot: snapshot("two"),
    });
    socket.reply(old.id, { subscriptionID: "old", snapshot: snapshot("one") });
    assert.equal(chat.state.sessionID, "two");
    assert.equal(chat.state.snapshot!.entries[0].message.blocks[0].text, "two");
    assert.ok(
      socket.requests.some(
        (request) =>
          request.method === "server/unsubscribe" &&
          request.params.subscriptionID === "old",
      ),
    );
    chat.select("two");
    assert.equal(
      socket.requests.filter(
        (request) => request.method === "harness/session/subscribe",
      ).length,
      2,
    );
  } finally {
    chat.close();
  }
});

test("gap and epoch mismatch suspend actions and recover through a fresh subscription", async () => {
  for (const [seqEpoch, updateSeq] of [
    ["epoch", 4],
    ["new", 3],
  ] as const) {
    const { chat, sockets } = setup();
    try {
      await setImmediate();
      const socket = sockets[0];
      socket.reply(subscription(socket).id, {
        subscriptionID: "sub",
        snapshot: snapshot(),
      });
      socket.frame({
        jsonrpc: "2.0",
        method: "harness/run/event",
        params: {
          subscriptionID: "sub",
          event: {
            sessionID: "one",
            runID: "run",
            kind: "run-ended",
            status: "success",
            updateSeq,
            seqEpoch,
          },
        },
      });
      assert.equal(chat.state.syncing, true);
      assert.equal(chat.state.snapshot!.runs[0].status, "running");
      await setImmediate();
      assert.equal(
        socket.requests.filter((r) => r.method === "harness/session/subscribe")
          .length,
        2,
      );
    } finally {
      chat.close();
    }
  }
});

test("subscription error is not an empty history or a ready session", async () => {
  const { chat, sockets } = setup();
  try {
    await setImmediate();
    const socket = sockets[0];
    socket.frame({
      jsonrpc: "2.0",
      id: subscription(socket).id,
      error: { code: -32603, message: "disk error" },
    });
    await setImmediate();
    assert.equal(chat.state.snapshot, null);
    assert.match(chat.state.error, /disk error/);
    assert.equal(chat.state.missing, false);
  } finally {
    chat.close();
  }
});

test("disconnect preserves view, automatic retry and manual retry have only one owner", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const { chat, sockets } = setup();
  try {
    await setImmediate();
    sockets[0].reply(subscription(sockets[0]).id, {
      subscriptionID: "sub",
      snapshot: snapshot(),
    });
    sockets[0].close();
    assert.equal(chat.state.snapshot!.entries.length, 1);
    assert.equal(chat.state.syncing, true);
    t.mock.timers.tick(1000);
    await setImmediate();
    assert.equal(sockets.length, 2);
    sockets[1].reply(subscription(sockets[1]).id, {
      subscriptionID: "new",
      snapshot: snapshot("restored"),
    });
    sockets[1].close();
    chat.connect(); // 手动重连取消还没触发的自动重试。
    await setImmediate();
    t.mock.timers.tick(1000);
    assert.equal(sockets.length, 3);
    assert.equal(
      sockets
        .flatMap((socket) => socket.requests)
        .filter((r) => r.method === "harness/session/send").length,
      0,
    );
    sockets[0].frame({
      jsonrpc: "2.0",
      method: "harness/run/event",
      params: { subscriptionID: "sub", event: {} },
    });
    assert.equal(
      chat.state.snapshot!.entries[0].message.blocks[0].text,
      "restored",
    );
    assert.deepEqual(reconnectDelays, [1000, 2000, 4000, 8000, 10000]);
  } finally {
    chat.close();
  }
  t.mock.timers.tick(20000);
  assert.equal(sockets.length, 3);
});
