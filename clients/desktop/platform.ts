import { Browser, Stream } from "@wailsio/runtime";
import type { Platform } from "../ui/src/types";

export const platform: Platform = {
  async openSocket() {
    return await Stream("rpc") as WebSocket;
  },
  async openExternal(url) {
    await Browser.OpenURL(url);
  },
};
