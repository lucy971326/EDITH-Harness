import type { RunEvent, Snapshot } from "../../../contracts/run.ts";
import type { RPCClient } from "./rpc.ts";

type SubscriptionResult = { subscriptionID: string; snapshot: Snapshot };

// 共用订阅生命周期；页面保留自己的投影，Diff 无需维护整份聊天历史。
export function runSubscription<Result extends SubscriptionResult>(
  client: RPCClient,
  subscribe: (accept: (result: Result) => void) => Promise<unknown>,
  handlers: {
    snapshot: (result: Result) => void;
    event: (event: RunEvent) => void;
    syncing: () => void;
    error: (error: unknown) => void;
  },
) {
  let subscriptionID = "";
  let revision = 0;
  let closed = false;
  let boundary: Pick<Snapshot, "seqEpoch" | "updateSeq"> | null = null;

  const removeListener = client.onRunEvent(
    ({ subscriptionID: incoming, event }) => {
      if (closed || incoming !== subscriptionID || !boundary) return;
      if (
        event.seqEpoch !== boundary.seqEpoch ||
        event.updateSeq > boundary.updateSeq + 1
      ) {
        void synchronize();
        return;
      }
      if (event.updateSeq <= boundary.updateSeq) return;
      boundary = { seqEpoch: event.seqEpoch, updateSeq: event.updateSeq };
      handlers.event(event);
    },
  );

  async function synchronize(): Promise<void> {
    if (closed) return;
    const current = ++revision;
    const previous = subscriptionID;
    subscriptionID = "";
    boundary = null;
    handlers.syncing();
    try {
      if (previous && client.connected) await client.unsubscribe(previous);
      if (closed || current !== revision || !client.connected) return;
      await subscribe((result) => {
        if (closed || current !== revision) {
          void client.unsubscribe(result.subscriptionID).catch(() => {});
          return;
        }
        subscriptionID = result.subscriptionID;
        boundary = {
          seqEpoch: result.snapshot.seqEpoch,
          updateSeq: result.snapshot.updateSeq,
        };
        // 必须在响应回调内安装快照，下一条通知才能直接使用它。
        handlers.snapshot(result);
      });
    } catch (error) {
      if (!closed && current === revision) handlers.error(error);
    }
  }

  function close(): void {
    closed = true;
    revision++;
    removeListener();
    if (subscriptionID && client.connected)
      void client.unsubscribe(subscriptionID).catch(() => {});
    subscriptionID = "";
    boundary = null;
  }

  return { synchronize, close };
}
