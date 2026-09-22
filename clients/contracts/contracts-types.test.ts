import type { Methods } from "./harness.js";
import type { ServerMethods } from "./appserver.js";

type Create = Methods["harness/session/create"];
type Get = Methods["harness/session/get"];
type List = Methods["harness/session/list"];

const create: Create["params"] = { workspace: "/work" };
const get: Get["params"] = { sessionID: "abc" };
const empty: List["result"] = { sessions: [] };
const session: Get["result"] = {
  session: {
    sessionID: "abc",
    title: "新对话",
    createdAt: "2026-09-07T10:00:00Z",
    settings: {
      agentID: "default",
      model: "",
      reasoningEffort: "",
      workspace: "/work",
      permissionMode: "ask_for_approval",
    },
  },
};
// @ts-expect-error 必填字段不能缺失。
const missing: Create["params"] = {};
// @ts-expect-error 未知字段不能成为契约的一部分。
const unknown: Get["params"] = { sessionID: "abc", taskID: "other" };
// @ts-expect-error 时间在线路上是字符串，而不是 Date 对象。
const wrongTime: Get["result"]["session"]["createdAt"] = new Date();
// @ts-expect-error 空列表不是 null。
const nullList: List["result"] = { sessions: null };
// @ts-expect-error 空对象契约也必须拒绝未知字段。
const unknownList: List["params"] = { filter: "all" };
// @ts-expect-error 空对象契约不能接收标量。
const scalarList: List["params"] = "all";
void [create, get, empty, session, missing, unknown, wrongTime, nullList];

type Send = Methods["harness/session/send"];
type Subscribe = Methods["harness/session/subscribe"];
const steer: Send["params"] = { sessionID: "abc", text: "继续" };
const guardedSteer: Send["params"] = {
  sessionID: "abc",
  text: "继续",
  expectedRunID: "run",
};
const numberedRun: Send["params"] = {
  sessionID: "abc",
  text: "继续",
  // @ts-expect-error 运行身份不是序号。
  expectedRunID: 1,
};
const subscribed: Subscribe["result"] = {
  subscriptionID: "sub",
  snapshot: { entries: [], runs: [], updateSeq: 0, seqEpoch: "epoch" },
};
const imageSend: Send["params"] = {
  sessionID: "abc",
  text: "图",
  // @ts-expect-error 图片必须通过已定义的 images 数组发送。
  image: "base64",
};
// @ts-expect-error 发送不是下一轮队列。
const queued: Send["result"] = { mode: "queued" };
const nullEntries: Subscribe["result"] = {
  subscriptionID: "sub",
  // @ts-expect-error 快照里的账本必须是数组。
  snapshot: { entries: null, runs: [], updateSeq: 0, seqEpoch: "epoch" },
};
void [steer, subscribed, imageSend, queued, nullEntries];

type SelectWorkspace = ServerMethods["workspace/select"];
const picked: SelectWorkspace["result"] = {
  canceled: false,
  workspace: "/work",
};
const canceledPick: SelectWorkspace["result"] = {
  canceled: true,
  workspace: "",
};
const selectParams: SelectWorkspace["params"] = {};
// @ts-expect-error 目录选择不接受参数。
const selectUnknown: SelectWorkspace["params"] = { workspace: "/work" };
// @ts-expect-error 取消也必须带上空字符串工作区。
const cancelMissingPath: SelectWorkspace["result"] = { canceled: true };
void [picked, canceledPick, selectParams, selectUnknown, cancelMissingPath];

type ModelList = ServerMethods["model/list"];
const modelList: ModelList["result"] = {
  models: [
    {
      id: "test",
      contextWindow: 1000,
      vision: false,
      reasoningEfforts: ["off"],
    },
  ],
};
// @ts-expect-error 目录不是密钥配置接口。
const modelConfig: ModelList["params"] = { apiKey: "secret" };
// @ts-expect-error 模型数组不能是 null。
const nullModels: ModelList["result"] = { models: null };
void [guardedSteer, numberedRun, modelList, modelConfig, nullModels];

type SubagentSubscribe = Methods["harness/subagent/subscribe"];
type SubagentSettings = Methods["harness/subagent/settings/update"];
const childTarget: SubagentSubscribe["params"] = {
  parentSessionID: "parent",
  taskID: "task",
};
const childSubscription: SubagentSubscribe["result"] = {
  subscriptionID: "subscription",
  childSessionID: "child",
  task: {} as SubagentSubscribe["result"]["task"],
  snapshot: {} as SubagentSubscribe["result"]["snapshot"],
};
const childSettings: SubagentSettings["params"] = {
  parentSessionID: "parent",
  taskID: "task",
  model: "model",
  reasoningEffort: "high",
};
const exposedChildSession: SubagentSubscribe["params"] = {
  // @ts-expect-error Web 端不能绕过父任务归属直接传子 Session ID。
  childSessionID: "child",
};
const changedChildAgent: SubagentSettings["params"] = {
  parentSessionID: "parent",
  taskID: "task",
  model: "model",
  reasoningEffort: "high",
  // @ts-expect-error Agent 类型和工作区不是子任务页面可修改的设置。
  agentID: "other",
};
void [childTarget, childSubscription, childSettings, exposedChildSession, changedChildAgent];

type ReadFile = ServerMethods["fs/readFile"];
type WriteFile = ServerMethods["fs/writeFile"];
type ReadDirectory = ServerMethods["fs/readDirectory"];
const readFile: ReadFile["params"] = { path: "/work/note.txt" };
const writeFile: WriteFile["params"] = {
  path: "/work/note.txt",
  dataBase64: "bm90ZQ==",
  expectedHash: "hash",
};
const directory: ReadDirectory["result"] = {
  entries: [{ fileName: "note.txt", isDirectory: false, isFile: true }],
};
// @ts-expect-error 保存必须携带读取时取得的版本。
const unsafeWrite: WriteFile["params"] = {
  path: "/work/note.txt",
  dataBase64: "bm90ZQ==",
};
// @ts-expect-error 文件内容在线路上使用 Base64 字符串。
const byteArray: ReadFile["result"] = { dataBase64: [1, 2], hash: "hash" };
void [readFile, writeFile, directory, unsafeWrite, byteArray];
