import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from "@/components/ui/collapsible";
import {
  AlertDialog,
  AlertDialogTrigger,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
  AlertDialogAction,
} from "@/components/ui/alert-dialog";
import {
  Bot,
  Plus,
  ChevronRight,
  Trash2,
  Wrench,
} from "../icons";
import type {
  AgentSaveParams,
  AgentToolChoice,
  AgentView,
} from "../../../contracts/appserver.ts";

import type { SettingsDraftState } from "./types";

type AgentDraft = AgentSaveParams & { id?: string };

function draftFrom(agent: AgentView): AgentDraft {
  return {
    id: agent.id,
    name: agent.name,
    systemPrompt: agent.systemPrompt,
    tools: [...agent.tools],
  };
}

export function AgentSettingsPanel({ agents, tools, loading, error, saving, onReload, onSave, onDelete, onStateChange }: {
  agents: AgentView[] | null;
  tools: AgentToolChoice[];
  loading: boolean;
  error: string;
  saving: boolean;
  onReload: () => void;
  onSave: (agent: AgentSaveParams) => Promise<AgentView | null>;
  onDelete: (agentID: string) => Promise<boolean>;
  onStateChange: (state: SettingsDraftState) => void;
}) {
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [draft, setDraft] = useState<AgentDraft | null>(null);
  const [saved, setSaved] = useState("");
  const [pendingTarget, setPendingTarget] = useState<string | "new" | null>(null);

  useEffect(() => {
    if (draft || !agents?.length) return;
    setSelectedID(agents[0].id);
    setDraft(draftFrom(agents[0]));
  }, [agents, draft]);

  const selected = agents?.find((agent) => agent.id === selectedID);
  const cannotDelete = !selected || selected.id === "default" || selected.inUse;
  const dirty = !!draft && (!selected || draft.name !== selected.name ||
    draft.systemPrompt !== selected.systemPrompt ||
    draft.tools.length !== selected.tools.length || draft.tools.some((name) => !selected.tools.includes(name)));

  function updateDraft(change: Partial<AgentDraft>) {
    if (!draft) return;
    setDraft({ ...draft, ...change });
    setSaved("");
  }

  function edit(agent: AgentView) {
    setSelectedID(agent.id);
    setDraft(draftFrom(agent));
    setSaved("");
  }

  function showTarget(target: string | "new") {
    if (target === "new") {
      setSelectedID(null);
      setDraft({ name: "新 Agent", systemPrompt: "", tools: [] });
      setSaved("");
      return;
    }
    const agent = agents?.find((item) => item.id === target);
    if (agent) edit(agent);
  }

  function requestTarget(target: string | "new") {
    if (saving || (target === "new" && draft && selectedID === null) || (target !== "new" && target === selectedID)) return;
    if (dirty) { setPendingTarget(target); return; }
    showTarget(target);
  }

  async function save(): Promise<boolean> {
    if (!draft || saving || !dirty || !draft.name.trim()) return false;
    const result = await onSave({ ...draft, name: draft.name.trim() });
    if (!result) return false;
    setSelectedID(result.id);
    setDraft(draftFrom(result));
    setSaved("已保存。");
    return true;
  }

  useEffect(() => { onStateChange({ dirty, saving }); }, [dirty, saving, onStateChange]);

  return (
    <>
      <header className="settings-heading settings-split-heading">
        <h2>Agent</h2>
        <Button
          size="sm"
          variant="outline"
          disabled={loading || saving || agents === null}
          onClick={() => requestTarget("new")}
        >
          <Plus />
          新建 Agent
        </Button>
      </header>

      {error && (
        <div role="status" className="inline-notice">
          {error}
          <Button variant="ghost" size="sm" onClick={onReload}>
            重新加载
          </Button>
        </div>
      )}
      {loading && <p className="metadata">正在加载 Agent…</p>}
      <div className="settings-two-pane">
        <aside className="settings-subnav" aria-label="Agent 列表">
          <h3>Agent</h3>
          <div className="settings-subnav-list">
            {agents?.map((agent) => (
              <Button key={agent.id} variant="ghost" className="settings-subnav-item"
                aria-pressed={selectedID === agent.id} disabled={saving}
                onClick={() => requestTarget(agent.id)}>
                <Bot />
                <span className="settings-subnav-copy">
                  <span className="settings-subnav-name">{agent.name}</span>
                  {agent.id === "default" && <span className="settings-subnav-meta">默认</span>}
                </span>
              </Button>
            ))}
            {draft && selectedID === null && <Button variant="ghost" className="settings-subnav-item" aria-pressed="true"
              disabled={saving} onClick={() => requestTarget("new")}>
              <Bot /><span className="settings-subnav-copy"><span className="settings-subnav-name">{draft.name}</span>
                <span className="settings-subnav-meta">未创建</span></span>
            </Button>}
          </div>
        </aside>
        <div className="settings-detail-pane">
      {draft && (
        <div className="agent-form">
          <div className="settings-detail-title-row"><div className="settings-identity"><span className="settings-identity-icon"><Bot /></span><h3>{selected ? selected.name : "新建 Agent"}</h3></div></div>
          <section className="settings-section">
            <div className="settings-section-header"><div><h3>基本信息</h3></div></div>
          <div className="settings-fields">
          <div className="settings-field">
          <Label htmlFor="agent-name">名称</Label>
          <Input
            id="agent-name"
            value={draft.name}
            disabled={saving}
            onChange={(event) =>
              updateDraft({ name: event.target.value })
            }
          />
          </div>
          </div>
          </section>
          <section className="settings-section settings-field">
          <Label htmlFor="agent-prompt">系统提示词</Label>
          <Textarea
            id="agent-prompt"
            rows={5}
            value={draft.systemPrompt}
            disabled={saving}
            onChange={(event) =>
              updateDraft({ systemPrompt: event.target.value })
            }
            placeholder="告诉 Agent 如何工作…"
          />
          </section>
          <Collapsible className="tools-config settings-section">
            <CollapsibleTrigger className="ui-focus">
              <Wrench />
              <span>可用工具</span>
              <span className="settings-badge">已选 {draft.tools.length} 项</span>
              <ChevronRight className="disclosure-chevron" />
            </CollapsibleTrigger>
            <CollapsibleContent className="tool-permission-list">
              {tools.map((tool) => (
                <Collapsible className="tool-permission" key={tool.name}>
                  <div className="tool-permission-row">
                    <CollapsibleTrigger className="tool-permission-summary ui-focus" aria-label={`查看 ${tool.name} 的说明`}>
                      <Wrench /><span>{tool.name}</span><ChevronRight className="disclosure-chevron" />
                    </CollapsibleTrigger>
                  <Switch
                    id={`tool-${tool.name}`}
                    aria-label={`启用工具 ${tool.name}`}
                    checked={draft.tools.includes(tool.name)}
                    disabled={saving}
                    onCheckedChange={(checked) =>
                      updateDraft({
                        tools: checked
                          ? [...draft.tools, tool.name]
                          : draft.tools.filter(
                              (name) => name !== tool.name,
                            ),
                      })
                    }
                  />
                  </div>
                  <CollapsibleContent className="tool-permission-description">
                    <p>{tool.description}</p>
                  </CollapsibleContent>
                </Collapsible>
              ))}
            </CollapsibleContent>
          </Collapsible>
          <div className="settings-danger-row">
            <p className="settings-description">
              {selected?.id === "default" ? "默认 Agent 不可删除。" : selected?.inUse ? "该 Agent 正被会话使用，暂时无法删除。" : ""}
            </p>
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button variant="ghost" className="settings-delete" disabled={saving || cannotDelete}>
                  <Trash2 />
                  删除
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>删除 {draft.name}？</AlertDialogTitle>
                  <AlertDialogDescription>
                    删除后无法恢复；正在使用的 Agent 不允许删除。
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>取消</AlertDialogCancel>
                  <AlertDialogAction
                    onClick={() =>
                      void (async () => {
                        if (!selected || !(await onDelete(selected.id)))
                          return;
                        const fallback =
                          agents?.find(
                            (agent) => agent.id !== selected.id,
                          ) ?? null;
                        setSelectedID(fallback?.id ?? null);
                        setDraft(fallback ? draftFrom(fallback) : null);
                        setSaved("");
                      })()
                    }
                  >
                    删除
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </div>
          <div className="settings-savebar">
            <span className="settings-save-status" role="status">{dirty ? "有未保存的更改" : saved || "所有更改已保存"}</span>
            <Button variant="ghost" disabled={saving || !dirty} onClick={() => {
              const fallback = selected ?? agents?.[0];
              if (fallback) edit(fallback);
              else { setDraft(null); setSelectedID(null); setSaved(""); }
            }}>放弃</Button>
            <Button
              disabled={saving || !dirty || !draft.name.trim()}
              onClick={() => void save()}
            >
              {saving ? "保存中…" : "保存"}
            </Button>
          </div>

        </div>
      )}
        </div>
      </div>
      <AlertDialog open={pendingTarget !== null} onOpenChange={(open) => { if (!open) setPendingTarget(null); }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>保存当前 Agent 的更改？</AlertDialogTitle>
            <AlertDialogDescription>切换后，未保存的内容会丢失。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>继续编辑</AlertDialogCancel>
            <AlertDialogAction onClick={() => {
              if (pendingTarget) showTarget(pendingTarget);
              setPendingTarget(null);
            }}>放弃更改</AlertDialogAction>
            <AlertDialogAction disabled={!draft?.name.trim() || saving} onClick={() => {
              const target = pendingTarget;
              setPendingTarget(null);
              void save().then((ok) => { if (ok && target) showTarget(target); });
            }}>保存并切换</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
