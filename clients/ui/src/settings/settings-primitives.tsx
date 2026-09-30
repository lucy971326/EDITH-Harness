import type { ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Hint } from "@/components/ui/tooltip";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { ArrowLeft, ChevronRight, Folder, Search } from "../icons";
import { workspaceName } from "../state/projects";

// 设置页只共用展示骨架；选择、草稿与保存仍由各领域页面持有。
export function SettingsHeader({ title, description, actions, back }: {
  title: string;
  description?: string;
  actions?: ReactNode;
  back?: { label: string; onClick: () => void; disabled?: boolean };
}) {
  return <>
    {back && <nav className="settings-breadcrumb" aria-label="返回上一级">
      <Button variant="ghost" size="sm" disabled={back.disabled} onClick={back.onClick}><ArrowLeft />{back.label}</Button>
      <ChevronRight /><span aria-current="page">{title}</span>
    </nav>}
    <header className="settings-heading settings-split-heading">
      <div><h2>{title}</h2>{description && <p>{description}</p>}</div>
      {actions && <div className="settings-toolbar">{actions}</div>}
    </header>
  </>;
}

export function SettingsListToolbar({ children, query, onQueryChange, placeholder, actions }: {
  children?: ReactNode;
  query: string;
  onQueryChange: (value: string) => void;
  placeholder: string;
  actions?: ReactNode;
}) {
  return <div className="settings-list-toolbar">
    {children}
    <div className="settings-search"><Search aria-hidden="true" />
      <Input aria-label={placeholder} placeholder={placeholder} value={query} onChange={(event) => onQueryChange(event.target.value)} />
    </div>
    {actions}
  </div>;
}

export function SettingsScope({ value, onChange, workspace, onChooseWorkspace, disabled }: {
  value: "user" | "workspace";
  onChange: (value: "user" | "workspace") => void;
  workspace: string;
  onChooseWorkspace?: () => void;
  disabled?: boolean;
}) {
  return <div className="settings-scope">
    <Select value={value} disabled={disabled} onValueChange={(next) => { if (next === "user" || next === "workspace") onChange(next); }}>
      <SelectTrigger aria-label="作用域"><SelectValue /></SelectTrigger>
      <SelectContent><SelectItem value="user">用户</SelectItem><SelectItem value="workspace">工作区</SelectItem></SelectContent>
    </Select>
    {value === "workspace" && <>
      <Hint text={workspace || "尚未选择工作区"}><span className="settings-workspace"><Folder /><span>{workspace ? workspaceName(workspace) : "未选择工作区"}</span></span></Hint>
      {onChooseWorkspace && <Button size="sm" variant="ghost" disabled={disabled} onClick={onChooseWorkspace}>{workspace ? "切换" : "选择"}</Button>}
    </>}
  </div>;
}

export function SettingsResourceRow({ icon, name, description, meta, onClick, disabled }: {
  icon: ReactNode;
  name: string;
  description?: string;
  meta?: ReactNode;
  onClick: () => void;
  disabled?: boolean;
}) {
  return <button type="button" className="settings-resource-row ui-focus" disabled={disabled} onClick={onClick}>
    <span className="settings-resource-icon">{icon}</span>
    <span className="settings-resource-copy"><strong>{name}</strong>{description && <span>{description}</span>}</span>
    {meta && <span className="settings-resource-meta">{meta}</span>}
    <ChevronRight className="settings-resource-chevron" />
  </button>;
}

export function SettingsEmpty({ children }: { children: ReactNode }) {
  return <div className="settings-empty"><p>{children}</p></div>;
}
