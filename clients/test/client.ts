import type { Methods } from '../contracts/harness.ts';
import type { ServerMethods } from '../contracts/appserver.ts';
import type { RunEvent, RunNotification } from '../contracts/run.ts';

type Calls = Methods & ServerMethods;
interface Pending {
  resolve: (value: unknown) => void;
  reject: (error: Error) => void;
  timer: ReturnType<typeof setTimeout>;
}
interface EventWaiter {
  subscriptionID: string;
  resolve: (event: RunEvent) => void;
  reject: (error: Error) => void;
  timer: ReturnType<typeof setTimeout>;
}

export class RPCFailure extends Error {
  code: number;
  constructor(code: number, message: string) { super(message); this.code = code; }
}

// 验收用 Client，不是正式 SDK；不重连、不重试有副作用的调用。
export class TestClient {
  // 网络连接与退出状态。
  private socket: WebSocket;
  private closed = false;

  // 发出的请求：用 ID 配对响应。
  private nextID = 0;
  private pending = new Map<string, Pending>();

  // 收到的运行事件：交给等待者，或暂存到队列。
  private events: RunNotification[] = [];
  private waiters: EventWaiter[] = [];

  private constructor(socket: WebSocket) {
    this.socket = socket;
    socket.addEventListener('message', this.receive.bind(this));
    socket.addEventListener('close', this.disconnected.bind(this));
    socket.addEventListener('error', this.disconnected.bind(this));
  }

  static async connect(url: string): Promise<TestClient> {
    const socket = new WebSocket(url);
    const client = new TestClient(socket);
    try {
      await new Promise<void>((resolve, reject) => {
        const timer = setTimeout(() => { socket.close(); reject(new Error('Connection timed out')); }, 5000);
        socket.addEventListener('open', () => {
          clearTimeout(timer);
          resolve();
        }, { once: true });
        socket.addEventListener('error', () => {
          clearTimeout(timer);
          reject(new Error('Connection failed'));
        }, { once: true });
      });
      const initialized = await client.call('initialize', { protocolVersion: 1 });
      if (initialized.protocolVersion !== 1) throw new Error('Unsupported protocol version');
      return client;
    } catch (error) {
      client.close();
      throw error;
    }
  }

  async call<Name extends keyof Calls>(method: Name, params: Calls[Name]['params']): Promise<Calls[Name]['result']> {
    if (this.closed) throw new Error('Connection closed');
    const id = String(++this.nextID);
    const result = await new Promise<unknown>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error('Request timed out; outcome unknown, do not retry automatically'));
      }, 10000);
      this.pending.set(id, { resolve, reject, timer });
      try {
        this.socket.send(JSON.stringify({ jsonrpc: '2.0', id, method, params }));
      } catch (error) {
        clearTimeout(timer);
        this.pending.delete(id);
        reject(error);
      }
    });
    return result as Calls[Name]['result'];
  }

  nextEvent(subscriptionID: string): Promise<RunEvent> {
    const index = this.events.findIndex(item => item.subscriptionID === subscriptionID);
    if (index >= 0) return Promise.resolve(this.events.splice(index, 1)[0].event);
    if (this.closed) return Promise.reject(new Error('Connection closed'));
    return new Promise((resolve, reject) => {
      const waiter: EventWaiter = {
        subscriptionID, resolve, reject,
        timer: setTimeout(() => {
          this.waiters = this.waiters.filter(item => item !== waiter);
          reject(new Error('Event timed out'));
        }, 10000),
      };
      this.waiters.push(waiter);
    });
  }

  async untilEnded(subscriptionID: string): Promise<RunEvent> {
    for (;;) {
      const event = await this.nextEvent(subscriptionID);
      if (event.kind === 'run-ended') return event;
    }
  }

  close(): void {
    this.disconnected();
    this.socket.close();
  }

  private receive(event: MessageEvent): void {
    try {
      const envelope = JSON.parse(String(event.data));
      if (envelope.jsonrpc !== '2.0') throw new Error('Invalid JSON-RPC version');
      if (typeof envelope.method === 'string') {
        if ('id' in envelope) {
          // 本测试 Client 不实现反向请求，但不会把需要回答的请求静默吞掉。
          this.socket.send(JSON.stringify({ jsonrpc: '2.0', id: envelope.id, error: { code: -32601, message: 'Method not found' } }));
          return;
        }
        if (envelope.method !== 'harness/run/event') return;
        const notification = envelope.params as RunNotification;
        if (typeof notification?.subscriptionID !== 'string' || typeof notification.event?.kind !== 'string') throw new Error('Invalid run notification');
        this.deliverRunEvent(notification);
        return;
      }
      if (typeof envelope.id !== 'string' || ('result' in envelope) === ('error' in envelope)) throw new Error('Invalid response');
      const pending = this.pending.get(envelope.id);
      if (!pending) return;
      if ('error' in envelope && (!Number.isInteger(envelope.error?.code) || typeof envelope.error?.message !== 'string')) throw new Error('Invalid error');
      this.pending.delete(envelope.id);
      clearTimeout(pending.timer);
      if ('error' in envelope) {
        pending.reject(new RPCFailure(envelope.error.code, envelope.error.message));
        return;
      }
      pending.resolve(envelope.result);
    } catch {
      this.close();
    }
  }

  // 事件有等待者就立即交付，否则保存在有界队列中。
  private deliverRunEvent(notification: RunNotification): void {
    const index = this.waiters.findIndex(item => item.subscriptionID === notification.subscriptionID);
    if (index >= 0) {
      const waiter = this.waiters.splice(index, 1)[0];
      clearTimeout(waiter.timer);
      waiter.resolve(notification.event);
      return;
    }
    if (this.events.length >= 2048) throw new Error('Test client event queue full');
    this.events.push(notification);
  }

  private disconnected(): void {
    this.closed = true;
    for (const pending of this.pending.values()) {
      clearTimeout(pending.timer);
      pending.reject(new Error('Connection closed; accepted operations may still run'));
    }
    this.pending.clear();
    for (const waiter of this.waiters) {
      clearTimeout(waiter.timer);
      waiter.reject(new Error('Connection closed'));
    }
    this.waiters = [];
    this.events = [];
  }
}
