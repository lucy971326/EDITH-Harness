import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Bell, Palette } from "../icons";
import type { NotificationStatus, PlatformNotifications } from "../types";
import { FontOptions, ThemeOptions } from "./appearance-settings";

export function GeneralSettingsPanel({ theme, setTheme, notifications, enabled, setEnabled }: {
  theme: string;
  setTheme: (theme: string) => void;
  notifications: PlatformNotifications;
  enabled: boolean;
  setEnabled: (enabled: boolean) => void;
}) {
  const [detail, setDetail] = useState<"appearance" | "notifications">("appearance");
  const [status, setStatus] = useState<NotificationStatus | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let active = true;
    void notifications.status().then((next) => { if (active) setStatus(next); })
      .catch(() => { if (active) setStatus({ permission: "unavailable", message: "无法读取通知权限" }); });
    return () => { active = false; };
  }, [notifications, detail]);

  async function toggle(next: boolean) {
    if (busy) return;
    setBusy(true);
    try {
      const permission = next ? await notifications.request() : await notifications.status();
      setStatus(permission);
      if (next && permission.permission !== "granted") return;
      const active = await notifications.setEnabled(next);
      setEnabled(active);
      if (next && !active) setStatus({ permission: "unavailable", message: "系统通知暂不可用" });
    } catch {
      setStatus({ permission: "unavailable", message: "无法更新通知设置" });
    } finally {
      setBusy(false);
    }
  }

  return <>
    <header className="settings-heading"><h2>通用</h2></header>
    <div className="settings-two-pane">
      <aside className="settings-subnav" aria-label="通用项目">
        <h3>通用</h3>
        <div className="settings-subnav-list">
          <Button variant="ghost" className="settings-subnav-item" aria-pressed={detail === "appearance"}
            onClick={() => setDetail("appearance")}><Palette /><span className="settings-subnav-name">外观</span></Button>
          <Button variant="ghost" className="settings-subnav-item" aria-pressed={detail === "notifications"}
            onClick={() => setDetail("notifications")}><Bell /><span className="settings-subnav-name">通知</span></Button>
        </div>
      </aside>
      <section className="settings-detail-pane">
        <div className="settings-detail-title-row">
          <div className="settings-identity">
            <span className="settings-identity-icon">{detail === "appearance" ? <Palette /> : <Bell />}</span>
            <h3>{detail === "appearance" ? "外观" : "后台通知"}</h3>
          </div>
        </div>
        {detail === "appearance" ? <><ThemeOptions theme={theme} setTheme={setTheme} /><FontOptions /></> : <>
          <div className="settings-section notification-setting">
            <div>
              <strong>系统通知</strong>
              <p className="settings-description">窗口未聚焦时，提醒待审批操作和任务结果。</p>
            </div>
            <Switch aria-label="系统通知" checked={enabled} disabled={busy || status?.permission === "unavailable"}
              onCheckedChange={(next) => void toggle(next)} />
          </div>
          {(status?.permission === "denied" || status?.permission === "not-granted") && status.message ?
            <p className="settings-notice" role="status">{status.message}</p> : null}
          {status?.permission === "unavailable" && <p className="settings-notice" role="status">
            {status.message} <Button variant="ghost" size="sm" onClick={() => void notifications.status().then(setStatus)}>重试检测</Button>
          </p>}
        </>}
      </section>
    </div>
  </>;
}
