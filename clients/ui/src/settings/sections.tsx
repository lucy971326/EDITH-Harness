import type { ComponentProps, ReactNode } from "react";
import { SlidersHorizontal, Bot, Shield, Webhook, Brain, Archive, Server, BookOpenCheck } from "../icons";
import { AgentSettingsPanel } from "./agent-settings";
import { GeneralSettingsPanel } from "./general-settings";
import type { SettingsDraftState } from "./types";

// 契约。每个分类拥有自己的表单，仅向外壳报告草稿与保存状态。
export type SettingsSection = {
  id: string;
  label: string;
  icon: typeof SlidersHorizontal;
  render: (onStateChange: (state: SettingsDraftState) => void) => ReactNode;
};

export type SettingsContentProps = Omit<ComponentProps<typeof AgentSettingsPanel>, "onStateChange"> &
  ComponentProps<typeof GeneralSettingsPanel> & {
    approvalSettings: SettingsSection["render"];
    hookSettings: SettingsSection["render"];
    modelSettings: SettingsSection["render"];
    mcpSettings: SettingsSection["render"];
    skillSettings: SettingsSection["render"];
    archivedSettings: SettingsSection["render"];
  };

// 唯一分类登记入口；顺序同时决定导航和面板，不在外壳添加分类分支。
export function createSettingsSections(props: SettingsContentProps): SettingsSection[] {
  return [
    { id: "general", label: "通用", icon: SlidersHorizontal,
      render: () => <GeneralSettingsPanel theme={props.theme} setTheme={props.setTheme}
        notifications={props.notifications} enabled={props.enabled} setEnabled={props.setEnabled} /> },
    { id: "agents", label: "Agent", icon: Bot,
      render: (onStateChange) => <AgentSettingsPanel
        agents={props.agents} tools={props.tools} loading={props.loading}
        error={props.error} saving={props.saving} onReload={props.onReload}
        onSave={props.onSave} onDelete={props.onDelete} onStateChange={onStateChange} /> },
    { id: "skills", label: "Skills", icon: BookOpenCheck, render: props.skillSettings },
    { id: "models", label: "模型与供应商", icon: Brain, render: props.modelSettings },
    { id: "approvals", label: "智能审批", icon: Shield, render: props.approvalSettings },
    { id: "hooks", label: "Hooks", icon: Webhook, render: props.hookSettings },
    { id: "mcp", label: "MCP", icon: Server, render: props.mcpSettings },
    { id: "archived", label: "已归档会话", icon: Archive, render: props.archivedSettings },
  ];
}
