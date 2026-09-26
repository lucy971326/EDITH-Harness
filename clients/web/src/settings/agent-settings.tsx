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
} from "../icons";
import type {
  AgentKindChoice,
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
    kind: agent.kind,
    systemPrompt: agent.systemPrompt,
    tools: [...agent.tools],
  };
}

export function AgentSettingsPanel({ agents, kinds, tools, loading, error, saving, onReload, onSave, onDelete, onStateChange }: {
  agents: AgentView[] | null;
  kinds: AgentKindChoice[];
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

  useEffect(() => {
    if (draft || !agents?.length) return;
    setSelectedID(agents[0].id);
    setDraft(draftFrom(agents[0]));
  }, [agents, draft]);

  const selected = agents?.find((agent) => agent.id === selectedID);
  const cannotDelete = !selected || selected.id === "default" || selected.inUse;
  const dirty = !!draft && (!selected || draft.name !== selected.name ||
    draft.kind !== selected.kind || draft.systemPrompt !== selected.systemPrompt ||
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

  useEffect(() => { onStateChange({ dirty, saving }); }, [dirty, saving, onStateChange]);

  return (
    <>
      <header className="settings-heading settings-section-header">
        <div><h2>Agent</h2><p>为不同任务配置提示词与可用工具。</p></div>
        <Button
          size="sm"
          variant="outline"
          disabled={loading || saving || dirty || kinds.length === 0}
          onClick={() => {
            setSelectedID(null);
            setDraft({
              name: "新 Agent",
              kind: kinds[0]?.kind ?? "",
              systemPrompt: "",
              tools: [],
            });
            setSaved("");
          }}
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
      <div className="agent-chips" role="group" aria-label="选择 Agent">
        {agents?.map((agent) => (
          <Button
            key={agent.id}
            variant={selectedID === agent.id ? "secondary" : "ghost"}
            aria-pressed={selectedID === agent.id}
            disabled={saving || dirty}
            onClick={() => edit(agent)}
          >
            <Bot />
            {agent.name}
            {agent.id === "default" && <span className="settings-badge">默认</span>}
          </Button>
        ))}
      </div>

      {draft && (
        <div className="agent-form">
          <section className="settings-section">
            <div className="settings-section-header"><div><h3>基本信息</h3><p>在聊天中选择 Agent 时，会显示这个名称。</p></div></div>
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
          {kinds.length > 1 && <div className="settings-field">
          <Label htmlFor="agent-kind">运行方式</Label>
          <select
            id="agent-kind"
            className="ui-focus ui-field agent-kind-select"
            value={draft.kind}
            disabled={saving}
            onChange={(event) =>
              updateDraft({ kind: event.target.value })
            }
          >
            {kinds.map((kind) => (
              <option key={kind.kind} value={kind.kind}>
                {kind.kind === "react" ? "ReAct · 推理与工具调用" : kind.kind}
              </option>
            ))}
          </select>
          </div>}
          </div>
          </section>
          <section className="settings-section settings-field">
          <Label htmlFor="agent-prompt">系统提示词</Label>
          <p className="settings-description">定义 Agent 的职责、工作方式和回答偏好。</p>
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
            <CollapsibleTrigger>
              <ChevronRight className="disclosure-chevron" />
              <span>可用工具</span>
              <span className="settings-badge">已选 {draft.tools.length} 项</span>
            </CollapsibleTrigger>
            <CollapsibleContent>
              {tools.map((tool) => (
                <div className="tool-permission" key={tool.name}>
                  <Label
                    htmlFor={`tool-${tool.name}`}
                    title={tool.description}
                  >
                    <span>{tool.name}</span>
                    <span className="settings-description">{tool.description}</span>
                  </Label>
                  <Switch
                    id={`tool-${tool.name}`}
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
              ))}
            </CollapsibleContent>
          </Collapsible>
          <div className="settings-savebar">
            <span className="settings-save-status" role="status">{dirty ? "有未保存的更改" : saved || "所有更改已保存"}</span>
            <Button variant="ghost" disabled={saving || !dirty} onClick={() => {
              const fallback = selected ?? agents?.[0];
              if (fallback) edit(fallback);
              else { setDraft(null); setSelectedID(null); setSaved(""); }
            }}>放弃更改</Button>
            <Button
              disabled={saving || !dirty || !draft.name.trim() || !draft.kind}
              onClick={() =>
                void (async () => {
                  const result = await onSave({
                    ...draft,
                    name: draft.name.trim(),
                  });
                  if (!result) return;
                  setSelectedID(result.id);
                  setDraft(draftFrom(result));
                  setSaved("已保存。");
                })()
              }
            >
              {saving ? "保存中…" : "保存更改"}
            </Button>
          </div>
          <div className="settings-danger-row">
            <p className="settings-description">
              {selected?.id === "default" ? "默认 Agent 始终保留，可按需修改。" : selected?.inUse ? "该 Agent 正被会话使用，暂时无法删除。" : "删除 Agent 后无法恢复。"}
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
        </div>
      )}
    </>
  );
}
