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
import { SettingsHeader, SettingsListToolbar, SettingsResourceRow, SettingsEmpty } from "./settings-primitives";

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
  const [query, setQuery] = useState("");
  const [pendingTarget, setPendingTarget] = useState<string | "new" | null>(null);

  const selected = agents?.find((agent) => agent.id === selectedID);
  const cannotDelete = !selected || selected.id === "default" || selected.inUse;
  const dirty = !!draft && (!selected || draft.name !== selected.name ||
    draft.systemPrompt !== selected.systemPrompt ||
    draft.tools.length !== selected.tools.length || draft.tools.some((name) => !selected.tools.includes(name)));

  function updateDraft(change: Partial<AgentDraft>) {
    if (!draft) return;
    setDraft({ ...draft, ...change });
  }

  function edit(agent: AgentView) {
    setSelectedID(agent.id);
    setDraft(draftFrom(agent));
  }

  function showTarget(target: string | "new") {
    if (target === "list") { setSelectedID(null); setDraft(null); return; }
    if (target === "new") {
      setSelectedID(null);
      setDraft({ name: "新 Agent", systemPrompt: "", tools: [] });
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
    setSelectedID(null);
    setDraft(null);
    return true;
  }

  useEffect(() => { onStateChange({ dirty, saving }); }, [dirty, saving, onStateChange]);

  return (
    <>
      <SettingsHeader title={draft ? selected?.name ?? "新建 Agent" : "Agent"}
        description={draft ? "设置名称、系统提示词与可用工具。" : "配置不同任务使用的 Agent。"}
        back={draft ? { label: "Agent", onClick: () => requestTarget("list"), disabled: saving } : undefined}
        actions={!draft &&
        <Button
          size="sm"
          disabled={loading || saving || agents === null}
          onClick={() => requestTarget("new")}
        >
          <Plus />
          新建 Agent
        </Button>} />

      {error && (
        <div role="status" className="inline-notice">
          {error}
          <Button variant="ghost" size="sm" onClick={onReload}>
            重新加载
          </Button>
        </div>
      )}
      {loading && <p className="metadata">正在加载 Agent…</p>}
      {!draft && <>
        <SettingsListToolbar query={query} onQueryChange={setQuery} placeholder="搜索 Agent">
          <span className="settings-description">{agents?.length ?? 0} 个 Agent</span>
        </SettingsListToolbar>
        <div className="settings-resource-list">{agents?.filter((agent) => agent.name.toLowerCase().includes(query.toLowerCase())).map((agent) =>
          <SettingsResourceRow key={agent.id} icon={<Bot />} name={agent.name}
            description={agent.systemPrompt || "尚未设置系统提示词"}
            meta={agent.id === "default" ? "默认" : `${agent.tools.length} 个工具`}
            disabled={saving} onClick={() => requestTarget(agent.id)} />)}
        </div>
        {agents && !loading && !agents.some((agent) => agent.name.toLowerCase().includes(query.toLowerCase())) &&
          <SettingsEmpty>{query ? "没有匹配的 Agent" : "还没有 Agent，点击新建开始。"}</SettingsEmpty>}
      </>}
      {draft && (
        <div className="agent-form settings-editor">
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
          <div className="settings-form-actions">
            {selected?.inUse && selected.id !== "default" && <span className="settings-save-status">正在使用，暂不可删除</span>}
            {selected && selected.id !== "default" && <AlertDialog>
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
                        setSelectedID(null);
                        setDraft(null);
                      })()
                    }
                  >
                    删除
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>}
            <Button variant="ghost" disabled={saving} onClick={() => requestTarget("list")}>取消</Button>
            <Button
              disabled={saving || !dirty || !draft.name.trim()}
              onClick={() => void save()}
            >
              {saving ? "保存中…" : "保存"}
            </Button>
          </div>

        </div>
      )}
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
