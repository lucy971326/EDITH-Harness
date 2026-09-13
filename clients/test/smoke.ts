import assert from 'node:assert/strict';
import { TestClient, RPCFailure } from './client.ts';

// 只由 Go 集成测试启动；地址和工作区都指向临时测试环境。
const url = process.env.HARNESS_TEST_RPC_URL;
const workspace = process.env.HARNESS_TEST_WORKSPACE;
if (!url || !workspace) throw new Error('Run via npm --prefix clients run rpc:test; do not use real user data');

let client = await TestClient.connect(url);
try {
  const { session } = await client.call('harness/session/create', { workspace });
  const params = { sessionID: session.sessionID };
  await client.call('harness/session/settings/update', {
    ...params,
    agentID: 'default',
    model: 'deepseek/deepseek-v4-flash',
    reasoningEffort: 'off',
  });
  assert.equal((await client.call('harness/session/get', params)).session.sessionID, session.sessionID);
  await assert.rejects(client.call('harness/session/get', { sessionID: 'missing' }), (error: unknown) => error instanceof RPCFailure && error.code === -32004);

  // 极快完成也不能漏：先订阅，再发送，收齐终态与最终落账结果。
  let subscription = await client.call('harness/session/subscribe', params);
  assert.deepEqual(subscription.snapshot.entries, []);
  assert.deepEqual(subscription.snapshot.runs, []);
  const sent = await client.call('harness/session/send', { ...params, text: 'complete' });
  assert.equal(sent.mode, 'started');
  assert.equal((await client.untilEnded(subscription.subscriptionID)).status, 'success');
  const snapshot = await client.call('harness/session/snapshot', params);
  assert(snapshot.entries.some(entry => entry.message.role === 'assistant' && entry.message.blocks.some(block => block.text === 'local model completed')));
  assert.equal(snapshot.runs.length, 1);
  assert.equal(snapshot.runs[0].status, 'success');
  assert.equal(typeof snapshot.seqEpoch, 'string');
  assert.ok(snapshot.seqEpoch.length > 0);
  await client.call('server/unsubscribe', { subscriptionID: subscription.subscriptionID });

  // 模型持续等待：确认开始后断线，再用新连接恢复同一个 Run。
  subscription = await client.call('harness/session/subscribe', params);
  await client.call('harness/session/send', { ...params, text: 'hold' });
  let runID = '';
  let draftID = '';
  for (;;) {
    const event = await client.nextEvent(subscription.subscriptionID);
    if (event.kind === 'text-delta') {
      runID = event.runID;
      draftID = event.entryID ?? '';
      break;
    }
  }
  assert.ok(draftID, 'first delta needs an entry id');
  client.close();
  client = await TestClient.connect(url);
  subscription = await client.call('harness/session/subscribe', params);
  const live = subscription.snapshot.runs.find(run => run.runID === runID);
  assert(live, 'Disconnect cancelled the run');
  assert.equal(live.status, 'running');
  assert(live.drafts?.some(draft => draft.entryID === draftID && draft.blocks.some(block => block.text === 'waiting')), 'snapshot lost in-progress text');
  const pendingSteer = client.call('harness/session/send', { ...params, text: 'steer while busy', expectedRunID: runID }).then(
    () => undefined,
    (error: unknown) => error,
  );
  await client.call('harness/session/stop', params);
  const steerError = await pendingSteer;
  assert(steerError instanceof RPCFailure && steerError.code === -32009);
  const ended = await client.untilEnded(subscription.subscriptionID);
  assert.equal(ended.runID, runID);
  assert.equal(ended.status, 'cancelled');
  const stopped = await client.call('harness/session/snapshot', params);
  assert.equal(stopped.runs.at(-1)?.status, 'cancelled');
  assert(!stopped.entries.some(entry => entry.message.blocks.some(block => block.text === 'steer while busy')));
  assert(stopped.entries.some(entry => entry.message.incomplete && entry.message.blocks.some(block => block.text === 'waiting')));
  console.log('PASS: create / query / send / completion / reconnect / steer / stop');
} finally {
  client.close();
}
