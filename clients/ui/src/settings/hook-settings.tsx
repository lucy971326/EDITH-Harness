import { useEffect, useState } from "react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Button } from "@/components/ui/button";
import { Hint } from "@/components/ui/tooltip";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { ArrowDown, ArrowUp, ChevronRight, Plus, RefreshCw, Trash2, Webhook } from "../icons";
import type { HookConfig, HookView } from "../../../contracts/appserver.ts";
import { RPCClient, formatRPCError } from "../client/rpc";
import type { SettingsDraftState } from "./types";
import { SettingsHeader, SettingsListToolbar, SettingsResourceRow, SettingsScope, SettingsEmpty } from "./settings-primitives";

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
  const [view, setView] = useState<HookView | null>(null);
  const [editor, setEditor] = useState<Editor | null>(null);
  const [pendingTarget, setPendingTarget] = useState<Target | null>(null);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [reload, setReload] = useState(0);
  const [query, setQuery] = useState("");
  const [deleteOpen, setDeleteOpen] = useState(false);

  const source = view?.[scope];
  const outdatedBackend = source?.hooks.some((hook) => !hook.flow) ?? false;
  const projectReady = scope !== "project" || !!workspace;
  const storedHook = editor?.index === null ? null : source?.hooks[editor?.index ?? -1];
  const dirty = !!editor && (editor.index === null || !storedHook ||
    JSON.stringify(editor.draft) !== JSON.stringify(toDraft(storedHook)));

  useEffect(() => { onStateChange({ dirty, saving }); }, [dirty, saving, onStateChange]);

  useEffect(() => {
    if (dirty) return;
    setWorkspace(currentWorkspace);
  }, [currentWorkspace]);

  useEffect(() => {
    let active = true;
    if (editor) return;
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
    return <form className="settings-form-card" onSubmit={(event) => {
      event.preventDefault();
      void submitEditor();
    }}>
      <div className="settings-field"><Label htmlFor={id + "-flow"}>触发时机</Label>
        <Select value={draft.flow} disabled={saving || editor.index !== null} onValueChange={(flow) =>
          setEditor({ ...editor, flow, draft: { ...draft, flow } })}>
          <SelectTrigger id={id + "-flow"}><SelectValue /></SelectTrigger>
          <SelectContent>{view?.flows.map((flow) => <SelectItem key={flow} value={flow}>{flow}</SelectItem>)}</SelectContent>
        </Select>
      </div>
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
      <div className="settings-form-actions">
        {editor.index !== null && source && <Button type="button" size="sm" variant="ghost"
          className="settings-delete" disabled={saving || !projectReady || (!!source.error && !source.hash)}
          onClick={() => setDeleteOpen(true)}>
          <Trash2 />删除 Hook
        </Button>}
        <Button type="button" size="sm" variant="ghost" disabled={saving}
          onClick={() => requestTarget({ kind: "close" })}>取消</Button>
        <Button type="submit" size="sm" disabled={saving || !dirty || !draft.name.trim() ||
          !draft.command.trim() || !client?.connected || (!!source?.error && !source.hash) ||
          !projectReady}>
          {saving ? "保存中…" : "保存"}
        </Button>
      </div>
    </form>;
  }

  return <>
    <SettingsHeader title={editor ? editor.index === null ? "新建 Hook" : storedHook?.name ?? "编辑 Hook" : "Hooks"}
      description={editor ? `${scope === "global" ? "用户" : "工作区"} Hook` : "在指定时机自动执行命令，按列表顺序运行。"}
      back={editor ? { label: "Hooks", onClick: () => requestTarget({ kind: "close" }), disabled: saving } : undefined}
      actions={!editor && <DropdownMenu><DropdownMenuTrigger asChild>
        <Button size="sm" disabled={saving || !client?.connected || !view?.flows?.length || !projectReady || (!!source?.error && !source.hash)}><Plus />新建 Hook</Button>
      </DropdownMenuTrigger><DropdownMenuContent align="end">
        {view?.flows?.map((flow) => <DropdownMenuItem key={flow} onSelect={() => requestTarget({ kind: "add", flow })}>{flow}</DropdownMenuItem>)}
      </DropdownMenuContent></DropdownMenu>} />
    {!editor && <SettingsListToolbar query={query} onQueryChange={setQuery} placeholder="搜索 Hook"
      actions={<Hint text="刷新 Hooks"><Button size="icon-sm" variant="ghost" aria-label="刷新 Hooks" disabled={saving || !client?.connected}
        onClick={() => requestTarget({ kind: "reload" })}><RefreshCw /></Button></Hint>}>
      <SettingsScope value={scope === "global" ? "user" : "workspace"} workspace={workspace} disabled={saving || !client?.connected}
        onChange={(value) => requestTarget({ kind: "scope", scope: value === "user" ? "global" : "project" })}
        onChooseWorkspace={() => void pickWorkspace()} />
    </SettingsListToolbar>}
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
          {editor ? renderEditor() : <>
            {view.flows.map((flow) => {
              const items = source.hooks.map((hook, index) => ({ hook, index }))
                .filter((entry) => entry.hook.flow === flow);
              const visible = items.filter(({ hook }) => `${hook.name} ${hook.command} ${flow}`.toLowerCase().includes(query.toLowerCase()));
              if (!visible.length) return null;
              return <section className="settings-resource-group" key={flow}>
                <h3>{flow}<span>{visible.length}</span></h3>
                <div className="settings-resource-list">
                  {visible.map(({ hook, index }) => {
                    const position = items.findIndex((item) => item.index === index);
                    return <div className="settings-resource-with-actions" key={hook.name + index}>
                        <SettingsResourceRow icon={<Webhook />} name={hook.name} description={hook.command}
                          meta={hook.enabled ? "已启用" : "已停用"} disabled={saving || !projectReady}
                          onClick={() => requestTarget({ kind: "edit", flow, index })} />
                        <div className="settings-row-actions">
                          <Hint text="上移"><Button size="icon-sm" variant="ghost"
                            aria-label={"上移 " + hook.name} disabled={saving || !projectReady || position === 0}
                            onClick={() => requestTarget({ kind: "move", index, other: items[position - 1].index })}>
                            <ArrowUp />
                          </Button></Hint>
                          <Hint text="下移"><Button size="icon-sm" variant="ghost"
                            aria-label={"下移 " + hook.name} disabled={saving || !projectReady || position === items.length - 1}
                            onClick={() => requestTarget({ kind: "move", index, other: items[position + 1].index })}>
                            <ArrowDown />
                          </Button></Hint>
                        </div>
                    </div>;
                  })}
                </div>
              </section>;
            })}
            {!source.hooks.some((hook) => `${hook.name} ${hook.command} ${hook.flow}`.toLowerCase().includes(query.toLowerCase())) &&
              <SettingsEmpty>{!projectReady ? "选择工作区后管理 Hooks。" : query ? "没有匹配的 Hook" : "还没有 Hook，点击新建选择触发时机。"}</SettingsEmpty>}
          </>}
          {saved && <p className="settings-save-status" role="status">已保存</p>}
          {view.lastError && <p className="inline-notice" role="status">最近一次 Hook 问题：{view.lastError}</p>}
        </>}
    <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}><AlertDialogContent>
      <AlertDialogHeader><AlertDialogTitle>删除 {storedHook?.name}？</AlertDialogTitle>
        <AlertDialogDescription>删除后，这条 Hook 将不再执行。</AlertDialogDescription></AlertDialogHeader>
      {error && <p className="inline-notice" role="alert">{error}</p>}
      <AlertDialogFooter><AlertDialogCancel disabled={saving}>取消</AlertDialogCancel>
        <Button variant="destructive" disabled={saving} onClick={() => {
          if (source && editor) void persist(source.hooks.filter((_, index) => index !== editor.index)).then((result) => { if (result) setDeleteOpen(false); });
        }}>删除</Button></AlertDialogFooter>
    </AlertDialogContent></AlertDialog>
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
