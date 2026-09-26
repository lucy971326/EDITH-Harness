import type { ComponentProps, ReactNode } from "react";
import { Plus, Settings } from "../icons";
import { SettingsPage } from "../settings/settings-page";

// 契约。动作只执行操作；页面拥有地址和内容，列表顺序就是导航顺序。
export type NavigationEntry = {
  id: string;
  label: string;
  icon: typeof Plus;
  placement: "primary" | "footer";
} & (
  | { kind: "action"; run: () => void; disabled?: boolean }
  | { kind: "page"; path: string; render: () => ReactNode; standalone?: boolean }
);

// 唯一登记入口。新增功能在此导入页面并登记；业务状态留在功能内。
export function createNavigation({
  newChat,
  settings,
}: {
  newChat: { run: () => void; disabled: boolean };
  settings: ComponentProps<typeof SettingsPage>;
}): NavigationEntry[] {
  return [
    { id: "new-chat", label: "新建会话", icon: Plus, placement: "primary", kind: "action", ...newChat },
    {
      id: "settings", label: "设置", icon: Settings, placement: "footer",
      kind: "page", path: "/settings", standalone: true,
      render: () => <SettingsPage {...settings} />,
    },
  ];
}
