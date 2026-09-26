import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { ChevronRight, Command, Plus, RefreshCw } from "../icons";
import type { HookConfig, HookView } from "../../../contracts/appserver.ts";
import { RPCClient, formatRPCError } from "../client/rpc";
import type { SettingsDraftState } from "./types";

type Scope = "global" | "project";
type Draft = Omit<HookConfig, "args" | "tools"> & { argsText: string; toolsText: string };

function toDraft(hook: HookConfig): Draft {
  return { ...hook, argsText: JSON.stringify(hook.args), toolsText: hook.tools.join("\n") };
}

export function HookSettingsPanel({ client, currentWorkspace, onStateChange }: {
  client: RPCClient | null;
  currentWorkspace: string;
  onStateChange: (state: SettingsDraftState) => void;
}) {
  const [scope, setScope] = useState<Scope>("global");
  const [workspace, setWorkspace] = useState(currentWorkspace);
  const [workspaceText, setWorkspaceText] = useState(currentWorkspace);
  const [view, setView] = useState<HookView | null>(null);
  const [draft, setDraft] = useState<Draft[]>([]);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [reload, setReload] = useState(0);
  const [openIndex, setOpenIndex] = useState<number | null>(null);

  useEffect(() => { onStateChange({ dirty, saving }); }, [dirty, saving, onStateChange]);

  useEffect(() => {
    if (dirty) return;
    setWorkspace(currentWorkspace);
    setWorkspaceText(currentWorkspace);
  }, [currentWorkspace]);

  useEffect(() => {
    let active = true;
    setView(null);
    setError("");
    setSaved(false);
    setOpenIndex(null);
    if (!client?.connected) return;
    void client.call("hooks/read", { workspace }).then((result) => {
      if (!active) return;
      setView(result);
      setDraft(result[scope].hooks.map(toDraft));
      setDirty(false);
    }).catch((cause: unknown) => {
      if (active) setError(formatRPCError(cause, "Hook 设置加载失败"));
    });
    return () => { active = false; };
  }, [client, workspace, scope, reload]);

  function changed() {
    setDirty(true);
    setSaved(false);
    setError("");
  }

  function protectDraft(): boolean {
    if (!dirty) return false;
    setError("有未保存的 Hook 修改，请先保存或重新加载。");
    return true;
  }

  function edit(index: number, change: Partial<Draft>) {
    setDraft((items) => items.map((item, i) => i === index ? { ...item, ...change } : item));
    changed();
  }

  async function pickWorkspace() {
    if (!client?.connected || protectDraft()) return;
    try {
      const picked = await client.call("workspace/select", {});
      if (!picked.canceled && picked.workspace) {
        setWorkspace(picked.workspace);
        setWorkspaceText(picked.workspace);
      }
    } catch (cause) {
      setError(formatRPCError(cause, "选择工作区失败"));
    }
  }

  async function save() {
    if (!client?.connected || !view || saving) return;
    let hooks: HookConfig[];
    try {
      hooks = draft.map(({ argsText, toolsText, ...item }) => {
        const args: unknown = JSON.parse(argsText);
        if (!Array.isArray(args) || !args.every((arg) => typeof arg === "string")) {
          throw new Error("参数必须是字符串 JSON 数组，例如 [\"--help\"]");
        }
        return { ...item, args, tools: toolsText.split("\n").map((name) => name.trim()).filter(Boolean) };
      });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "参数格式错误");
      return;
    }
    setSaving(true);
    setError("");
    setSaved(false);
    try {
      const result = await client.call("hooks/save", {
        scope, workspace, hash: view[scope].hash, hooks,
      });
      setView(result);
      setDraft(result[scope].hooks.map(toDraft));
      setDirty(false);
      setSaved(true);
    } catch (cause) {
      setError(formatRPCError(cause, "保存失败，请重新加载配置"));
    } finally {
      setSaving(false);
    }
  }

  async function trust() {
    if (!client?.connected || !view?.project.hash || saving) return;
    setSaving(true);
    setError("");
    try {
      const result = await client.call("hooks/trust", { workspace, hash: view.project.hash });
      setView(result);
      setSaved(false);
    } catch (cause) {
      setError(formatRPCError(cause, "确认失败，请重新加载配置"));
    } finally {
      setSaving(false);
    }
  }

  const source = view?.[scope];
  function addHook() {
    setDraft((items) => [...items, {
      name: "", enabled: true, command: "", argsText: "[]", toolsText: "", timeoutSeconds: 0,
    }]);
    setOpenIndex(draft.length);
    changed();
  }

  return <>
    <header className="settings-heading"><h2>Hooks</h2><p>在工具调用前执行脚本，用于检查、记录或拦截操作。</p></header>
    <section className="settings-section">
    <div className="settings-section-header"><div><h3>生效范围</h3><p>{scope === "global" ? "全局配置对所有项目生效。" : "项目配置只对指定工作区生效，使用前需要信任。"}</p></div></div>
    <div className="settings-segmented" role="group" aria-label="Hook 来源">
      {(["global", "project"] as const).map((item) => <Button key={item}
        variant="ghost" aria-pressed={scope === item} size="sm" disabled={saving}
        onClick={() => { if (item !== scope && !protectDraft()) setScope(item); }}>
        {item === "global" ? "全局" : "项目"}
      </Button>)}
    </div>
    {scope === "project" && <div className="settings-field hook-workspace">
      <Label htmlFor="hook-workspace">工作区</Label>
      <Input id="hook-workspace" value={workspaceText} disabled={saving} placeholder="选择或输入项目目录" onChange={(event) => setWorkspaceText(event.target.value)} />
      <div className="settings-actions">
        <Button size="sm" variant="outline" disabled={saving} onClick={() => {
          if (protectDraft()) return;
          if (workspace === workspaceText) setReload((value) => value + 1);
          else setWorkspace(workspaceText);
        }}>加载</Button>
        <Button size="sm" variant="outline" disabled={saving} onClick={() => void pickWorkspace()}>选择文件夹</Button>
      </div>
    </div>}
    </section>
    {!client?.connected && <p className="inline-notice">连接后台后可修改。</p>}
    {error && <div className="inline-notice" role="alert">{error}
      {!view && <Button size="sm" variant="ghost" disabled={saving || !client?.connected} onClick={() => setReload((value) => value + 1)}>重新加载</Button>}
    </div>}
    {!view && client?.connected && !error && <p className="metadata">正在加载…</p>}
    {view && source && <>
      {source.error && <p className="inline-notice" role="alert">配置读取失败：{source.error}</p>}
      {scope === "project" && !!source.hash && !view.trusted && <>
        <p className="inline-notice">项目配置已改变，项目 Hook 已暂停。</p>
        <Button size="sm" variant="outline" disabled={saving || dirty || !!source.error} onClick={() => void trust()}>信任当前项目配置</Button>
      </>}
      <section className="settings-section">
      <div className="settings-section-header"><div><h3>工具调用前 <span className="settings-badge">{draft.length}</span></h3><p>多个 Hook 按列表顺序执行。</p></div>
        {draft.length > 0 && <Button size="sm" variant="outline" disabled={saving || !client?.connected} onClick={addHook}><Plus />新增 Hook</Button>}
      </div>
      {draft.length === 0 && <div className="settings-empty">
        <Command /><h3>还没有{scope === "global" ? "全局" : "项目"} Hook</h3>
        <p>添加脚本，让工具执行遵循你的工作流程。</p>
        <Button size="sm" variant="outline" disabled={saving || !client?.connected || (!!source.error && !source.hash) || (scope === "project" && !workspace)} onClick={addHook}><Plus />添加第一个 Hook</Button>
      </div>}
      <div className="hook-list">
        {draft.map((hook, index) => <Collapsible
          key={index}
          className="hook-item"
          open={openIndex === index}
          onOpenChange={(open) => setOpenIndex(open ? index : null)}
        >
          <CollapsibleTrigger className="hook-summary">
            <ChevronRight className="disclosure-chevron" />
            <span className="hook-summary-name">{hook.name.trim() || `未命名 Hook ${index + 1}`}</span>
            <span className="metadata">{hook.enabled ? "已启用" : "已停用"}</span>
          </CollapsibleTrigger>
          <CollapsibleContent className="agent-form hook-form">
            <div className="settings-actions">
              <Switch
                aria-label={`${hook.name || `Hook ${index + 1}`}启用状态`}
                checked={hook.enabled}
                disabled={saving}
                onCheckedChange={(enabled) => edit(index, { enabled })}
              />
              <span>{hook.enabled ? "已启用" : "已停用"}</span>
              <Button size="sm" variant="outline" disabled={saving || index === 0} onClick={() => {
                setDraft((items) => {
                  const next = [...items];
                  [next[index - 1], next[index]] = [next[index], next[index - 1]];
                  return next;
                });
                setOpenIndex(index - 1);
                changed();
              }}>上移</Button>
              <Button size="sm" variant="outline" disabled={saving || index === draft.length - 1} onClick={() => {
                setDraft((items) => {
                  const next = [...items];
                  [next[index], next[index + 1]] = [next[index + 1], next[index]];
                  return next;
                });
                setOpenIndex(index + 1);
                changed();
              }}>下移</Button>
              <Button size="sm" variant="ghost" disabled={saving} onClick={() => {
                setDraft((items) => items.filter((_, i) => i !== index));
                setOpenIndex(null);
                changed();
              }}>删除</Button>
            </div>
            <Label htmlFor={`hook-name-${index}`}>名称</Label>
            <Input id={`hook-name-${index}`} value={hook.name} disabled={saving} onChange={(event) => edit(index, { name: event.target.value })} />
            <Label htmlFor={`hook-command-${index}`}>命令</Label>
            <Input id={`hook-command-${index}`} value={hook.command} disabled={saving} placeholder="/usr/bin/python3" onChange={(event) => edit(index, { command: event.target.value })} />
            <Label htmlFor={`hook-args-${index}`}>参数（JSON 字符串数组）</Label>
            <Textarea id={`hook-args-${index}`} rows={2} value={hook.argsText} disabled={saving} onChange={(event) => edit(index, { argsText: event.target.value })} />
            <Label htmlFor={`hook-tools-${index}`}>匹配工具（每行一个，留空匹配全部）</Label>
            <Textarea id={`hook-tools-${index}`} rows={2} value={hook.toolsText} disabled={saving} onChange={(event) => edit(index, { toolsText: event.target.value })} />
            <Label htmlFor={`hook-timeout-${index}`}>超时秒数（0 使用默认 10 秒）</Label>
            <Input id={`hook-timeout-${index}`} type="number" min={0} max={60} value={hook.timeoutSeconds} disabled={saving} onChange={(event) => edit(index, { timeoutSeconds: Number(event.target.value) })} />
          </CollapsibleContent>
        </Collapsible>)}
      </div>
      </section>
      <div className="settings-savebar">
        <span className="settings-save-status" role="status">{dirty ? "有未保存的更改" : source.error ? "配置读取失败" : saved ? "已保存" : "所有更改已保存"}</span>
        <Button variant="ghost" disabled={saving || !client?.connected} onClick={() => setReload((value) => value + 1)}><RefreshCw />{dirty ? "放弃更改" : "重新加载"}</Button>
        <Button disabled={!dirty || saving || !client?.connected || (!!source.error && !source.hash) || (scope === "project" && !workspace)} onClick={() => void save()}>
          {saving ? "保存中…" : "保存更改"}
        </Button>
      </div>
      {view.lastError && <p className="inline-notice" role="status">最近一次 Hook 问题：{view.lastError}</p>}
    </>}
  </>;
}
