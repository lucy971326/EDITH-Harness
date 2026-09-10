import type { Methods } from '../clients/contracts/harness';

type Create = Methods['harness/session/create'];
type Get = Methods['harness/session/get'];
type List = Methods['harness/session/list'];

const create: Create['params'] = { workspace: '/work' };
const get: Get['params'] = { sessionID: 'abc' };
const empty: List['result'] = { sessions: [] };
const session: Get['result'] = {
  session: {
    sessionID: 'abc', title: '新对话', createdAt: '2026-09-07T10:00:00Z',
    settings: { agentID: 'default', model: '', reasoningEffort: '', workspace: '/work' },
  },
};
// @ts-expect-error 必填字段不能缺失。
const missing: Create['params'] = {};
// @ts-expect-error 未知字段不能成为契约的一部分。
const unknown: Get['params'] = { sessionID: 'abc', taskID: 'other' };
// @ts-expect-error 时间在线路上是字符串，而不是 Date 对象。
const wrongTime: Get['result']['session']['createdAt'] = new Date();
// @ts-expect-error 空列表不是 null。
const nullList: List['result'] = { sessions: null };
// @ts-expect-error 空对象契约也必须拒绝未知字段。
const unknownList: List['params'] = { filter: 'all' };
// @ts-expect-error 空对象契约不能接收标量。
const scalarList: List['params'] = 'all';
void [create, get, empty, session, missing, unknown, wrongTime, nullList];

type Send = Methods['harness/session/send'];
type Subscribe = Methods['harness/session/subscribe'];
const steer: Send['params'] = { sessionID: 'abc', text: '继续' };
const subscribed: Subscribe['result'] = { subscriptionID: 'sub', snapshot: { entries: [], runs: [] } };
// @ts-expect-error 第二步只开放文字输入，不能悄悄加入未实现的图片参数。
const imageSend: Send['params'] = { sessionID: 'abc', text: '图', image: 'base64' };
// @ts-expect-error 发送不是下一轮队列。
const queued: Send['result'] = { mode: 'queued' };
// @ts-expect-error 快照里的账本必须是数组。
const nullEntries: Subscribe['result'] = { subscriptionID: 'sub', snapshot: { entries: null, runs: [] } };
void [steer, subscribed, imageSend, queued, nullEntries];
