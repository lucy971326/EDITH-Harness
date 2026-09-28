import { useCallback, useEffect, useRef, useState } from "react";
import type { SessionActivity } from "../../../contracts/harness";
import { formatRPCError, RPCError, type RPCClient } from "../client/rpc";

// 只缓存后台投影；通知先失效、串行重读，避免慢响应把新状态覆盖掉。
export function useSessionActivity(client: RPCClient | null) {
  const [sessions, setSessions] = useState<SessionActivity[]>([]);
  const [error, setError] = useState("");
  const acknowledged = useRef(new Map<string, number>());
  const pending = useRef(new Set<string>());
  const currentClient = useRef(client);
  currentClient.current = client;
  useEffect(() => {
    setSessions([]);
    setError("");
    acknowledged.current = new Map();
    pending.current = new Set();
    if (!client) return;
    let active = true;
    let subscriptionID = "";
    let loading = false;
    let dirty = false;
    async function refresh() {
      dirty = true;
      if (loading || !active) return;
      loading = true;
      try {
        while (dirty && active) {
          dirty = false;
          const result = await client!.call("harness/session/activity/list", {});
          if (active) { setSessions(result.sessions); setError(""); }
        }
      } catch (cause) {
        if (active) { setError(formatRPCError(cause, "会话状态同步失败")); if (!(cause instanceof RPCError)) client!.close(); }
      } finally { loading = false; }
    }
    client.onActivity = (id) => { if (id === subscriptionID) void refresh(); };
    void client.call("harness/session/activity/subscribe", {}, { accept(result) {
      if (!active) { void client.unsubscribe(result.subscriptionID).catch(() => {}); return; }
      subscriptionID = result.subscriptionID;
      setSessions(result.sessions);
    } }).catch((cause) => { if (active) { setError(formatRPCError(cause, "会话状态同步失败")); if (!(cause instanceof RPCError)) client.close(); } });
    return () => {
      active = false;
      client.onActivity = null;
      if (subscriptionID && client.connected) void client.unsubscribe(subscriptionID).catch(() => {});
    };
  }, [client]);
  const markRead = useCallback((sessionID: string, runID: string, seq: number) => {
    if (!client?.connected || (acknowledged.current.get(sessionID) ?? 0) >= seq) return;
    const key = `${sessionID}:${runID}`;
    if (pending.current.has(key)) return;
    const requests = pending.current;
    requests.add(key);
    void client.call("harness/session/read", { sessionID, runID }).then(() => {
      if (currentClient.current !== client || !client.connected) return;
      acknowledged.current.set(sessionID, Math.max(seq, acknowledged.current.get(sessionID) ?? 0));
    }).catch((cause) => {
      if (currentClient.current === client && client.connected) setError(formatRPCError(cause, "已读位置保存失败，未读标记已保留"));
    }).finally(() => requests.delete(key));
  }, [client]);
  return { sessions, error, markRead };
}
