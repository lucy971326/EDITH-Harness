import type { ComponentType, ReactNode } from "react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Plus, X } from "../icons";

export interface AuxiliaryTab {
  id: string;
  kind: string;
  title: string;
  dirty?: boolean;
  contextPath?: string;
}

export interface AuxiliaryView {
  kind: string;
  label: string;
  icon: ComponentType<{ className?: string }>;
  render: () => ReactNode;
  onCreate: () => void;
}

export function AuxiliaryPanel({
  tabs,
  activeTabID,
  views,
  onActivateTab,
  onCloseTab,
  onHide,
}: {
  tabs: AuxiliaryTab[];
  activeTabID: string;
  views: AuxiliaryView[];
  onActivateTab: (tab: AuxiliaryTab) => void;
  onCloseTab: (tab: AuxiliaryTab) => void;
  onHide: () => void;
}) {
  const activeTab = tabs.find((tab) => tab.id === activeTabID);
  const activeView =
    views.find((view) => view.kind === activeTab?.kind) ?? views[0];

  return (
    <div className="auxiliary-workspace">
      <header className="auxiliary-tabs-header">
        <div
          className="workspace-tab-list"
          role="tablist"
          aria-label="辅助工作区标签页"
        >
          {tabs.map((tab) => {
            const view = views.find((candidate) => candidate.kind === tab.kind);
            if (!view) return null;
            const Icon = view.icon;
            return (
              <div
                className="workspace-tab"
                data-active={tab.id === activeTabID}
                key={tab.id}
              >
                <button
                  role="tab"
                  aria-selected={tab.id === activeTabID}
                  title={tab.contextPath ?? tab.title}
                  data-file-path={tab.contextPath}
                  onClick={() => onActivateTab(tab)}
                >
                  <Icon />
                  <span>{tab.title}</span>
                  {tab.dirty && <i aria-label="未保存" />}
                </button>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={`关闭 ${tab.title}`}
                  data-context-close
                  onClick={() => onCloseTab(tab)}
                >
                  <X />
                </Button>
              </div>
            );
          })}
        </div>

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="auxiliary-add"
              aria-label="打开辅助视图"
            >
              <Plus />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            {views.map((view) => {
              const Icon = view.icon;
              return (
                <DropdownMenuItem key={view.kind} onSelect={view.onCreate}>
                  <Icon />
                  {view.label}
                </DropdownMenuItem>
              );
            })}
          </DropdownMenuContent>
        </DropdownMenu>

        <Button
          variant="ghost"
          size="icon"
          className="workspace-hide"
          aria-label="关闭辅助工作区"
          onClick={onHide}
        >
          <X />
        </Button>
      </header>

      <div className="auxiliary-view">{activeView?.render()}</div>
    </div>
  );
}
