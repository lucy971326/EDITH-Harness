import type { Methods } from '../appserver/generated';

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
