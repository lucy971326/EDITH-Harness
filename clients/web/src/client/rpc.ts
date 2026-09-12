import type {
  Methods,
  SendParams,
  SubscribeResult,
} from "../../../contracts/harness.ts";
import type { ServerMethods } from "../../../contracts/appserver.ts";
import type { AgentSaveParams } from "../../../contracts/appserver.ts";
import type { RunNotification } from "../../../contracts/run.ts";

type Calls = Methods & ServerMethods;

export type ConnectionStatus = "connecting" | "connected" | "disconnected";
export type SocketFactory = (url: string) => WebSocket;
export type StatusListener = (
  status: ConnectionStatus,
  detail?: string,
) => void;

const defaultTimeoutMs = 10_000;
const openTimeoutMs = 5_000;

export const jsonrpcNotFound = -32004;
const jsonrpcInvalidParams = -32602;
const workspaceUnavailableMessage = "workspace is not available";

export class RPCError extends Error {
  code: number;
  constructor(code: number, message: string) {
    super(message);
    this.code = code;
  }
}

export function isSessionNotFound(error: unknown): boolean {
  return error instanceof RPCError && error.code === jsonrpcNotFound;
}

export function shouldClearSessionOnGetError(error: unknown): boolean {
  return isSessionNotFound(error);
}

export function isWorkspaceUnavailable(error: unknown): boolean {
  return (
    error instanceof RPCError &&
    error.code === jsonrpcInvalidParams &&
    error.message === workspaceUnavailableMessage
  );
}

interface Pending {
  resolve: (value: unknown) => void;
  reject: (error: Error) => void;
  timer: ReturnType<typeof setTimeout> | null;
}

export interface CallOptions<Result = unknown> {
  timeoutMs?: number | null;
  // 订阅边界须在响应这一刻安装，先于紧随其后的通知。
  accept?: (result: Result) => void;
}

// 浏览器 JSON-RPC 连接。请求 ID 只配对响应；有副作用的调用超时或断线后不自动重发。
export class RPCClient {
  onRun: ((notification: RunNotification) => void) | null = null;
  private readonly url: string;
  private readonly onStatus: StatusListener;
  private readonly openSocket: SocketFactory;
  private socket: WebSocket | null = null;
  private nextID = 0;
  private pending = new Map<string, Pending>();
  private closed = false;
  private initialized = false;
  private status: ConnectionStatus = "connecting";
  private started = false;

  constructor(
    url: string,
    onStatus: StatusListener,
    openSocket: SocketFactory = (target) => new WebSocket(target),
  ) {
    this.url = url;
    this.onStatus = onStatus;
    this.openSocket = openSocket;
  }

  get connected(): boolean {
    return this.status === "connected" && this.initialized && !this.closed;
  }

  async connect(): Promise<void> {
    if (this.closed) throw new Error("连接已关闭");
    if (this.started) throw new Error("请用新连接重试");
    this.started = true;
    this.status = "connecting";
    this.onStatus("connecting");
    const socket = this.openSocket(this.url);
    this.socket = socket;
    try {
      await waitForOpen(socket);
      if (this.closed || this.socket !== socket) throw new Error("连接已关闭");
      socket.addEventListener("message", this.receive);
      socket.addEventListener("close", this.handleDisconnect);
      socket.addEventListener("error", this.handleDisconnect);
      const initialized = await this.call("initialize", { protocolVersion: 1 });
      if (initialized.protocolVersion !== 1) {
        throw new Error("不支持的协议版本");
      }
      this.initialized = true;
      this.status = "connected";
      this.onStatus("connected");
    } catch (error) {
      this.fail(error);
      throw error;
    }
  }

  async call<Name extends keyof Calls>(
    method: Name,
    params: Calls[Name]["params"],
    options?: CallOptions<Calls[Name]["result"]>,
  ): Promise<Calls[Name]["result"]> {
    if (
      this.closed ||
      this.socket == null ||
      this.socket.readyState !== WebSocket.OPEN
    ) {
      throw new Error("未连接或尚未初始化");
    }
    if (!this.initialized && method !== "initialize") {
      throw new Error("未连接或尚未初始化");
    }
    const id = String(++this.nextID);
    const timeoutMs =
      options?.timeoutMs === undefined ? defaultTimeoutMs : options.timeoutMs;
    const result = await new Promise<unknown>((resolve, reject) => {
      const pending: Pending = {
        resolve(value) {
          try {
            options?.accept?.(value as Calls[Name]["result"]);
            resolve(value);
          } catch (error) {
            reject(error instanceof Error ? error : new Error("响应处理失败"));
          }
        },
        reject,
        timer: null,
      };
      if (timeoutMs != null) {
        pending.timer = setTimeout(() => {
          this.pending.delete(id);
          reject(new Error("请求超时，结果不明，不会自动重发"));
        }, timeoutMs);
      }
      this.pending.set(id, pending);
      try {
        this.socket!.send(
          JSON.stringify({ jsonrpc: "2.0", id, method, params }),
        );
      } catch (error) {
        this.pending.delete(id);
        if (pending.timer) clearTimeout(pending.timer);
        reject(error instanceof Error ? error : new Error("发送失败"));
      }
    });
    return result as Calls[Name]["result"];
  }

  list() {
    return this.call("harness/session/list", {});
  }

  create(workspace: string) {
    return this.call("harness/session/create", { workspace });
  }

  get(sessionID: string) {
    return this.call("harness/session/get", { sessionID });
  }

  selectWorkspace() {
    return this.call("workspace/select", {}, { timeoutMs: null });
  }

  models() {
    return this.call("model/list", {});
  }

  agents() {
    return this.call("agent/list", {});
  }

  saveAgent(params: AgentSaveParams) {
    return this.call("agent/save", params);
  }

  deleteAgent(agentID: string) {
    return this.call("agent/delete", { agentID });
  }

  updateSettings(params: Methods["harness/session/settings/update"]["params"]) {
    return this.call("harness/session/settings/update", params);
  }

  send(params: SendParams) {
    return this.call("harness/session/send", params);
  }

  stop(sessionID: string) {
    return this.call("harness/session/stop", { sessionID });
  }

  async subscribe(
    sessionID: string,
    accept: (result: SubscribeResult) => void,
  ) {
    try {
      return await this.call(
        "harness/session/subscribe",
        { sessionID },
        { accept },
      );
    } catch (error) {
      // 结果不明时无法知道订阅 ID；断线让后台整体清理，不留下幽灵订阅。
      if (!(error instanceof RPCError)) this.close();
      throw error;
    }
  }

  async unsubscribe(subscriptionID: string) {
    try {
      await this.call("server/unsubscribe", { subscriptionID });
    } catch (error) {
      this.close();
      throw error;
    }
  }

  close(): void {
    this.closed = true;
    this.fail(new Error("连接已断开；已接受的操作可能仍在后台执行"));
  }

  private receive = (event: MessageEvent): void => {
    try {
      const envelope = JSON.parse(String(event.data));
      if (envelope.jsonrpc !== "2.0")
        throw new Error("Invalid JSON-RPC version");
      if (typeof envelope.method === "string") {
        if ("id" in envelope) {
          this.socket?.send(
            JSON.stringify({
              jsonrpc: "2.0",
              id: envelope.id,
              error: { code: -32601, message: "Method not found" },
            }),
          );
        } else if (
          envelope.method === "harness/run/event" &&
          typeof envelope.params?.subscriptionID === "string" &&
          envelope.params?.event
        ) {
          this.onRun?.(envelope.params as RunNotification);
        }
        return;
      }
      if (
        typeof envelope.id !== "string" ||
        "result" in envelope === "error" in envelope
      ) {
        throw new Error("Invalid response");
      }
      const pending = this.pending.get(envelope.id);
      if (!pending) return;
      if ("error" in envelope) {
        if (
          !Number.isInteger(envelope.error?.code) ||
          typeof envelope.error?.message !== "string"
        ) {
          throw new Error("Invalid error");
        }
        this.settlePending(envelope.id, pending, () => {
          pending.reject(
            new RPCError(envelope.error.code, envelope.error.message),
          );
        });
        return;
      }
      this.settlePending(envelope.id, pending, () => {
        pending.resolve(envelope.result);
      });
    } catch {
      this.fail(new Error("连接已断开；已接受的操作可能仍在后台执行"));
    }
  };

  private handleDisconnect = (): void => {
    this.fail(new Error("连接已断开；已接受的操作可能仍在后台执行"));
  };

  private fail(error: unknown): void {
    const detail = error instanceof Error ? error.message : "连接失败";
    this.initialized = false;
    this.rejectAll(error instanceof Error ? error : new Error(detail));
    const socket = this.socket;
    this.socket = null;
    socket?.removeEventListener("message", this.receive);
    socket?.removeEventListener("close", this.handleDisconnect);
    socket?.removeEventListener("error", this.handleDisconnect);
    socket?.close();
    if (this.status !== "disconnected") {
      this.status = "disconnected";
      this.onStatus("disconnected", detail);
    }
  }

  private settlePending(
    id: string,
    pending: Pending,
    settle: () => void,
  ): void {
    this.pending.delete(id);
    if (pending.timer) clearTimeout(pending.timer);
    settle();
  }

  private rejectAll(error: Error): void {
    for (const pending of this.pending.values()) {
      if (pending.timer) clearTimeout(pending.timer);
      pending.reject(error);
    }
    this.pending.clear();
  }
}

function waitForOpen(socket: WebSocket): Promise<void> {
  if (socket.readyState === WebSocket.OPEN) return Promise.resolve();
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      cleanup();
      socket.close();
      reject(new Error("连接超时"));
    }, openTimeoutMs);
    function cleanup() {
      clearTimeout(timer);
      socket.removeEventListener("open", opened);
      socket.removeEventListener("error", failed);
      socket.removeEventListener("close", failed);
    }
    function opened() {
      cleanup();
      resolve();
    }
    function failed() {
      cleanup();
      reject(new Error("连接失败"));
    }
    socket.addEventListener("open", opened);
    socket.addEventListener("error", failed);
    socket.addEventListener("close", failed);
  });
}

export function rpcURL(): string {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}/rpc`;
}

export function formatRPCError(error: unknown, fallback: string): string {
  if (error instanceof RPCError) return `${fallback}：${error.message}`;
  if (error instanceof Error && error.message) return error.message;
  return fallback;
}
