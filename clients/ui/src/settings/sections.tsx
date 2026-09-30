import type { ComponentProps, ReactNode } from "react";
import { SlidersHorizontal, Bot, Shield, Webhook, Brain, Archive, Server, BookOpenCheck, Command } from "../icons";
import { AgentSettingsPanel } from "./agent-settings";
import { GeneralSettingsPanel } from "./general-settings";
import type { SettingsDraftState } from "./types";

// 契约。每个分类拥有自己的表单，仅向外壳报告草稿与保存状态。
export type SettingsSection = {
  id: string;
  label: string;
  icon: typeof SlidersHorizontal;
  group: "基础设置" | "Agent 能力" | "会话";
  render: (onStateChange: (state: SettingsDraftState) => void) => ReactNode;
};

export type SettingsContentProps = Omit<ComponentProps<typeof AgentSettingsPanel>, "onStateChange"> &
  ComponentProps<typeof GeneralSettingsPanel> & {
    approvalSettings: SettingsSection["render"];
    hookSettings: SettingsSection["render"];
    modelSettings: SettingsSection["render"];
    mcpSettings: SettingsSection["render"];
    skillSettings: SettingsSection["render"];
    commandSettings: SettingsSection["render"];
    archivedSettings: SettingsSection["render"];
  };

// 唯一分类登记入口；顺序同时决定导航和面板，不在外壳添加分类分支。
export function createSettingsSections(props: SettingsContentProps): SettingsSection[] {
  return [
    { id: "general", label: "通用", icon: SlidersHorizontal, group: "基础设置",
      render: () => <GeneralSettingsPanel theme={props.theme} setTheme={props.setTheme}
        palette={props.palette} setPalette={props.setPalette}
        notifications={props.notifications} enabled={props.enabled} setEnabled={props.setEnabled} /> },
    { id: "models", label: "模型与供应商", icon: Brain, group: "基础设置", render: props.modelSettings },
    { id: "approvals", label: "智能审批", icon: Shield, group: "基础设置", render: props.approvalSettings },
    { id: "agents", label: "Agent", icon: Bot, group: "Agent 能力",
      render: (onStateChange) => <AgentSettingsPanel
        agents={props.agents} tools={props.tools} loading={props.loading}
        error={props.error} saving={props.saving} onReload={props.onReload}
        onSave={props.onSave} onDelete={props.onDelete} onStateChange={onStateChange} /> },
    { id: "skills", label: "Skills", icon: BookOpenCheck, group: "Agent 能力", render: props.skillSettings },
    { id: "commands", label: "命令", icon: Command, group: "Agent 能力", render: props.commandSettings },
    { id: "hooks", label: "Hooks", icon: Webhook, group: "Agent 能力", render: props.hookSettings },
    { id: "mcp", label: "MCP", icon: Server, group: "Agent 能力", render: props.mcpSettings },
    { id: "archived", label: "已归档会话", icon: Archive, group: "会话", render: props.archivedSettings },
  ];
}
