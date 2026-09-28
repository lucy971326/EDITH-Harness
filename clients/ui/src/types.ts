import type { SocketFactory } from "./client/rpc.ts";
import type { RPCClient } from "./client/rpc.ts";

// 数据。平台读取的系统通知权限与可用性。
export type NotificationStatus = {
  permission: "granted" | "not-granted" | "denied" | "unavailable";
  message?: string;
};

// 契约。两端只向共用设置页暴露通知权限、开关、测试和点击目标。
export interface PlatformNotifications {
  status: () => Promise<NotificationStatus>;
  request: () => Promise<NotificationStatus>;
  setEnabled: (enabled: boolean) => Promise<boolean>;
  test: () => Promise<void>;
  subscribe?: (client: RPCClient, open: (sessionID: string) => void) => () => void;
  onOpen?: (open: (sessionID: string) => void) => () => void;
}

// 契约。只有 Windows 自绘标题栏需要窗口按钮；macOS 保留原生交通灯。
export interface DesktopWindowControls {
  minimize: () => Promise<void>;
  toggleMaximize: () => Promise<void>;
  close: () => Promise<void>;
  onMaximizedChange: (update: (maximized: boolean) => void) => () => void;
}

// 平台只提供当前两端确有差异的能力；页面与连接状态仍由共用 UI 持有。
export interface Platform {
  openSocket: SocketFactory;
  openExternal: (url: string) => Promise<void>;
  notifications: PlatformNotifications;
  desktop?: {
    windowControls?: DesktopWindowControls;
    onCloseRequest: (
      snapshot: () => { terminalOpen: boolean; dirty: boolean; saving: boolean },
      confirm: (request: { id: number; running: boolean; terminalOpen: boolean; dirty: boolean }) => void,
    ) => () => void;
    answerExit: (id: number, confirmed: boolean) => void;
  };
}
