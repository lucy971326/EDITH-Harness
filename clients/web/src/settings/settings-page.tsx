import { useEffect, useState, type ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
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
  ArrowLeft,
  Sun,
  Moon,
  Monitor,
  Bot,
  Shield,
  Command,
  Plus,
  ChevronRight,
  Trash2,
  Check,
} from "../icons";
import type {
  AgentKindChoice,
  AgentSaveParams,
  AgentToolChoice,
  AgentView,
} from "../../../contracts/appserver.ts";

type AgentDraft = AgentSaveParams & { id?: string };

// 数据。设置表单向页面报告的编辑与保存状态。
export type SettingsDraftState = { dirty: boolean; saving: boolean };

function draftFrom(agent: AgentView): AgentDraft {
  return {
    id: agent.id,
    name: agent.name,
    kind: agent.kind,
    systemPrompt: agent.systemPrompt,
    tools: [...agent.tools],
  };
}

export function SettingsPage({
  theme,
  setTheme,
  onBack,
  agents,
  kinds,
  tools,
  loading,
  error,
  saving,
  onReload,
  onSave,
  onDelete,
  approvalSettings,
  hookSettings,
}: {
  theme: string;
  setTheme: (theme: string) => void;
  onBack: () => void;
  agents: AgentView[] | null;
  kinds: AgentKindChoice[];
  tools: AgentToolChoice[];
  loading: boolean;
  error: string;
  saving: boolean;
  onReload: () => void;
  onSave: (agent: AgentSaveParams) => Promise<AgentView | null>;
  onDelete: (agentID: string) => Promise<boolean>;
  approvalSettings: (onStateChange: (state: SettingsDraftState) => void) => ReactNode;
  hookSettings: (onStateChange: (state: SettingsDraftState) => void) => ReactNode;
}) {
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [draft, setDraft] = useState<AgentDraft | null>(null);
  const [saved, setSaved] = useState("");
  const [approvalState, setApprovalState] = useState<SettingsDraftState>({ dirty: false, saving: false });
  const [hookState, setHookState] = useState<SettingsDraftState>({ dirty: false, saving: false });
  const [confirmBack, setConfirmBack] = useState(false);

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

  return (
    <section className="settings-page">
      <Tabs
        defaultValue="appearance"
        orientation="vertical"
        className="settings-layout"
      >
        <div className="settings-rail">
          <Button variant="ghost" className="settings-back"
            disabled={saving || approvalState.saving || hookState.saving}
            onClick={() => {
              if (dirty || approvalState.dirty || hookState.dirty) setConfirmBack(true);
              else onBack();
            }}>
            <ArrowLeft />返回聊天
          </Button>
          <h1>设置</h1>
        <TabsList className="settings-nav" aria-label="设置分类">
          <TabsTrigger value="appearance">
            <Sun />
            外观
          </TabsTrigger>
          <TabsTrigger value="agents">
            <Bot />
            Agent
          </TabsTrigger>
          <TabsTrigger value="approvals"><Shield />智能审批</TabsTrigger>
          <TabsTrigger value="hooks"><Command />Hooks</TabsTrigger>
        </TabsList>
          <span className="settings-rail-caption">Harness · 工作台偏好</span>
        </div>
        <div className="settings-content">
          <TabsContent value="approvals" forceMount>{approvalSettings(setApprovalState)}</TabsContent>
          <TabsContent value="hooks" forceMount>{hookSettings(setHookState)}</TabsContent>
          <TabsContent value="appearance" forceMount>
            <header className="settings-heading">
              <h2>外观</h2>
              <p>让工作台适合你的使用习惯。</p>
            </header>
            <section className="settings-section">
              <div className="settings-section-header">
                <div><h3>界面主题</h3><p>选择明暗风格，或随系统自动切换。</p></div>
                <span className="settings-badge">即时生效</span>
              </div>
            <div className="theme-grid" role="group" aria-label="界面主题">
              {[
                { id: "light", name: "浅色", icon: Sun },
                { id: "dark", name: "深色", icon: Moon },
                { id: "system", name: "跟随系统", icon: Monitor },
              ].map((item) => (
                <button
                  key={item.id}
                  className={`theme-option ${theme === item.id ? "active" : ""}`}
                  onClick={() => setTheme(item.id)}
                  aria-pressed={theme === item.id}
                >
                  <span className={`theme-preview theme-preview-${item.id}`} aria-hidden="true">
                    <span className="theme-preview-sidebar"><i /><i /><i /></span>
                    <span className="theme-preview-content"><i /><i /><span /></span>
                  </span>
                  <span className="theme-option-label"><item.icon /><span>{item.name}</span>
                    <span className="theme-check">{theme === item.id && <Check />}</span>
                  </span>
                </button>
              ))}
            </div>
            </section>
          </TabsContent>

          <TabsContent value="agents" forceMount>
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
          </TabsContent>
        </div>
      </Tabs>
      <AlertDialog open={confirmBack} onOpenChange={setConfirmBack}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>放弃更改并返回聊天？</AlertDialogTitle>
            <AlertDialogDescription>设置中还有未保存的修改。你可以继续编辑，或放弃这些修改。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>继续编辑</AlertDialogCancel>
            <AlertDialogAction onClick={onBack}>放弃并返回</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  );
}
