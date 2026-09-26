import type { ComponentProps, ReactNode } from "react";
import { Sun, Bot, Shield, Command } from "../icons";
import { AgentSettingsPanel } from "./agent-settings";
import { AppearanceSettingsPanel } from "./appearance-settings";
import type { SettingsDraftState } from "./types";

// 契约。每个分类拥有自己的表单，仅向外壳报告草稿与保存状态。
export type SettingsSection = {
  id: string;
  label: string;
  icon: typeof Sun;
  render: (onStateChange: (state: SettingsDraftState) => void) => ReactNode;
};

export type SettingsContentProps = Omit<ComponentProps<typeof AgentSettingsPanel>, "onStateChange"> &
  ComponentProps<typeof AppearanceSettingsPanel> & {
    approvalSettings: SettingsSection["render"];
    hookSettings: SettingsSection["render"];
  };

// 唯一分类登记入口；顺序同时决定导航和面板，不在外壳添加分类分支。
export function createSettingsSections(props: SettingsContentProps): SettingsSection[] {
  return [
    { id: "appearance", label: "外观", icon: Sun,
      render: () => <AppearanceSettingsPanel theme={props.theme} setTheme={props.setTheme} /> },
    { id: "agents", label: "Agent", icon: Bot,
      render: (onStateChange) => <AgentSettingsPanel
        agents={props.agents} kinds={props.kinds} tools={props.tools} loading={props.loading}
        error={props.error} saving={props.saving} onReload={props.onReload}
        onSave={props.onSave} onDelete={props.onDelete} onStateChange={onStateChange} /> },
    { id: "approvals", label: "智能审批", icon: Shield, render: props.approvalSettings },
    { id: "hooks", label: "Hooks", icon: Command, render: props.hookSettings },
  ];
}
