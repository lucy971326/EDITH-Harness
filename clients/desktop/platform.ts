import { Browser, Events, Stream } from "@wailsio/runtime";
import type { Platform } from "../ui/src/types";

let closeSnapshot: (() => { terminalOpen: boolean; dirty: boolean; saving: boolean }) | null = null;

export const platform: Platform = {
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
