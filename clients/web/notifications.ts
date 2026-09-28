import { RPCError, type RPCClient } from "../ui/src/client/rpc";
import type { PlatformNotifications, NotificationStatus } from "../ui/src/types";

function status(): NotificationStatus {
  if (!window.isSecureContext || !("Notification" in window)) {
    return { permission: "unavailable", message: "当前浏览器不支持通知" };
  }
  if (Notification.permission === "granted") return { permission: "granted" };
  if (Notification.permission === "denied") {
    return { permission: "denied", message: "请在浏览器的网站权限中允许通知" };
  }
  return { permission: "not-granted" };
}

function notify(sessionID: string, body: string, open: (sessionID: string) => void) {
  if (status().permission !== "granted" || (document.visibilityState === "visible" && document.hasFocus())) return;
  try {
    const notification = new Notification("Harness", { body, tag: `harness-${sessionID}-${body}` });
    notification.onclick = () => {
      notification.close();
      window.focus();
      open(sessionID);
    };
  } catch {
    // 浏览器或系统可能在授权后仍拒绝发送；不影响后台任务。
  }
}

export const notifications: PlatformNotifications = {
  async status() { return status(); },
  async request() {
    const current = status();
    if (current.permission !== "not-granted") return current;
    try {
      await Notification.requestPermission();
    } catch {
      return { permission: "unavailable", message: "浏览器未能请求通知权限" };
    }
    return status();
  },
  async setEnabled(enabled) { return enabled && status().permission === "granted"; },
  subscribe(client: RPCClient, open) {
    let active = true;
    let approvalID = "";
    let terminalID = "";
    let known = new Set<string>();
    const subscriptionError = (error: unknown) => {
      // 结果不明时只能断线，让后台清理可能已创建的订阅。
      if (!(error instanceof RPCError)) client.close();
    };
    const stopApproval = client.onApprovalChange((notification) => {
      if (!active || notification.subscriptionID !== approvalID) return;
      const next = new Set(notification.event.map((item) => item.id));
      for (const item of notification.event) {
        if (!known.has(item.id)) notify(item.sessionID, "有一项操作等待确认", open);
      }
      known = next;
    });
    const stopTerminal = client.onTerminal((notification) => {
      if (!active || notification.subscriptionID !== terminalID) return;
      const { sessionID, status: runStatus } = notification.event;
      notify(sessionID, runStatus === "success" ? "任务已完成" : "任务执行失败", open);
    });
    void client.call("approval/subscribe", {}, { accept(result) {
      if (!active) { void client.unsubscribe(result.subscriptionID).catch(() => {}); return; }
      approvalID = result.subscriptionID;
      known = new Set(result.pending.map((item) => item.id));
    } }).catch(subscriptionError);
    void client.call("harness/run/terminal/subscribe", {}, { accept(result) {
      if (!active) { void client.unsubscribe(result.subscriptionID).catch(() => {}); return; }
      terminalID = result.subscriptionID;
    } }).catch(subscriptionError);
    return () => {
      active = false;
      stopApproval();
      stopTerminal();
      if (client.connected) {
        if (approvalID) void client.unsubscribe(approvalID).catch(() => {});
        if (terminalID) void client.unsubscribe(terminalID).catch(() => {});
      }
    };
  },
};
