import type { SocketFactory } from "./rpc.ts";

// Wails Stream 提供 RPC 用到的 WebSocket 子集；Web 页面仍连接 /rpc。
export const socketFactory: SocketFactory | undefined =
  typeof window !== "undefined" &&
  (window.location.protocol === "wails:" ||
    window.location.hostname === "wails.localhost")
    ? async () => {
        const { Stream } = await import("@wailsio/runtime");
        return Stream("rpc") as WebSocket;
      }
    : undefined;
