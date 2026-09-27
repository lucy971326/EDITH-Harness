import { useEffect, useState } from "react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { ArrowDown, ArrowUp, ChevronRight, Folder, FolderOpen, Globe, Plus, RefreshCw, Trash2, Webhook } from "../icons";
import type { HookConfig, HookView } from "../../../contracts/appserver.ts";
import { RPCClient, formatRPCError } from "../client/rpc";
import type { SettingsDraftState } from "./types";

type Scope = "global" | "project";
type Draft = Omit<HookConfig, "args" | "tools"> & { argsText: string; toolsText: string };
type Editor = { flow: string; index: number | null; draft: Draft };
type Target =
  | { kind: "scope"; scope: Scope }
  | { kind: "add"; flow: string }
  | { kind: "edit"; flow: string; index: number }
  | { kind: "move"; index: number; other: number }
  | { kind: "close" | "reload" };

function toDraft(hook: HookConfig): Draft {
  return { ...hook, argsText: JSON.stringify(hook.args), toolsText: hook.tools.join("\n") };
}

function emptyDraft(flow: string): Draft {
  return { flow, name: "", enabled: true, command: "", argsText: "[]", toolsText: "", timeoutSeconds: 0 };
}

function parseDraft(draft: Draft): HookConfig {
  const args: unknown = JSON.parse(draft.argsText);
  if (!Array.isArray(args) || !args.every((arg) => typeof arg === "string")) {
    throw new Error("参数必须是字符串 JSON 数组，例如 [\"--help\"]");
  }
  if (!draft.name.trim() || !draft.command.trim()) {
    throw new Error("请填写名称和命令");
  }
  return {
    name: draft.name.trim(),
    flow: draft.flow,
    enabled: draft.enabled,
    command: draft.command,
    args,
    tools: draft.toolsText.split("\n").map((name) => name.trim()).filter(Boolean),
    timeoutSeconds: draft.timeoutSeconds,
  };
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
  const [editor, setEditor] = useState<Editor | null>(null);
  const [pendingTarget, setPendingTarget] = useState<Target | null>(null);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [reload, setReload] = useState(0);

  const source = view?.[scope];
  const outdatedBackend = source?.hooks.some((hook) => !hook.flow) ?? false;
  const projectReady = scope !== "project" || (!!workspace && workspaceText === workspace);
  const storedHook = editor?.index === null ? null : source?.hooks[editor?.index ?? -1];
  const dirty = !!editor && (editor.index === null || !storedHook ||
    JSON.stringify(editor.draft) !== JSON.stringify(toDraft(storedHook)));

  useEffect(() => { onStateChange({ dirty, saving }); }, [dirty, saving, onStateChange]);

  useEffect(() => {
    if (dirty) return;
    setWorkspace(currentWorkspace);
    setWorkspaceText(currentWorkspace);
  }, [currentWorkspace]);

  useEffect(() => {
    let active = true;
    setView(null);
    setEditor(null);
    setError("");
    setSaved(false);
    if (!client?.connected) return;
    void client.call("hooks/read", { workspace }).then((result) => {
      if (active) setView(result);
    }).catch((cause: unknown) => {
      if (active) setError(formatRPCError(cause, "Hook 设置加载失败"));
    });
    return () => { active = false; };
  }, [client, workspace, scope, reload]);

  function protectDraft(): boolean {
    if (!dirty) return false;
    setError("先保存或放弃当前 Hook 的更改。");
    return true;
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

  async function persist(hooks: HookConfig[], current: HookView | null = view): Promise<HookView | null> {
    if (!client?.connected || !current || saving || !projectReady ||
      (!!current[scope].error && !current[scope].hash)) return null;
    setSaving(true);
    setError("");
    setSaved(false);
    try {
      const result = await client.call("hooks/save", {
        scope, workspace, hash: current[scope].hash, hooks,
      });
      setView(result);
      setEditor(null);
      setSaved(true);
      return result;
    } catch (cause) {
      setError(formatRPCError(cause, "保存失败，请重新加载配置"));
      return null;
    } finally {
      setSaving(false);
    }
  }

  async function submitEditor(current: HookView | null = view): Promise<HookView | null> {
    if (!editor || !current) return null;
    let hook: HookConfig;
    try {
      hook = parseDraft(editor.draft);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Hook 格式错误");
      return null;
    }
    const hooks = [...current[scope].hooks];
    if (editor.index === null) hooks.push(hook);
    else if (hooks[editor.index]) hooks[editor.index] = hook;
    else return null;
    return persist(hooks, current);
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

  function showTarget(target: Target, current: HookView) {
    setError("");
    setSaved(false);
    switch (target.kind) {
      case "scope":
        setEditor(null);
        setScope(target.scope);
        break;
      case "add":
        setEditor({ flow: target.flow, index: null, draft: emptyDraft(target.flow) });
        break;
      case "edit": {
        const hook = current[scope].hooks[target.index];
        if (hook) setEditor({ flow: target.flow, index: target.index, draft: toDraft(hook) });
        break;
      }
      case "close":
        setEditor(null);
        break;
      case "reload":
        setEditor(null);
        setReload((value) => value + 1);
        break;
      case "move": {
        const hooks = [...current[scope].hooks];
        [hooks[target.index], hooks[target.other]] = [hooks[target.other], hooks[target.index]];
        setEditor(null);
        void persist(hooks, current);
        break;
      }
    }
  }

  function requestTarget(target: Target) {
    if (!view || saving) return;
    if (dirty) { setPendingTarget(target); return; }
    showTarget(target, view);
  }

  async function saveBeforeSwitch() {
    const target = pendingTarget;
    setPendingTarget(null);
    const result = await submitEditor();
    if (result && target) showTarget(target, result);
  }

  function discardBeforeSwitch() {
    if (pendingTarget && view) showTarget(pendingTarget, view);
    setPendingTarget(null);
  }

  function renderEditor() {
    if (!editor) return null;
    const id = editor.index === null ? "hook-new" : "hook-" + editor.index;
    const draft = editor.draft;
    const update = (change: Partial<Draft>) => {
      setEditor({ ...editor, draft: { ...draft, ...change } });
      setError("");
      setSaved(false);
    };
    return <form className="agent-form hook-form" onSubmit={(event) => {
      event.preventDefault();
      void submitEditor();
    }}>
      <div className="settings-actions">
        <Switch id={id + "-enabled"} checked={draft.enabled} disabled={saving}
          onCheckedChange={(enabled) => update({ enabled })} />
        <Label htmlFor={id + "-enabled"}>{draft.enabled ? "已启用" : "已停用"}</Label>
      </div>
      <div className="settings-field"><Label htmlFor={id + "-name"}>名称</Label>
        <Input id={id + "-name"} value={draft.name} disabled={saving}
          onChange={(event) => update({ name: event.target.value })} /></div>
      <div className="settings-field"><Label htmlFor={id + "-command"}>命令</Label>
        <Input id={id + "-command"} value={draft.command} disabled={saving} placeholder="/usr/bin/python3"
          onChange={(event) => update({ command: event.target.value })} /></div>
      <div className="settings-field"><Label htmlFor={id + "-args"}>参数（JSON 字符串数组）</Label>
        <Textarea id={id + "-args"} rows={2} value={draft.argsText} disabled={saving}
          onChange={(event) => update({ argsText: event.target.value })} /></div>
      <Collapsible className="settings-advanced" key={id}>
        <CollapsibleTrigger className="settings-advanced-trigger ui-focus"><ChevronRight className="disclosure-chevron" />高级设置</CollapsibleTrigger>
        <CollapsibleContent className="settings-advanced-content">
          <div className="settings-field"><Label htmlFor={id + "-tools"}>匹配工具</Label>
            <Textarea id={id + "-tools"} rows={2} value={draft.toolsText} disabled={saving} placeholder="每行一个，留空匹配全部"
              onChange={(event) => update({ toolsText: event.target.value })} /></div>
          <div className="settings-field"><Label htmlFor={id + "-timeout"}>超时秒数</Label>
            <Input id={id + "-timeout"} type="number" min={0} max={60}
              value={draft.timeoutSeconds} disabled={saving}
              onChange={(event) => update({ timeoutSeconds: Number(event.target.value) })} />
            <span className="settings-description">0 使用默认 10 秒</span></div>
        </CollapsibleContent>
      </Collapsible>
      <div className="hook-form-footer">
        {editor.index !== null && source && <Button type="button" size="sm" variant="ghost"
          className="settings-delete" disabled={saving || !projectReady || (!!source.error && !source.hash)}
          onClick={() => void persist(source.hooks.filter((_, index) => index !== editor.index))}>
          <Trash2 />删除 Hook
        </Button>}
        <Button type="button" size="sm" variant="ghost" disabled={saving}
          onClick={() => setEditor(null)}>放弃</Button>
        <Button type="submit" size="sm" disabled={saving || !dirty || !draft.name.trim() ||
          !draft.command.trim() || !client?.connected || (!!source?.error && !source.hash) ||
          !projectReady}>
          {saving ? "保存中…" : editor.index === null ? "添加 Hook" : "保存"}
        </Button>
      </div>
    </form>;
  }

  return <>
    <header className="settings-heading"><h2>Hooks</h2></header>
    <div className="settings-two-pane">
      <aside className="settings-subnav" aria-label="Hook 生效范围">
        <h3>生效范围</h3>
        <div className="settings-subnav-list">
          {(["global", "project"] as const).map((item) => <Button key={item}
            variant="ghost" className="settings-subnav-item" aria-pressed={scope === item} disabled={saving}
            onClick={() => requestTarget({ kind: "scope", scope: item })}>
            {item === "global" ? <Globe /> : <Folder />}
            <span className="settings-subnav-copy">
              <span className="settings-subnav-name">{item === "global" ? "全局" : "项目"}</span>
              {view && <span className="settings-subnav-meta">{view[item].hooks.length} 个 Hook</span>}
            </span>
          </Button>)}
        </div>
      </aside>
      <div className="settings-detail-pane">
        <div className="settings-detail-title-row">
          <div className="settings-identity"><span className="settings-identity-icon"><Webhook /></span><h3>{scope === "global" ? "全局 Hooks" : "项目 Hooks"}</h3></div>
          <Button size="icon-sm" variant="ghost" aria-label="重新加载 Hook 配置"
            title="重新加载" disabled={saving || !client?.connected}
            onClick={() => requestTarget({ kind: "reload" })}><RefreshCw /></Button>
        </div>
        {scope === "project" && <div className="settings-field hook-workspace">
          <Label htmlFor="hook-workspace">工作区</Label>
          <Input id="hook-workspace" value={workspaceText} disabled={saving}
            placeholder="选择或输入项目目录" onChange={(event) => setWorkspaceText(event.target.value)} />
          <div className="settings-actions">
            <Button size="sm" variant="outline" disabled={saving} onClick={() => {
              if (protectDraft()) return;
              if (workspace === workspaceText) setReload((value) => value + 1);
              else setWorkspace(workspaceText);
            }}>加载</Button>
            <Button size="sm" variant="outline" disabled={saving}
              onClick={() => void pickWorkspace()}><FolderOpen />选择文件夹</Button>
          </div>
          {!projectReady && <span className="settings-description">先加载工作区，再编辑项目 Hook。</span>}
        </div>}
        {!client?.connected && <p className="inline-notice">连接后台后可修改。</p>}
        {error && <div className="inline-notice" role="alert">{error}
          {!view && <Button size="sm" variant="ghost" disabled={saving || !client?.connected}
            onClick={() => setReload((value) => value + 1)}>重新加载</Button>}
        </div>}
        {!view && client?.connected && !error && <p className="metadata">正在加载…</p>}
        {view && !view.flows?.length && <p className="inline-notice" role="alert">后台未提供 Hook 点位。</p>}
        {outdatedBackend && <p className="inline-notice" role="alert">请重启后台以更新 Hook 数据。</p>}
        {view && source && !!view.flows?.length && !outdatedBackend && <>
          {source.error && <p className="inline-notice" role="alert">配置读取失败：{source.error}</p>}
          {scope === "project" && !!source.hash && !view.trusted && <>
            <p className="inline-notice">项目配置已改变，项目 Hook 已暂停。</p>
            <Button size="sm" variant="outline" disabled={saving || dirty || !projectReady || !!source.error}
              onClick={() => void trust()}>信任当前项目配置</Button>
          </>}
          <div className="hook-flows">
            {view.flows.map((flow) => {
              const items = source.hooks.map((hook, index) => ({ hook, index }))
                .filter((entry) => entry.hook.flow === flow);
              return <section className="hook-flow" key={flow}>
                <div className="hook-flow-header">
                  <h4>{flow} <span className="settings-badge">{items.length}</span></h4>
                  <Button size="icon-sm" variant="outline" aria-label={"在 " + flow + " 添加 Hook"}
                    title="添加 Hook" disabled={saving || !client?.connected ||
                      (!!source.error && !source.hash) || !projectReady}
                    onClick={() => requestTarget({ kind: "add", flow })}><Plus /></Button>
                </div>
                {editor?.flow === flow && editor.index === null && renderEditor()}
                {items.length === 0 && editor?.flow !== flow &&
                  <p className="metadata hook-flow-empty">暂无 Hook</p>}
                <div className="hook-list">
                  {items.map(({ hook, index }, position) => {
                    const open = editor?.flow === flow && editor.index === index;
                    return <div className="hook-item" key={hook.name + index}>
                      <div className="hook-summary">
                        <button type="button" className="hook-summary-main" aria-expanded={open}
                          disabled={!projectReady}
                          onClick={() => requestTarget(open ? { kind: "close" } : { kind: "edit", flow, index })}>
                          <ChevronRight className={open ? "disclosure-chevron open" : "disclosure-chevron"} />
                          <span className="hook-summary-name">{hook.name}</span>
                          <span className="metadata">{hook.enabled ? "已启用" : "已停用"}</span>
                        </button>
                        <div className="hook-summary-actions">
                          <Button size="icon-sm" variant="ghost" title="上移"
                            aria-label={"上移 " + hook.name} disabled={saving || !projectReady || position === 0}
                            onClick={() => requestTarget({ kind: "move", index, other: items[position - 1].index })}>
                            <ArrowUp />
                          </Button>
                          <Button size="icon-sm" variant="ghost" title="下移"
                            aria-label={"下移 " + hook.name} disabled={saving || !projectReady || position === items.length - 1}
                            onClick={() => requestTarget({ kind: "move", index, other: items[position + 1].index })}>
                            <ArrowDown />
                          </Button>
                        </div>
                      </div>
                      {open && renderEditor()}
                    </div>;
                  })}
                </div>
              </section>;
            })}
          </div>
          {saved && <p className="settings-save-status" role="status">已保存</p>}
          {view.lastError && <p className="inline-notice" role="status">最近一次 Hook 问题：{view.lastError}</p>}
        </>}
      </div>
    </div>
    <AlertDialog open={pendingTarget !== null}
      onOpenChange={(open) => { if (!open) setPendingTarget(null); }}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>保存当前 Hook 的更改？</AlertDialogTitle>
          <AlertDialogDescription>切换后，未保存的内容会丢失。</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>继续编辑</AlertDialogCancel>
          <AlertDialogAction onClick={discardBeforeSwitch}>放弃更改</AlertDialogAction>
          <AlertDialogAction disabled={!editor?.draft.name.trim() || !editor.draft.command.trim() || saving}
            onClick={() => void saveBeforeSwitch()}>保存并切换</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </>;
}
