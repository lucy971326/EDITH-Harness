import { runSubscription } from "./run-subscription.ts";
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
  notice: string;
}

export const initialChatState: ChatConnectionState = {
  connection: "connecting",
  detail: "",
  sessionID: null,
  snapshot: null,
  syncing: false,
  error: "",
  missing: false,
  notice: "",
};

export const reconnectDelays = [1000, 2000, 4000, 8000, 10000];

// 一页一条连接、一个当前订阅。只恢复读取，不重发发送/停止等业务操作。
export class ChatConnection {
  client: RPCClient | null = null;
  state: ChatConnectionState = { ...initialChatState };

  private subscription: ReturnType<typeof runSubscription> | null = null;
  private attempt = 0;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private noticeTimer: ReturnType<typeof setTimeout> | null = null;
  private renderFrame: number | null = null;
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
    this.subscription?.close();
    this.subscription = null;
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
        this.subscription?.close();
        this.subscription = null;
        this.update({ syncing: this.state.sessionID !== null });
        const delay =
          reconnectDelays[Math.min(this.attempt++, reconnectDelays.length - 1)];
        this.timer = setTimeout(() => this.connect(), delay);
      },
      this.socketFactory,
    );
    this.client = client;
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
    this.update({
      sessionID,
      snapshot: null,
      syncing: sessionID !== null,
      error: "",
      missing: false,
      notice: "",
    });
    void this.synchronize();
  }

  async synchronize(): Promise<void> {
    const client = this.client;
    const sessionID = this.state.sessionID;
    this.subscription?.close();
    this.subscription = null;
    if (!client?.connected || this.closed) return;
    this.update({ syncing: sessionID !== null, error: "", missing: false });
    if (!sessionID) return;
    const subscription = runSubscription(
      client,
      (accept) => client.subscribe(sessionID, accept),
      {
        syncing: () =>
          this.update({ syncing: true, error: "", missing: false }),
        snapshot: (result) =>
          this.update({ snapshot: result.snapshot, syncing: false }),
        event: (event) => {
          if (!this.state.snapshot || event.sessionID !== sessionID) return;
          const snapshot = applyRunEvent(this.state.snapshot, event);
          if (!snapshot) {
            void this.synchronize();
            return;
          }
          if (snapshot === this.state.snapshot) return;
          if (event.kind === "notice" && event.text) {
            if (this.noticeTimer) clearTimeout(this.noticeTimer);
            this.update({ notice: event.text });
            this.noticeTimer = setTimeout(
              () => this.update({ notice: "" }),
              8000,
            );
          }
          // 模型可能在一帧内送来许多很小的增量。内部投影立即前进，画面每帧最多刷新一次。
          this.update({ snapshot }, true);
          if (
            (event.kind === "message" &&
              event.entry?.message.role === "user") ||
            event.kind === "run-ended"
          ) {
            this.metadata(client);
          }
        },
        error: (error) =>
          this.update({
            syncing: false,
            error: formatRPCError(error, "历史同步失败"),
            missing: isSessionNotFound(error),
          }),
      },
    );
    this.subscription = subscription;
    await subscription.synchronize();
  }

  close(): void {
    this.closed = true;
    this.subscription?.close();
    this.subscription = null;
    if (this.timer) clearTimeout(this.timer);
    if (this.noticeTimer) clearTimeout(this.noticeTimer);
    this.timer = null;
    if (this.renderFrame !== null && typeof cancelAnimationFrame === "function")
      cancelAnimationFrame(this.renderFrame);
    this.renderFrame = null;
    this.client?.close();
    this.client = null;
  }

  private update(
    patch: Partial<ChatConnectionState>,
    deferToFrame = false,
  ): void {
    this.state = { ...this.state, ...patch };
    if (deferToFrame && typeof requestAnimationFrame === "function") {
      if (this.renderFrame !== null) return;
      this.renderFrame = requestAnimationFrame(() => {
        this.renderFrame = null;
        if (!this.closed) this.change(this.state);
      });
      return;
    }
    if (this.renderFrame !== null && typeof cancelAnimationFrame === "function")
      cancelAnimationFrame(this.renderFrame);
    this.renderFrame = null;
    this.change(this.state);
  }
}
