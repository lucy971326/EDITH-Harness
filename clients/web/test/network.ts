// 被 Go 的隔离网络验收启动；使用正式 Web 的连接和投影，不使用测试 Client 的队列。
import assert from "node:assert/strict";
import { setTimeout as pause } from "node:timers/promises";
import { ChatConnection } from "../src/client/chat.ts";
import { RPCError } from "../src/client/rpc.ts";
import { activeRun, chatMessages } from "../src/state/chat.ts";

async function until(predicate: () => boolean, label: string) {
  const deadline = Date.now() + 6000;
  while (!predicate()) {
    if (Date.now() > deadline) throw new Error(`timeout: ${label}`);
    await pause(5);
  }
}

const url = process.env.HARNESS_TEST_RPC_URL!;
const workspace = process.env.HARNESS_TEST_WORKSPACE!;
const chat = new ChatConnection(
  url,
  () => {},
  () => {},
  () => {},
);
chat.connect();
try {
  await until(() => !!chat.client?.connected, "connect");
  const client = chat.client!;
  const { models } = await client.models();
  assert.ok(
    models.some(
      (model) =>
        model.id === "deepseek/deepseek-v4-flash" &&
        model.reasoningEfforts.includes("high"),
    ),
  );
  const created = await client.create(workspace);
  const id = created.session.sessionID;
  chat.select(id);
  await until(() => !chat.state.syncing && !!chat.state.snapshot, "snapshot");
  assert.equal(chat.state.snapshot!.entries.length, 0);
  const params = {
    sessionID: id,
    model: "deepseek/deepseek-v4-flash",
    reasoningEffort: "high",
  };
  await client.send({ ...params, text: "你好" });
  await until(
    () => chat.state.snapshot?.runs[0]?.status === "success",
    "instant completion",
  );
  const live = chatMessages(chat.state.snapshot!);
  const restored = await client.call("harness/session/snapshot", {
    sessionID: id,
  });
  assert.deepEqual(live, chatMessages(restored));
  assert.equal((await client.get(id)).session.title, "你好");
  await client.send({ ...params, text: "hold" });
  await until(
    () =>
      !!chat.state.snapshot?.runs.some((run) =>
        run.drafts?.some((draft) =>
          draft.blocks.some((block) => block.text === "waiting"),
        ),
      ),
    "partial draft",
  );
  const runID = activeRun(chat.state.snapshot)!.runID;
  const draftID = activeRun(chat.state.snapshot)!.drafts![0].entryID;
  chat.connect(); // 关闭旧 socket，但后台 Run 不停止。
  await until(
    () => !!chat.client?.connected && !chat.state.syncing,
    "reconnect",
  );
  assert.equal(activeRun(chat.state.snapshot)!.runID, runID);
  assert.equal(activeRun(chat.state.snapshot)!.drafts![0].entryID, draftID);
  await chat.client!.send({
    sessionID: id,
    text: "调整方向",
    expectedRunID: runID,
  });
  await until(
    () =>
      chat.state.snapshot!.entries.some(
        (item) => item.message.blocks[0]?.text === "调整方向",
      ),
    "steer",
  );
  await chat.client!.stop(id);
  await until(
    () =>
      chat.state.snapshot!.runs.find((run) => run.runID === runID)?.status ===
      "cancelled",
    "stop",
  );
  assert.equal(
    chat.state.snapshot!.entries.find((item) => item.id === draftID)?.message
      .incomplete,
    true,
  );
  const count = chat.state.snapshot!.entries.length;
  await assert.rejects(
    chat.client!.send({ sessionID: id, text: "late", expectedRunID: runID }),
    (error) => error instanceof RPCError && error.code === -32009,
  );
  assert.equal(chat.state.snapshot!.entries.length, count);
  const final = chatMessages(chat.state.snapshot!);
  await chat.synchronize();
  assert.deepEqual(chatMessages(chat.state.snapshot!), final);
  console.log(
    "PASS 正式 Web Client: 模型/发送/历史同形/半句重连/定向插话/停止/同 ID 恢复",
  );
} finally {
  chat.close();
}
