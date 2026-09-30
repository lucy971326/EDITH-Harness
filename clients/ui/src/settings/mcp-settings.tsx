import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Hint } from "@/components/ui/tooltip";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import type { MCPServerView, MCPSettingsView, MCPSaveInput, MCPOAuthTaskView } from "../../../contracts/appserver";
import { RPCClient, formatRPCError } from "../client/rpc";
import { ArrowLeft, ChevronRight, ExternalLink, FileText, KeyRound, LogOut, Plus, RefreshCw, Pencil, Server, Trash2, Wrench, X } from "../icons";
import type { SettingsDraftState } from "./types";

type Scope = "global" | "project";
type Target = { scope: Scope; name: string; edit: boolean; create: boolean };
type SecretField = "command" | "url" | "cwd";
type Draft = {
  name: string; type: "stdio" | "http";
  command: string; url: string; cwd: string;
  clear: Record<SecretField, boolean>;
  args: string[]; replaceArgs: boolean;
  env: Record<string, string | null>;
  headers: Record<string, string | null>;
  oauthClientId: string; oauthClientIdMetadataUrl: string; oauthClientSecretEnv: string; oauthRedirectUrl: string;
  clearOAuthClientSecretEnv: boolean;
  includeTools: string; excludeTools: string;
};

function newDraft(item?: MCPServerView): Draft {
  return {
    name: item?.name ?? "", type: item?.type === "http" || item?.type === "streamable-http" ? "http" : "stdio",
    command: "", url: "", cwd: "", clear: { command: false, url: false, cwd: false },
    args: [], replaceArgs: !item, env: {}, headers: {},
    oauthClientId: item?.oauthClientId ?? "", oauthClientIdMetadataUrl: item?.oauthClientIdMetadataUrl ?? "",
    oauthClientSecretEnv: "", oauthRedirectUrl: item?.oauthRedirectUrl ?? "",
    clearOAuthClientSecretEnv: false,
    includeTools: item?.includeTools.join(", ") ?? "", excludeTools: item?.excludeTools.join(", ") ?? "",
  };
}

function list(text: string): string[] {
  return text.split(",").map((part) => part.trim()).filter(Boolean);
}

function statusLabel(item: MCPServerView): string {
  if (item.overridden) return "被项目配置覆盖";
  switch (item.status) {
    case "connected": return `已连接 · ${item.tools.length} 个工具`;
    case "failed": return "连接失败";
    case "invalid": return "配置无效";
    case "auth-required": return "需要登录";
    case "insufficient-scope": return "需要增加授权权限";
    case "disabled": return "已关闭";
    default: return "配置已保存 · 尚未连接";
  }
}

export function MCPSettingsPanel({ client, currentWorkspace, onOpenFile, onStateChange, openExternal }: {
  client: RPCClient | null;
  currentWorkspace: string;
  onOpenFile: (path: string) => void;
  onStateChange: (state: SettingsDraftState) => void;
  openExternal: (url: string) => Promise<void>;
}) {
  const [view, setView] = useState<MCPSettingsView | null>(null);
  const [target, setTarget] = useState<Target>({ scope: "global", name: "", edit: false, create: false });
  const [draft, setDraft] = useState<Draft | null>(null);
  const [pending, setPending] = useState<Target | null>(null);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [reload, setReload] = useState(0);
  const [resetPrompt, setResetPrompt] = useState(false);
  const [newEnvKey, setNewEnvKey] = useState("");
  const [newHeaderKey, setNewHeaderKey] = useState("");
  const [authTask, setAuthTask] = useState<MCPOAuthTaskView | null>(null);
  const [authTaskTarget, setAuthTaskTarget] = useState<{ scope: Scope; name: string; workspace: string } | null>(null);
  const [trustPrompt, setTrustPrompt] = useState(false);

  const current = target.scope === "project" ? view?.project : view?.global;
  const selected = current?.find((item) => item.name === target.name);
  const selectedAuthTask = selected && authTaskTarget?.workspace === currentWorkspace &&
    authTaskTarget.scope === target.scope && authTaskTarget.name === selected.name ? authTask : null;
  const authBusy = selectedAuthTask != null && ["preparing", "waiting", "connecting"].includes(selectedAuthTask.state);
  const authNeeded = selected?.status === "auth-required" || selected?.status === "insufficient-scope";
  const showOAuth = selected && selected.type !== "stdio" && !selected.overridden &&
    (authNeeded || selected.hasOAuthCredentials || selectedAuthTask != null);
  const dirty = !!draft && (target.create || JSON.stringify(draft) !== JSON.stringify(newDraft(selected)) ||
    !!newEnvKey.trim() || !!newHeaderKey.trim());
  useEffect(() => { onStateChange({ dirty, saving }); }, [dirty, saving, onStateChange]);

  useEffect(() => {
    let active = true;
    if (!client?.connected) return;
    void client.call("mcp/read", { workspace: currentWorkspace }).then((result) => {
      if (!active) return;
      setView(result);
      setTarget((old) => {
        const items = old.scope === "project" ? result.project : result.global;
        return items.some((item) => item.name === old.name) || old.create ? old :
          { ...old, name: items[0]?.name ?? "", edit: false, create: false };
      });
    }).catch((cause: unknown) => { if (active) setError(formatRPCError(cause, "MCP 配置读取失败")); });
    return () => { active = false; };
  }, [client, currentWorkspace, reload]);

  useEffect(() => {
    if (!client?.connected || !authTask || !["preparing", "waiting", "connecting"].includes(authTask.state)) return;
    let active = true;
    const timer = window.setInterval(() => {
      void client.call("mcp/auth/status", { id: authTask.id }).then(async (next) => {
        if (!active) return;
        setAuthTask(next);
        if (["complete", "failed", "cancelled"].includes(next.state)) await refreshDisk();
      }).catch((cause: unknown) => { if (active) setError(formatRPCError(cause, "授权状态读取失败")); });
    }, 1000);
    return () => { active = false; window.clearInterval(timer); };
  }, [client, authTask?.id, authTask?.state, currentWorkspace]);

  function select(next: Target) {
    if (saving) return;
    if (dirty) { setPending(next); return; }
    show(next);
  }

  function show(next: Target) {
    const item = (next.scope === "project" ? view?.project : view?.global)?.find((entry) => entry.name === next.name);
    setTarget(next);
    setDraft(next.edit ? newDraft(item) : null);
    setNewEnvKey("");
    setNewHeaderKey("");
    setError("");
  }

  async function refreshDisk() {
    if (!client?.connected) return;
    const disk = await client.call("mcp/read", { workspace: currentWorkspace });
    setView(disk);
  }

  async function refreshAfterMutation(result: MCPSettingsView) {
    setView(result);
    if (!currentWorkspace) return;
    try { await refreshDisk(); }
    catch (cause) { setError(formatRPCError(cause, "已保存，但项目配置刷新失败")); }
  }

  function payload(): MCPSaveInput | null {
    if (!draft || !view) return null;
    const name = draft.name.trim();
    if (!name) { setError("填写 Server 名称"); return null; }
    if (newEnvKey.trim() || newHeaderKey.trim()) { setError("请先添加填写中的环境变量或 Header"); return null; }
    if (target.create && view.global.some((item) => item.name === name)) { setError("Server 名称已存在"); return null; }
    const input: MCPSaveInput = {
      name, revision: view.revision, create: target.create,
      includeTools: list(draft.includeTools), excludeTools: list(draft.excludeTools),
    };
    if (draft.type === "http") {
      input.oauthClientId = draft.oauthClientId;
      input.oauthClientIdMetadataUrl = draft.oauthClientIdMetadataUrl;
      input.oauthRedirectUrl = draft.oauthRedirectUrl;
      if (draft.oauthClientSecretEnv !== "" || draft.clearOAuthClientSecretEnv) input.oauthClientSecretEnv = draft.oauthClientSecretEnv;
    }
    if (target.create) input.type = draft.type;
    for (const field of ["command", "url", "cwd"] as const) {
      if (draft[field] !== "" || draft.clear[field] || target.create) input[field] = draft[field];
    }
    if (target.create || draft.replaceArgs) input.args = draft.args;
    for (const kind of ["env", "headers"] as const) {
      const known = new Set(kind === "env" ? selected?.envKeys : selected?.headerKeys);
      const changes: Record<string, string | null> = {};
      for (const [key, value] of Object.entries(draft[kind])) {
        if (value === "" && known.has(key)) continue;
        if (value === "") { setError(`填写 ${key} 的值`); return null; }
        changes[key] = value;
      }
      if (Object.keys(changes).length) input[kind] = changes;
    }
    return input;
  }

  async function save(): Promise<boolean> {
    if (!client?.connected || !draft || !view || saving) return false;
    const input = payload();
    if (!input) return false;
    setSaving(true); setError("");
    try {
      const result = await client.call("mcp/save", input, { timeoutMs: null });
      await refreshAfterMutation(result);
      setTarget({ scope: "global", name: input.name, edit: false, create: false });
      setDraft(null);
      if (authTask?.state === "failed" || authTask?.state === "cancelled") {
        setAuthTask(null);
        setAuthTaskTarget(null);
      }
      return true;
    } catch (cause) {
      setError(formatRPCError(cause, "保存失败"));
      try { await refreshDisk(); } catch { /* 草稿和原错误保留 */ }
      return false;
    } finally { setSaving(false); }
  }

  async function saveThenSwitch() {
    const next = pending;
    if (next && await save()) { setPending(null); show(next); }
  }

  async function toggle(item: MCPServerView, enabled: boolean) {
    if (!client?.connected || !view || saving) return;
    setSaving(true); setError("");
    try {
      const result = await client.call("mcp/save", {
        name: item.name, revision: view.revision, create: false, enabled,
      }, { timeoutMs: null });
      await refreshAfterMutation(result);
    } catch (cause) {
      setError(formatRPCError(cause, "开关未保存"));
      try { await refreshDisk(); } catch { /* 保留错误 */ }
    } finally { setSaving(false); }
  }

  async function deleteServer() {
    if (!client?.connected || !view || !selected || saving) return;
    setSaving(true); setError("");
    try {
      const result = await client.call("mcp/delete", { name: selected.name, revision: view.revision }, { timeoutMs: null });
      await refreshAfterMutation(result); setDraft(null);
      setTarget({ scope: "global", name: result.global[0]?.name ?? "", edit: false, create: false });
    } catch (cause) {
      setError(formatRPCError(cause, "删除失败"));
      try { await refreshDisk(); } catch { /* 保留错误 */ }
    } finally { setSaving(false); }
  }

  async function retry() {
    if (!client?.connected || !selected || saving) return;
    setSaving(true); setError("");
    try { await refreshAfterMutation(await client.call("mcp/retry", { name: selected.name }, { timeoutMs: null })); }
    catch (cause) { setError(formatRPCError(cause, "重试失败")); }
    finally { setSaving(false); }
  }

  async function startOAuth() {
    if (!client?.connected || !selected || saving) return;
    setSaving(true); setError(""); setAuthTask(null);
    try {
      const task = await client.call("mcp/auth/start", { workspace: currentWorkspace, scope: target.scope, name: selected.name }, { timeoutMs: 40000 });
      setAuthTaskTarget({ scope: target.scope, name: selected.name, workspace: currentWorkspace });
      setAuthTask(task);
    } catch (cause) { setError(formatRPCError(cause, "启动 MCP 登录失败")); }
    finally { setSaving(false); }
  }

  async function trustAndStartOAuth() {
    if (!client?.connected || !view) return;
    setTrustPrompt(false); setSaving(true); setError("");
    try {
      await client.call("mcp/auth/trustProject", { workspace: currentWorkspace, version: view.projectVersion });
    } catch (cause) { setError(formatRPCError(cause, "项目配置确认失败")); setSaving(false); return; }
    setSaving(false);
    await startOAuth();
  }

  async function cancelOAuth() {
    if (!client?.connected || !authTask) return;
    try { setAuthTask(await client.call("mcp/auth/cancel", { id: authTask.id }, { timeoutMs: 10000 })); }
    catch (cause) { setError(formatRPCError(cause, "取消登录失败")); }
  }

  async function openAuthorizationURL(address: string) {
    try {
      await openExternal(address);
    } catch (cause) {
      setError(formatRPCError(cause, "打开授权页面失败"));
    }
  }

  async function logoutOAuth() {
    if (!client?.connected || !selected || saving) return;
    setSaving(true); setError("");
    try {
      const result = await client.call("mcp/auth/logout", { workspace: currentWorkspace, scope: target.scope, name: selected.name });
      await refreshAfterMutation(result); setAuthTask(null);
    } catch (cause) { setError(formatRPCError(cause, "退出登录失败")); }
    finally { setSaving(false); }
  }

  async function resetInvalid() {
    if (!client?.connected || !view || saving) return;
    setSaving(true); setError("");
    try {
      await refreshAfterMutation(await client.call("mcp/resetInvalid", { revision: view.revision }, { timeoutMs: null }));
      setTarget({ scope: "global", name: "", edit: false, create: false });
      setResetPrompt(false);
    } catch (cause) {
      setError(formatRPCError(cause, "重建失败"));
      try { await refreshDisk(); } catch { /* 保留错误 */ }
    } finally { setSaving(false); }
  }

  function updateField(field: SecretField, value: string) {
    if (draft) setDraft({ ...draft, [field]: value, clear: { ...draft.clear, [field]: false } });
  }

  function secretField(field: SecretField, title: string, configured: boolean) {
    if (!draft) return null;
    return <div className="settings-field" key={field}>
      <Label htmlFor={`mcp-${field}`}>{title}</Label>
      <div className="mcp-secret-row"><Input id={`mcp-${field}`} value={draft[field]} disabled={saving || draft.clear[field]}
        placeholder={configured && !target.create ? "已设置 · 留空保持原值" : title}
        onChange={(event) => updateField(field, event.target.value)} />
        {configured && !target.create && field === "cwd" && <Button variant="ghost" size="sm" disabled={saving}
          onClick={() => setDraft({ ...draft, [field]: "", clear: { ...draft.clear, [field]: !draft.clear[field] } })}>
          {draft.clear[field] ? "撤销清除" : "清除"}
        </Button>}</div>
    </div>;
  }

  function keyValues(kind: "env" | "headers", known: string[]) {
    if (!draft) return null;
    const changes = draft[kind];
    const keys = [...new Set([...known, ...Object.keys(changes)])].filter((key) => changes[key] !== null);
    const newKey = kind === "env" ? newEnvKey : newHeaderKey;
    const setNewKey = kind === "env" ? setNewEnvKey : setNewHeaderKey;
    return <div className="settings-field"><Label>{kind === "env" ? "环境变量" : "HTTP Header"}</Label>
      {keys.map((key) => <div className="mcp-pair" key={key}>
        <Input aria-label={`${key} 名称`} value={key} disabled />
        <Input aria-label={`${key} 值`} value={changes[key] ?? ""} type="password" autoComplete="off" disabled={saving}
          placeholder={known.includes(key) ? "已设置 · 留空保持" : "值"}
          onChange={(event) => {
            const updated = { ...changes };
            if (known.includes(key) && event.target.value === "") delete updated[key];
            else updated[key] = event.target.value;
            setDraft({ ...draft, [kind]: updated });
          }} />
        <Button variant="ghost" size="icon-sm" aria-label={`移除 ${key}`} disabled={saving}
          onClick={() => setDraft({ ...draft, [kind]: { ...changes, [key]: null } })}><X /></Button>
      </div>)}
      <div className="mcp-secret-row"><Input aria-label={kind === "env" ? "新环境变量名称" : "新 Header 名称"}
        value={newKey} onChange={(event) => setNewKey(event.target.value)} />
        <Button variant="ghost" size="sm" disabled={saving || !newKey.trim() || keys.includes(newKey.trim())}
          onClick={() => {
            const key = newKey.trim();
            const updated = { ...changes };
            if (known.includes(key)) delete updated[key];
            else updated[key] = "";
            setDraft({ ...draft, [kind]: updated });
            setNewKey("");
          }}><Plus />添加</Button></div>
    </div>;
  }

  return <div className="model-settings-page">
    <header className="settings-heading"><h2>MCP</h2></header>
    {error && <div className="inline-notice" role="alert">{error} <Button variant="ghost" size="sm" onClick={() => setReload((n) => n + 1)}>重新加载</Button></div>}
    <div className="settings-two-pane">
      <aside className="settings-subnav">
        <div className="settings-split-heading"><h3>全局</h3><Button size="icon-sm" variant="ghost" aria-label="添加 MCP Server"
          disabled={saving || !!view?.globalError} onClick={() => select({ scope: "global", name: "", edit: true, create: true })}><Plus /></Button></div>
        <div className="settings-subnav-list">{view?.global.map((item) => <Button key={item.name} variant="ghost"
          className="settings-subnav-item" aria-pressed={target.scope === "global" && target.name === item.name && !target.create}
          onClick={() => select({ scope: "global", name: item.name, edit: false, create: false })}>
          <Server /><span className="settings-subnav-copy"><span className="settings-subnav-name">{item.name}</span>
            <span className="settings-subnav-meta">{statusLabel(item)}</span></span>
        </Button>)}</div>
        {!!view?.project.length && <h3 className="mcp-project-heading">项目 · 只读</h3>}
        <div className="settings-subnav-list">{view?.project.map((item) => <Button key={item.name} variant="ghost"
          className="settings-subnav-item" aria-pressed={target.scope === "project" && target.name === item.name}
          onClick={() => select({ scope: "project", name: item.name, edit: false, create: false })}>
          <Server /><span className="settings-subnav-copy"><span className="settings-subnav-name">{item.name}</span>
            <span className="settings-subnav-meta">只读 · {statusLabel(item)}</span></span>
        </Button>)}</div>
      </aside>
      <div className="settings-detail-pane">
        {view?.globalError && <section className="settings-section"><h3>全局配置无法读取</h3>
          <p className="metadata">{view.globalPath}</p><Button variant="outline" onClick={() => setResetPrompt(true)}>备份并重建空配置</Button></section>}
        {view?.projectError && <section className="settings-section"><h3>项目配置无法读取</h3>
          <p className="metadata">{view.projectError}</p>{view.projectPaths.map((path) =>
            <div className="mcp-file-path" key={path}><code>{path}</code><Button size="sm" variant="ghost"
              onClick={() => onOpenFile(path)}>打开</Button><Button size="sm" variant="ghost"
              onClick={() => void navigator.clipboard.writeText(path)}>复制路径</Button></div>)}</section>}
        {!view?.globalError && target.edit && draft && <>
          <Button size="sm" variant="ghost" onClick={() => select({ scope: "global", name: selected?.name ?? "", edit: false, create: false })}><ArrowLeft />返回概况</Button>
          <h3 className="model-settings-editor-title">{target.create ? "添加 Server" : `编辑 · ${draft.name}`}</h3>
          <div className="model-settings-form mcp-settings-form">
            <div className="settings-fields"><div className="settings-field"><Label htmlFor="mcp-name">名称</Label>
              <Input id="mcp-name" value={draft.name} disabled={!target.create || saving}
                onChange={(event) => setDraft({ ...draft, name: event.target.value })} /></div>
              <div className="settings-field"><Label htmlFor="mcp-type">类型</Label>
                <Select value={draft.type} disabled={!target.create || saving} onValueChange={(type) => setDraft({ ...draft,
                  type: type as Draft["type"], command: "", url: "", cwd: "", args: [], env: {}, headers: {},
                  oauthClientId: "", oauthClientIdMetadataUrl: "", oauthClientSecretEnv: "", oauthRedirectUrl: "",
                  clearOAuthClientSecretEnv: false,
                })}>
                  <SelectTrigger id="mcp-type"><SelectValue /></SelectTrigger>
                  <SelectContent><SelectItem value="stdio">STDIO</SelectItem><SelectItem value="http">Streamable HTTP</SelectItem></SelectContent>
                </Select></div></div>
            {draft.type === "stdio" ? <>
              {secretField("command", "启动命令", !!selected?.hasCommand)}
              <div className="settings-field"><Label>参数{selected && !draft.replaceArgs ? ` · 已设置 ${selected.argCount} 个` : ""}</Label>
                {selected && !draft.replaceArgs ? <Button variant="outline" size="sm" onClick={() => setDraft({ ...draft, replaceArgs: true })}>替换参数</Button> : <>
                  {draft.args.map((arg, index) => <div className="mcp-secret-row" key={index}><Input aria-label={`参数 ${index + 1}`} value={arg}
                    onChange={(event) => setDraft({ ...draft, args: draft.args.map((value, i) => i === index ? event.target.value : value) })} />
                    <Button variant="ghost" size="icon-sm" aria-label={`移除参数 ${index + 1}`} onClick={() => setDraft({ ...draft, args: draft.args.filter((_, i) => i !== index) })}><X /></Button></div>)}
                  <Button variant="ghost" size="sm" onClick={() => setDraft({ ...draft, args: [...draft.args, ""] })}><Plus />添加参数</Button>
                </>}</div>
            </> : secretField("url", "Server URL", !!selected?.hasURL)}
            <Collapsible className="settings-advanced" key={`${target.create ? "new" : target.name}:${draft.type}`}
              defaultOpen={selectedAuthTask?.reason === "client-registration-required"}>
              <CollapsibleTrigger className="settings-advanced-trigger ui-focus">
                <ChevronRight className="disclosure-chevron" />高级设置
              </CollapsibleTrigger>
              <CollapsibleContent className="settings-advanced-content">
                {draft.type === "stdio" ? <>
                  {secretField("cwd", "工作目录", !!selected?.hasCWD)}
                  {keyValues("env", selected?.envKeys ?? [])}
                </> : <>
                  {keyValues("headers", selected?.headerKeys ?? [])}
                  <div className="settings-field"><Label htmlFor="mcp-oauth-id">OAuth 客户端 ID · 可选</Label>
                    <Input id="mcp-oauth-id" value={draft.oauthClientId} disabled={saving}
                      placeholder="预注册的 Client ID" onChange={(event) => setDraft({ ...draft, oauthClientId: event.target.value })} /></div>
                  <div className="settings-field"><Label htmlFor="mcp-oauth-cimd">CIMD 地址 · 可选</Label>
                    <Input id="mcp-oauth-cimd" value={draft.oauthClientIdMetadataUrl} disabled={saving}
                      placeholder="https://…/client.json" onChange={(event) => setDraft({ ...draft, oauthClientIdMetadataUrl: event.target.value })} /></div>
                  <div className="settings-field"><Label htmlFor="mcp-oauth-secret">客户端密钥环境变量 · 可选</Label>
                    <div className="mcp-secret-row"><Input id="mcp-oauth-secret" value={draft.oauthClientSecretEnv} disabled={saving || draft.clearOAuthClientSecretEnv}
                      placeholder={selected?.oauthClientSecretConfigured ? "已设置 · 留空保持" : "例如 MCP_CLIENT_SECRET"}
                      onChange={(event) => setDraft({ ...draft, oauthClientSecretEnv: event.target.value })} />
                      {selected?.oauthClientSecretConfigured && <Button variant="ghost" size="sm" disabled={saving}
                        onClick={() => setDraft({ ...draft, oauthClientSecretEnv: "", clearOAuthClientSecretEnv: !draft.clearOAuthClientSecretEnv })}>
                        {draft.clearOAuthClientSecretEnv ? "撤销清除" : "清除"}</Button>}</div></div>
                  <div className="settings-field"><Label htmlFor="mcp-oauth-redirect">已登记的回调地址 · 预注册/CIMD 必填</Label>
                    <Input id="mcp-oauth-redirect" value={draft.oauthRedirectUrl} disabled={saving}
                      placeholder="http://127.0.0.1:端口/oauth/callback" onChange={(event) => setDraft({ ...draft, oauthRedirectUrl: event.target.value })} /></div>
                </>}
                <div className="settings-field"><Label htmlFor="mcp-include">允许的工具</Label><Input id="mcp-include" value={draft.includeTools}
                  placeholder="留空表示全部" onChange={(event) => setDraft({ ...draft, includeTools: event.target.value })} /></div>
                <div className="settings-field"><Label htmlFor="mcp-exclude">禁用的工具</Label><Input id="mcp-exclude" value={draft.excludeTools}
                  onChange={(event) => setDraft({ ...draft, excludeTools: event.target.value })} /></div>
              </CollapsibleContent>
            </Collapsible>
          </div>
          {!target.create && <Button variant="ghost" className="settings-delete" onClick={() => void deleteServer()}><Trash2 />删除 Server</Button>}
          <div className="settings-savebar"><span className="settings-save-status">{dirty ? "有未保存的更改" : "尚无更改"}</span>
            <Button variant="ghost" onClick={() => show({ scope: "global", name: selected?.name ?? "", edit: false, create: false })}>放弃</Button>
            <Button disabled={saving || !dirty} onClick={() => void save()}>保存</Button></div>
        </>}
        {(!view?.globalError || target.scope === "project") && !target.edit && selected && <>
          <div className="settings-detail-title-row">
            <div className="settings-identity">
              <span className="settings-identity-icon"><Server /></span>
              <div><h3>{selected.name}</h3><p>{selected.type === "stdio" ? "STDIO" : "Streamable HTTP"}</p></div>
            </div>
            {target.scope === "global" && <div className="settings-toolbar">
              <Button variant="outline" size="sm" onClick={() => select({ ...target, edit: true })}><Pencil />编辑连接</Button>
              <Switch aria-label={`启用 ${selected.name}`} checked={selected.enabled} disabled={saving}
                onCheckedChange={(enabled) => void toggle(selected, enabled)} />
            </div>}
          </div>
          {selected.error && !authNeeded && <div className="inline-notice" role="status">{selected.error}</div>}
          <dl className="settings-facts">
            <div><dt>连接状态</dt><dd>
              <span className="settings-connection-status" data-status={selected.status}>{statusLabel(selected)}</span>
              {target.scope === "global" && selected.enabled && <Hint text="重连全部全局 Server"><Button variant="ghost" size="icon-sm" disabled={saving}
                aria-label="重连全部全局 Server" onClick={() => void retry()}><RefreshCw /></Button></Hint>}
            </dd></div>
            <div><dt>{selected.type === "stdio" ? "启动配置" : "服务地址"}</dt><dd>{selected.type === "stdio"
              ? `${selected.hasCommand ? "命令已设置" : "命令未设置"} · ${selected.argCount} 个参数`
              : selected.hasURL ? "已设置" : "未设置"}</dd></div>
            {selected.source && <div><dt>配置来源</dt><Hint text={selected.source}><dd><FileText />
              <span className="settings-truncate">{target.scope === "global" ? "全局" : "项目 · 只读"} · {selected.source.split(/[\\/]/).pop()}</span>
            </dd></Hint></div>}
          </dl>
          {showOAuth && <div className="mcp-auth-action" data-attention={authNeeded && !authBusy} role="status">
            <span className="mcp-auth-icon"><KeyRound /></span>
            <div className="mcp-auth-copy">
              <strong>{authBusy ? "正在授权" : selectedAuthTask?.state === "failed" ? "授权未完成" :
                selectedAuthTask?.state === "cancelled" ? "授权已取消" :
                selectedAuthTask?.state === "complete" && !selected.hasOAuthCredentials ? "授权已完成" :
                selected.status === "insufficient-scope" ? "需要增加授权权限" :
                selected.status === "auth-required" ? "需要登录" : "已授权"}</strong>
              <span>{selectedAuthTask?.message || (selected.status === "insufficient-scope" ? "当前权限不足，请重新授权。" :
                selected.status === "auth-required" ? "此 Server 请求 OAuth 授权。" : "凭据保存在本机。")}</span>
            </div>
            <div className="mcp-auth-actions">
              {authBusy ? <>
                {selectedAuthTask?.url && <Button size="sm" variant="outline" onClick={() => void openAuthorizationURL(selectedAuthTask.url!)}><ExternalLink />打开授权页面</Button>}
                <Button size="sm" variant="ghost" onClick={() => void cancelOAuth()}>取消</Button>
              </> : <>
                {selectedAuthTask?.reason === "client-registration-required" && (target.scope === "global"
                  ? <Button size="sm" variant="outline" onClick={() => select({ ...target, edit: true })}><Pencil />编辑授权配置</Button>
                  : selected.source && <Button size="sm" variant="outline" onClick={() => onOpenFile(selected.source)}><FileText />打开配置文件</Button>)}
                {selected.enabled && !(target.scope === "global" && selectedAuthTask?.reason === "client-registration-required") &&
                  <Button size="sm" variant={selected.hasOAuthCredentials ? "outline" : "default"} disabled={saving}
                  onClick={() => target.scope === "project" ? setTrustPrompt(true) : void startOAuth()}>
                  {selected.hasOAuthCredentials ? "重新授权" : "登录授权"}</Button>}
                {selected.hasOAuthCredentials && <Button size="sm" variant="ghost" disabled={saving} onClick={() => void logoutOAuth()}><LogOut />退出登录</Button>}
              </>}
            </div>
          </div>}
          <section className="settings-section mcp-tools">
            <div className="settings-section-header"><h3>可用工具 <span className="settings-badge">{selected.tools.length}</span></h3></div>
            {selected.tools.length ? <div className="settings-item-list">{selected.tools.map((tool) =>
              <div className="mcp-tool-row" key={tool}><Wrench /><span>{tool.startsWith(`mcp__${selected.name}__`)
                ? tool.slice(`mcp__${selected.name}__`.length) : tool}</span></div>)}</div>
              : <div className="settings-empty"><Wrench /><p>暂无可用工具</p></div>}
          </section>
        </>}
        {(!view?.globalError || target.scope === "project") && !target.edit && !selected && !view?.projectError && <p className="metadata">暂无 MCP Server</p>}
      </div>
    </div>
    <AlertDialog open={!!pending} onOpenChange={(open) => { if (!open) setPending(null); }}>
      <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>保存当前更改？</AlertDialogTitle>
        <AlertDialogDescription>切换 Server 前处理当前草稿。</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel>继续编辑</AlertDialogCancel>
          <AlertDialogAction variant="outline" onClick={() => { if (pending) show(pending); setPending(null); }}>放弃并切换</AlertDialogAction>
          <AlertDialogAction onClick={() => void saveThenSwitch()}>保存并切换</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
    </AlertDialog>
    <AlertDialog open={resetPrompt} onOpenChange={setResetPrompt}><AlertDialogContent>
      <AlertDialogHeader><AlertDialogTitle>重建全局 MCP 配置？</AlertDialogTitle>
        <AlertDialogDescription>原文件会先备份，再创建空配置。</AlertDialogDescription></AlertDialogHeader>
      <AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel>
        <AlertDialogAction onClick={() => void resetInvalid()}>备份并重建</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
    </AlertDialog>
    <AlertDialog open={trustPrompt} onOpenChange={setTrustPrompt}><AlertDialogContent>
      <AlertDialogHeader><AlertDialogTitle>确认项目 MCP 配置？</AlertDialogTitle>
        <AlertDialogDescription>本次确认覆盖项目中的全部 MCP Server。登录后会连接以下已启用项；STDIO 会在宿主执行启动命令。</AlertDialogDescription></AlertDialogHeader>
      <div className="settings-item-list">{view?.project.filter((server) => server.enabled).map((server) =>
        <div className="mcp-tool-row" key={server.name}><Server /><span>{server.name} · {server.type === "stdio" ? "STDIO 命令" : "HTTP 服务"}</span></div>)}</div>
      <p className="metadata">配置文件：{view?.projectPaths.join("、")}</p>
      <AlertDialogFooter><AlertDialogCancel>取消</AlertDialogCancel>
        <AlertDialogAction onClick={() => void trustAndStartOAuth()}>确认并登录</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
    </AlertDialog>
  </div>;
}
