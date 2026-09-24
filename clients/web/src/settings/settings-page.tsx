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
  approvalSettings: ReactNode;
  hookSettings: ReactNode;
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

  function edit(agent: AgentView) {
    setSelectedID(agent.id);
    setDraft(draftFrom(agent));
    setSaved("");
  }

  return (
    <section className="settings-page">
      <Button variant="ghost" onClick={onBack}>
        <ArrowLeft />
        返回聊天
      </Button>
      <h1>设置</h1>
      <Tabs
        defaultValue="appearance"
        orientation="vertical"
        className="settings-layout"
      >
        <TabsList className="settings-nav">
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
        <div className="settings-content">
          <TabsContent value="approvals">{approvalSettings}</TabsContent>
          <TabsContent value="hooks">{hookSettings}</TabsContent>
          <TabsContent value="appearance">
            <h2>外观</h2>
            <h3 className="section-label">主题</h3>
            <div className="theme-grid">
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
                  <item.icon />
                  <span>{item.name}</span>
                  {theme === item.id && <Check />}
                </button>
              ))}
            </div>
          </TabsContent>

          <TabsContent value="agents">
            <div className="settings-section-header">
              <h2>Agent</h2>
              <Button
                size="sm"
                variant="outline"
                disabled={loading || saving || kinds.length === 0}
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
                新建
              </Button>
            </div>

            {error && (
              <div role="status" className="inline-notice">
                {error}
                <Button variant="ghost" size="sm" onClick={onReload}>
                  重新加载
                </Button>
              </div>
            )}
            {loading && <p className="metadata">正在加载 Agent…</p>}
            <div className="agent-chips">
              {agents?.map((agent) => (
                <Button
                  key={agent.id}
                  variant={selectedID === agent.id ? "secondary" : "ghost"}
                  onClick={() => edit(agent)}
                >
                  {agent.name}
                </Button>
              ))}
            </div>

            {draft && (
              <div className="agent-form">
                <Label htmlFor="agent-name">名称</Label>
                <Input
                  id="agent-name"
                  value={draft.name}
                  disabled={saving}
                  onChange={(event) =>
                    setDraft({ ...draft, name: event.target.value })
                  }
                />
                <Label htmlFor="agent-kind">执行类型</Label>
                <select
                  id="agent-kind"
                  className="agent-kind-select"
                  value={draft.kind}
                  disabled={saving}
                  onChange={(event) =>
                    setDraft({ ...draft, kind: event.target.value })
                  }
                >
                  {kinds.map((kind) => (
                    <option key={kind.kind} value={kind.kind}>
                      {kind.kind}
                    </option>
                  ))}
                </select>
                <Label htmlFor="agent-prompt">系统提示词</Label>
                <Textarea
                  id="agent-prompt"
                  rows={5}
                  value={draft.systemPrompt}
                  disabled={saving}
                  onChange={(event) =>
                    setDraft({ ...draft, systemPrompt: event.target.value })
                  }
                  placeholder="告诉 Agent 如何工作…"
                />
                <Collapsible className="tools-config">
                  <CollapsibleTrigger>
                    <ChevronRight className="disclosure-chevron" />
                    高级配置 · 工具权限
                  </CollapsibleTrigger>
                  <CollapsibleContent>
                    {tools.map((tool) => (
                      <div className="tool-permission" key={tool.name}>
                        <Label
                          htmlFor={`tool-${tool.name}`}
                          title={tool.description}
                        >
                          {tool.name}
                        </Label>
                        <Switch
                          id={`tool-${tool.name}`}
                          checked={draft.tools.includes(tool.name)}
                          disabled={saving}
                          onCheckedChange={(checked) =>
                            setDraft({
                              ...draft,
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
                <div className="agent-form-footer">
                  <Button
                    disabled={saving || !draft.name.trim() || !draft.kind}
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
                  <AlertDialog>
                    <AlertDialogTrigger asChild>
                      <Button variant="ghost" disabled={saving || cannotDelete}>
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
                {cannotDelete && selected && (
                  <p className="metadata">
                    {selected.id === "default"
                      ? "默认 Agent 可以编辑，不能删除。"
                      : "该 Agent 正被会话使用，不能删除。"}
                  </p>
                )}
                {saved && (
                  <p role="status" className="muted">
                    {saved}
                  </p>
                )}
              </div>
            )}
          </TabsContent>
        </div>
      </Tabs>
    </section>
  );
}
