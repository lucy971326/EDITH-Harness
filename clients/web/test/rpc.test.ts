import assert from "node:assert/strict";
import { test } from "node:test";
import {
  RPCClient,
  RPCError,
  shouldClearSessionOnGetError,
  type SocketFactory,
} from "../src/client/rpc.ts";

class FakeSocket extends EventTarget {
  readyState = 0;
  sent: string[] = [];
  replies: string[] = [];

  send(data: string) {
    this.sent.push(data);
    const envelope = JSON.parse(data) as { id: string; method: string };
    const reply = this.replies.shift();
    if (reply === "timeout") return;
    queueMicrotask(() => {
      const body =
        reply ??
        JSON.stringify({ jsonrpc: "2.0", id: envelope.id, result: defaultResult(envelope.method) });
      this.dispatchEvent(new MessageEvent("message", { data: body }));
    });
  }

  close() {
    this.readyState = 3;
    this.dispatchEvent(new Event("close"));
  }

  open() {
    this.readyState = 1;
    this.dispatchEvent(new Event("open"));
  }
}

function defaultResult(method: string): unknown {
  if (method === "initialize") return { protocolVersion: 1 };
  if (method === "harness/session/list") return { sessions: [] };
  if (method === "workspace/select") return { canceled: true, workspace: "" };
  return {};
}

function factory(socket: FakeSocket): SocketFactory {
  return () => {
    queueMicrotask(() => socket.open());
    return socket as unknown as WebSocket;
  };
}

test("initialize must succeed before business calls", async () => {
  const socket = new FakeSocket();
  const statuses: string[] = [];
  const client = new RPCClient("ws://example/rpc", (status) => statuses.push(status), factory(socket));
  await client.connect();
  assert.deepEqual(statuses, ["connecting", "connected"]);
  const listed = await client.list();
  assert.deepEqual(listed, { sessions: [] });
  assert.equal(JSON.parse(socket.sent[0]).method, "initialize");
  assert.equal(JSON.parse(socket.sent[1]).method, "harness/session/list");
  client.close();
});

test("initialize failure disconnects and blocks business calls", async () => {
  const socket = new FakeSocket();
  socket.replies.push(JSON.stringify({ jsonrpc: "2.0", id: "1", error: { code: -32001, message: "Initialization rejected" } }));
  const statuses: string[] = [];
  const client = new RPCClient("ws://example/rpc", (status) => statuses.push(status), factory(socket));
  await assert.rejects(() => client.connect(), RPCError);
  assert.equal(statuses.at(-1), "disconnected");
  await assert.rejects(() => client.list(), /未连接或尚未初始化/);
});

test("disconnect rejects pending requests without retry", async () => {
  const socket = new FakeSocket();
  const client = new RPCClient("ws://example/rpc", () => {}, factory(socket));
  await client.connect();
  socket.replies.push("timeout");
  const pending = client.list();
  socket.close();
  await assert.rejects(pending, /连接已断开/);
  client.close();
});

test("request timeout says outcome is unknown", async () => {
  const socket = new FakeSocket();
  const client = new RPCClient("ws://example/rpc", () => {}, factory(socket));
  await client.connect();
  socket.replies.push("timeout");
  await assert.rejects(client.call("harness/session/list", {}, { timeoutMs: 20 }), /结果不明/);
  client.close();
});

test("unsupported reverse requests return method not found", async () => {
  const socket = new FakeSocket();
  const client = new RPCClient("ws://example/rpc", () => {}, factory(socket));
  await client.connect();
  socket.dispatchEvent(
    new MessageEvent("message", {
      data: JSON.stringify({ jsonrpc: "2.0", id: "srv", method: "client/ask", params: {} }),
    }),
  );
  await Promise.resolve();
  const reply = JSON.parse(socket.sent.at(-1)!);
  assert.equal(reply.error.code, -32601);
  assert.equal(reply.error.message, "Method not found");
  client.close();
});

test("unknown notifications are ignored", async () => {
  const socket = new FakeSocket();
  const client = new RPCClient("ws://example/rpc", () => {}, factory(socket));
  await client.connect();
  socket.dispatchEvent(
    new MessageEvent("message", {
      data: JSON.stringify({ jsonrpc: "2.0", method: "harness/run/event", params: {} }),
    }),
  );
  const listed = await client.list();
  assert.deepEqual(listed, { sessions: [] });
  client.close();
});

test("malformed error still finishes a request that has no timeout", async () => {
  const socket = new FakeSocket();
  const client = new RPCClient("ws://example/rpc", () => {}, factory(socket));
  await client.connect();
  socket.replies.push("timeout");
  const pending = client.selectWorkspace();
  socket.dispatchEvent(
    new MessageEvent("message", {
      data: JSON.stringify({ jsonrpc: "2.0", id: "2", error: { code: "bad" } }),
    }),
  );
  await assert.rejects(pending, /连接已断开/);
  client.close();
});

test("only session-not-found clears the current selection", () => {
  assert.equal(shouldClearSessionOnGetError(new RPCError(-32004, "session not found")), true);
  assert.equal(shouldClearSessionOnGetError(new Error("连接已断开；已接受的操作可能仍在后台执行")), false);
  assert.equal(shouldClearSessionOnGetError(new Error("请求超时，结果不明，不会自动重发")), false);
  assert.equal(shouldClearSessionOnGetError(new RPCError(-32603, "Internal error")), false);
});

test("workspace select does not use the short timeout", async () => {
  const socket = new FakeSocket();
  const client = new RPCClient("ws://example/rpc", () => {}, factory(socket));
  await client.connect();
  socket.replies.push(
    JSON.stringify({ jsonrpc: "2.0", id: "2", result: { canceled: false, workspace: "/tmp/work" } }),
  );
  const result = await client.selectWorkspace();
  assert.equal(result.canceled, false);
  assert.equal(result.workspace, "/tmp/work");
  client.close();
});
