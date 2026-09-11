import { useState, type Dispatch, type SetStateAction } from "react";
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
  SlidersHorizontal,
  Plus,
  ChevronRight,
  Trash2,
  Check,
} from "./icons";
import type { Agent } from "./demo";

export function SettingsPage({
  agents,
  setAgents,
  usedAgentIds,
  theme,
  setTheme,
  onBack,
}: {
  agents: Agent[];
  setAgents: Dispatch<SetStateAction<Agent[]>>;
  usedAgentIds: string[];
  theme: string;
  setTheme: (theme: string) => void;
  onBack: () => void;
}) {
  const [selected, setSelected] = useState(agents[0].id);
  const [draft, setDraft] = useState(agents[0]);
  const [saved, setSaved] = useState("");
  const cannotDelete =
    draft.id === "default" || usedAgentIds.includes(draft.id);
  function edit(agent: Agent) {
    setSelected(agent.id);
    setDraft({ ...agent });
    setSaved("");
  }
  return (
    <section className="settings-page">
      <Button variant="ghost" onClick={onBack}>
        <ArrowLeft />
        返回聊天
      </Button>
      <h1>设置</h1>
      <Tabs defaultValue="appearance" orientation="vertical" className="settings-layout">
        <TabsList className="settings-nav">
          <TabsTrigger value="appearance">
            <SlidersHorizontal />
            外观
          </TabsTrigger>
          <TabsTrigger value="agents">
            <Bot />
            Agent
          </TabsTrigger>
        </TabsList>
        <div className="settings-content">
          <TabsContent value="appearance">
            <h2>外观</h2>
            <p className="muted">让工作空间更适合你的习惯。</p>
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
            <div className="appearance-note">
              <div>
                <span>配色</span>
                <strong>Stone 暖灰</strong>
              </div>
              <div>
                <span>字体</span>
                <strong>系统字体</strong>
              </div>
              <div>
                <span>图标</span>
                <strong>Lucide</strong>
              </div>
            </div>
            <p className="metadata">
              主题在本机记住；所有页面使用同一套语义 Token。
            </p>
          </TabsContent>
          <TabsContent value="agents">
            <div className="settings-section-header">
              <div>
                <h2>Agent</h2>
                <p className="muted">定义它如何工作，而不是重复配置模型。</p>
              </div>
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  const agent = {
                    id: crypto.randomUUID(),
                    name: "新 Agent",
                    prompt: "",
                    tools: ["读取文件"],
                  };
                  setAgents((items) => [...items, agent]);
                  edit(agent);
                }}
              >
                <Plus />
                新建
              </Button>
            </div>
            <div className="agent-chips">
              {agents.map((agent) => (
                <Button
                  key={agent.id}
                  variant={selected === agent.id ? "secondary" : "ghost"}
                  onClick={() => edit(agent)}
                >
                  {agent.name}
                </Button>
              ))}
            </div>
            <div className="agent-form">
              <Label htmlFor="agent-name">名称</Label>
              <Input
                id="agent-name"
                value={draft.name}
                onChange={(event) =>
                  setDraft({ ...draft, name: event.target.value })
                }
              />
              <Label htmlFor="agent-prompt">系统提示词</Label>
              <Textarea
                id="agent-prompt"
                rows={5}
                value={draft.prompt}
                onChange={(event) =>
                  setDraft({ ...draft, prompt: event.target.value })
                }
                placeholder="告诉 Agent 如何工作…"
              />
              <Collapsible className="tools-config">
                <CollapsibleTrigger>
                  <ChevronRight className="disclosure-chevron" />
                  高级配置 · 工具权限
                </CollapsibleTrigger>
                <CollapsibleContent>
                  {["读取文件", "编辑文件", "运行命令"].map((tool) => (
                    <div className="tool-permission" key={tool}>
                      <Label htmlFor={tool}>{tool}</Label>
                      <Switch
                        id={tool}
                        checked={draft.tools.includes(tool)}
                        onCheckedChange={(checked) =>
                          setDraft({
                            ...draft,
                            tools: checked
                              ? [...draft.tools, tool]
                              : draft.tools.filter((item) => item !== tool),
                          })
                        }
                      />
                    </div>
                  ))}
                </CollapsibleContent>
              </Collapsible>
              <div className="agent-form-footer">
                <Button
                  disabled={!draft.name.trim()}
                  onClick={() => {
                    setAgents((items) =>
                      items.map((item) =>
                        item.id === draft.id
                          ? { ...draft, name: draft.name.trim() }
                          : item,
                      ),
                    );
                    setSaved("已保存到原型内存，刷新后恢复示例。");
                  }}
                >
                  保存更改
                </Button>
                <AlertDialog>
                  <AlertDialogTrigger asChild>
                    <Button variant="ghost" disabled={cannotDelete}>
                      <Trash2 />
                      删除
                    </Button>
                  </AlertDialogTrigger>
                  <AlertDialogContent>
                    <AlertDialogHeader>
                      <AlertDialogTitle>删除 {draft.name}？</AlertDialogTitle>
                      <AlertDialogDescription>
                        仅删除当前原型中的 Agent，不影响真实设置。
                      </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                      <AlertDialogCancel>取消</AlertDialogCancel>
                      <AlertDialogAction
                        onClick={() => {
                          setAgents((items) =>
                            items.filter((item) => item.id !== draft.id),
                          );
                          edit(agents[0]);
                        }}
                      >
                        删除
                      </AlertDialogAction>
                    </AlertDialogFooter>
                  </AlertDialogContent>
                </AlertDialog>
              </div>
              {cannotDelete && (
                <p className="metadata">
                  {draft.id === "default"
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
          </TabsContent>
        </div>
      </Tabs>
    </section>
  );
}
