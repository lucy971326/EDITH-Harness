import type { Snapshot } from "../../../contracts/run.ts";
import { applyRunEvent } from "../state/chat.ts";
import {
  RPCClient,
  formatRPCError,
  isSessionNotFound,
  type ConnectionStatus,
  type SocketFactory,
} from "./rpc.ts";

export interface ChatConnectionState {
  connection: ConnectionStatus;
  detail: string;
  sessionID: string | null;
  snapshot: Snapshot | null;
  syncing: boolean;
  error: string;
  missing: boolean;
}

export const initialChatState: ChatConnectionState = {
  connection: "connecting",
  detail: "",
  sessionID: null,
  snapshot: null,
  syncing: false,
  error: "",
  missing: false,
};

export const reconnectDelays = [1000, 2000, 4000, 8000, 10000];

// 一页一条连接、一个当前订阅。只恢复读取，不重发发送/停止等业务操作。
export class ChatConnection {
  client: RPCClient | null = null;
  state: ChatConnectionState = { ...initialChatState };

  private subscriptionID: string | null = null;
  private revision = 0;
  private attempt = 0;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private closed = false;
  private readonly url: string;
  private readonly change: (state: ChatConnectionState) => void;
  private readonly ready: (client: RPCClient) => void;
  private readonly metadata: (client: RPCClient) => void;
  private readonly socketFactory?: SocketFactory;

  constructor(
    url: string,
    change: (state: ChatConnectionState) => void,
    ready: (client: RPCClient) => void,
    metadata: (client: RPCClient) => void,
    socketFactory?: SocketFactory,
  ) {
    this.url = url;
    this.change = change;
    this.ready = ready;
    this.metadata = metadata;
    this.socketFactory = socketFactory;
  }

  connect(): void {
    if (this.closed) return;
    if (this.timer) clearTimeout(this.timer);
    this.timer = null;
    const previous = this.client;
    this.client = null;
    previous?.close();
    this.subscriptionID = null;
    this.revision++;
    this.update({
      connection: "connecting",
      syncing: this.state.sessionID !== null,
      error: "",
    });

    const client = new RPCClient(
      this.url,
      (status, detail) => {
        if (this.client !== client || this.closed) return;
        this.update({ connection: status, detail: detail ?? "" });
        if (status !== "disconnected") return;
        this.revision++;
        this.subscriptionID = null;
        this.update({ syncing: this.state.sessionID !== null });
        const delay =
          reconnectDelays[Math.min(this.attempt++, reconnectDelays.length - 1)];
        this.timer = setTimeout(() => this.connect(), delay);
      },
      this.socketFactory,
    );
    this.client = client;
    client.onRun = ({ subscriptionID, event }) => {
      if (
        this.client !== client ||
        subscriptionID !== this.subscriptionID ||
        event.sessionID !== this.state.sessionID ||
        !this.state.snapshot ||
        this.state.syncing
      )
        return;
      const snapshot = applyRunEvent(this.state.snapshot, event);
      if (!snapshot) {
        void this.synchronize();
        return;
      }
      if (snapshot === this.state.snapshot) return;
      this.update({ snapshot });
      if (
        (event.kind === "message" && event.entry?.message.role === "user") ||
        event.kind === "run-ended"
      ) {
        this.metadata(client);
      }
    };
    void client
      .connect()
      .then(() => {
        if (this.client !== client || this.closed) return;
        this.attempt = 0;
        this.ready(client);
        void this.synchronize();
      })
      .catch(() => {
        /* RPCClient 已报告断线并安排重连。 */
      });
  }

  select(sessionID: string | null): void {
    if (sessionID === this.state.sessionID) return;
    this.revision++;
    this.update({
      sessionID,
      snapshot: null,
      syncing: sessionID !== null,
      error: "",
      missing: false,
    });
    void this.synchronize();
  }

  async synchronize(): Promise<void> {
    const client = this.client;
    const sessionID = this.state.sessionID;
    const revision = ++this.revision;
    const previous = this.subscriptionID;
    this.subscriptionID = null;
    if (!client?.connected || this.closed) return;
    this.update({ syncing: sessionID !== null, error: "", missing: false });
    try {
      if (previous) await client.unsubscribe(previous);
      if (!sessionID || revision !== this.revision || this.client !== client)
        return;
      await client.subscribe(sessionID, (result) => {
        if (
          this.closed ||
          revision !== this.revision ||
          this.client !== client
        ) {
          // 切走后才拿到 ID，也要解除；不能把它留到连接关闭才清理。
          void client.unsubscribe(result.subscriptionID).catch(() => {});
          return;
        }
        this.subscriptionID = result.subscriptionID;
        this.update({ snapshot: result.snapshot, syncing: false });
      });
    } catch (error) {
      if (this.closed || revision !== this.revision || this.client !== client)
        return;
      this.update({
        syncing: false,
        error: formatRPCError(error, "历史同步失败"),
        missing: isSessionNotFound(error),
      });
    }
  }

  close(): void {
    this.closed = true;
    this.revision++;
    if (this.timer) clearTimeout(this.timer);
    this.timer = null;
    this.client?.close();
    this.client = null;
  }

  private update(patch: Partial<ChatConnectionState>): void {
    this.state = { ...this.state, ...patch };
    this.change(this.state);
  }
}
