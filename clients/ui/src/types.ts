import type { SocketFactory } from "./client/rpc.ts";

// 平台只提供当前两端确有差异的能力；页面与连接状态仍由共用 UI 持有。
export interface Platform {
  openSocket: SocketFactory;
  openExternal: (url: string) => Promise<void>;
}
