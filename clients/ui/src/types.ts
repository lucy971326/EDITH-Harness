import type { SocketFactory } from "./client/rpc.ts";

// 平台只提供当前两端确有差异的能力；页面与连接状态仍由共用 UI 持有。
export interface Platform {
  openSocket: SocketFactory;
  openExternal: (url: string) => Promise<void>;
  desktop?: {
    onCloseRequest: (
      snapshot: () => { terminalOpen: boolean; dirty: boolean; saving: boolean },
      confirm: (request: { id: number; running: boolean; terminalOpen: boolean; dirty: boolean }) => void,
    ) => () => void;
    answerExit: (id: number, confirmed: boolean) => void;
  };
}
