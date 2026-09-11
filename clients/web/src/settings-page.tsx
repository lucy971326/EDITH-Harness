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

export function SettingsPage({
  theme,
  setTheme,
  onBack,
}: {
  theme: string;
  setTheme: (theme: string) => void;
  onBack: () => void;
}) {
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
              <Button size="sm" variant="outline" disabled>
                <Plus />
                新建
              </Button>
            </div>
            <p className="metadata">后台尚未接入，无法加载或修改 Agent。</p>
            <div className="agent-form">
              <Label htmlFor="agent-name">名称</Label>
              <Input id="agent-name" value="" disabled placeholder="未加载" />
              <Label htmlFor="agent-prompt">系统提示词</Label>
              <Textarea
                id="agent-prompt"
                rows={5}
                value=""
                disabled
                placeholder="未加载"
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
                      <Switch id={tool} disabled />
                    </div>
                  ))}
                </CollapsibleContent>
              </Collapsible>
              <div className="agent-form-footer">
                <Button disabled>保存更改</Button>
                <Button variant="ghost" disabled>
                  <Trash2 />
                  删除
                </Button>
              </div>
            </div>
          </TabsContent>
        </div>
      </Tabs>
    </section>
  );
}
