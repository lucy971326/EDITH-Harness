import { Browser, Call, Events, Stream } from "@wailsio/runtime";
import type { Platform, NotificationStatus } from "../ui/src/types";

let closeSnapshot: (() => { terminalOpen: boolean; dirty: boolean; saving: boolean }) | null = null;
let notificationOpen: ((sessionID: string) => void) | null = null;
let deferredNotificationOpen = "";

export const platform: Platform = {
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
