import type { Platform } from "../ui/src/types";
import { notifications } from "./notifications";

export const platform: Platform = {
  notifications,
  openSocket() {
    const url = new URL("/rpc", window.location.href);
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
    return new WebSocket(url);
  },
  async openExternal(url) {
    window.open(url, "_blank", "noopener,noreferrer");
  },
};
