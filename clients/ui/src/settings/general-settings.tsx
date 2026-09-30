import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { SettingsHeader } from "./settings-primitives";
import type { NotificationStatus, PlatformNotifications } from "../types";
import { FontOptions, PaletteOptions, ThemeOptions, type PaletteID } from "./appearance-settings";

export function GeneralSettingsPanel({ theme, setTheme, palette, setPalette, notifications, enabled, setEnabled }: {
  theme: string;
  setTheme: (theme: string) => void;
  palette: PaletteID;
  setPalette: (palette: PaletteID) => void;
  notifications: PlatformNotifications;
  enabled: boolean;
  setEnabled: (enabled: boolean) => void;
}) {
  const [status, setStatus] = useState<NotificationStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [testMessage, setTestMessage] = useState("");

  useEffect(() => {
    let active = true;
    void notifications.status().then((next) => { if (active) setStatus(next); })
      .catch(() => { if (active) setStatus({ permission: "unavailable", message: "无法读取通知权限" }); });
    return () => { active = false; };
  }, [notifications]);

  async function toggle(next: boolean) {
    if (busy) return;
    setBusy(true);
    setTestMessage("");
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

  async function sendTest() {
    if (busy) return;
    setBusy(true);
    setTestMessage("");
    try {
      await notifications.test();
      setTestMessage("测试通知已提交给系统。");
    } catch {
      setTestMessage("测试通知发送失败，请检查系统通知设置。");
    } finally {
      setBusy(false);
    }
  }

  return <>
    <SettingsHeader title="通用" description="调整外观与通知，修改后立即生效。" />
    <section className="settings-section">
      <div className="settings-section-header"><h3>外观</h3></div>
      <ThemeOptions theme={theme} setTheme={setTheme} />
      <PaletteOptions palette={palette} setPalette={setPalette} />
    </section>
    <FontOptions />
    <section className="settings-section">
      <div className="settings-section-header"><h3>通知</h3></div>
      <div className="settings-group-card">
          <div className="settings-option-row notification-setting">
            <div>
              <strong>系统通知</strong>
              <p className="settings-description">窗口未聚焦时，提醒待审批操作和任务结果。</p>
            </div>
            <Switch aria-label="系统通知" checked={enabled} disabled={busy || status?.permission === "unavailable"}
              onCheckedChange={(next) => void toggle(next)} />
          </div>
          <div className="settings-option-row"><span>检查系统通知是否正常显示</span>
            <Button variant="outline" size="sm" disabled={busy || !enabled || status?.permission !== "granted"}
              onClick={() => void sendTest()}>发送测试通知</Button>
          </div>
      </div>
          {(status?.permission === "denied" || status?.permission === "not-granted") && status.message ?
            <p className="settings-notice" role="status">{status.message}</p> : null}
          {status?.permission === "unavailable" && <p className="settings-notice" role="status">
            {status.message} <Button variant="ghost" size="sm" onClick={() => void notifications.status().then(setStatus)}>重试检测</Button>
          </p>}
          {testMessage && <p className="settings-description notification-test-status" role="status">{testMessage}</p>}
    </section>
  </>;
}
