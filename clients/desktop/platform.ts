import { Browser, Call, Events, Stream, System, Window } from "@wailsio/runtime";
import type { DesktopWindowControls, Platform, NotificationStatus } from "../ui/src/types";

let closeSnapshot: (() => { terminalOpen: boolean; dirty: boolean; saving: boolean }) | null = null;
let notificationOpen: ((sessionID: string) => void) | null = null;
let deferredNotificationOpen = "";

const windowControls: DesktopWindowControls = {
  minimize: () => Window.Minimise(),
  toggleMaximize: () => Window.ToggleMaximise(),
  close: () => Window.Close(),
  onMaximizedChange(update) {
    let active = true;
    let changed = false;
    const offMaximize = Events.On("common:WindowMaximise", () => { changed = true; update(true); });
    const offUnmaximize = Events.On("common:WindowUnMaximise", () => { changed = true; update(false); });
    const offRestore = Events.On("common:WindowRestore", () => {
      changed = true;
      void Window.IsMaximised().then((value) => { if (active) update(value); }).catch(() => {});
    });
    void Window.IsMaximised().then((value) => { if (active && !changed) update(value); }).catch(() => {});
    return () => { active = false; offMaximize(); offUnmaximize(); offRestore(); };
  },
};

const platform: Platform = {
  notifications: {
    status: () => Call.ByName("harness/clients/desktop.Notifications.Status") as Promise<NotificationStatus>,
    request: () => Call.ByName("harness/clients/desktop.Notifications.RequestAuthorization") as Promise<NotificationStatus>,
    setEnabled: (enabled) => Call.ByName("harness/clients/desktop.Notifications.SetEnabled", enabled) as Promise<boolean>,
    test: () => Call.ByName("harness/clients/desktop.Notifications.SendTest") as Promise<void>,
    onOpen(open) {
      notificationOpen = open;
      if (deferredNotificationOpen) {
        open(deferredNotificationOpen);
        deferredNotificationOpen = "";
      }
      const takePending = () => {
        void Call.ByName("harness/clients/desktop.Notifications.TakePendingOpen").then((sessionID) => {
          if (typeof sessionID !== "string" || !sessionID) return;
          if (notificationOpen) notificationOpen(sessionID);
          else deferredNotificationOpen = sessionID;
        }).catch(() => {});
      };
      const off = Events.On("desktop-notification-open", takePending);
      takePending();
      return () => { off(); if (notificationOpen === open) notificationOpen = null; };
    },
  },
  async openSocket() {
    return await Stream("rpc") as WebSocket;
  },
  async openExternal(url) {
    await Browser.OpenURL(url);
  },
  desktop: {
    onCloseRequest(snapshot, confirm) {
      closeSnapshot = snapshot;
      const offRequest = Events.On("desktop-close-request", (event) => {
        const request = event.data as { id: number };
        Events.Emit("desktop-close-state", { id: request.id, ...snapshot() });
      });
      const offConfirm = Events.On("desktop-exit-confirm", (event) => {
        confirm(event.data as { id: number; running: boolean; terminalOpen: boolean; dirty: boolean });
      });
      return () => { offRequest(); offConfirm(); closeSnapshot = null; };
    },
    answerExit(id, confirmed) {
      Events.Emit("desktop-exit-decision", {
        id, confirmed, ...(closeSnapshot?.() ?? { saving: true }),
      });
    },
  },
};

export async function createPlatform(): Promise<Platform> {
  const { OS } = await System.Environment();
  if (OS !== "windows" && OS !== "darwin") throw new Error("EDITH Desktop 仅支持 Windows 和 macOS");
  if (!platform.desktop) throw new Error("Desktop 平台能力未初始化");
  return {
    ...platform,
    desktop: { ...platform.desktop, windowControls: OS === "windows" ? windowControls : undefined },
  };
}
