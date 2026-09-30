import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Hint } from "@/components/ui/tooltip";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { ModelAuthView, ModelSettings, ModelSettingsView, ProviderSettings, ReasoningSettings, SaveProviderSettings } from "../../../contracts/appserver";
import { RPCClient, formatRPCError } from "../client/rpc";
import { ArrowLeft, ArrowUp, ArrowDown, Brain, Building2, ChevronRight, Plus, Pencil, Trash2, X } from "../icons";
import type { SettingsDraftState } from "./types";

type ProviderDraft = SaveProviderSettings;
type Editor = "overview" | "provider" | "model";
type Target =
  | { kind: "provider"; id: string }
  | { kind: "add-provider"; presetID?: string }
  | { kind: "edit-provider"; id: string }
  | { kind: "overview"; id?: string }
  | { kind: "model"; key: string }
  | { kind: "add-model"; providerID: string; presetKey?: string };
const protocols: { id: ProviderSettings["protocol"]; label: string; address: string }[] = [
  { id: "openai-chat", label: "OpenAI Chat Completions", address: "https://api.openai.com/v1" },
  { id: "openai-responses", label: "OpenAI Responses", address: "https://api.openai.com/v1" },
  { id: "openai-codex", label: "ChatGPT 订阅", address: "https://chatgpt.com/backend-api/codex" },
  { id: "anthropic", label: "Anthropic", address: "https://api.anthropic.com" },
];
const accountProviders = ["openai-codex", "xai-oauth"];

export function providerDraft(provider: ProviderSettings, revision: string): ProviderDraft {
  return { id: provider.id, protocol: provider.protocol, baseURL: provider.baseURL,
    apiKey: "", clearAPIKey: false, revision };
}

function emptyModel(provider: string): ModelSettings {
  return { key: "", provider, id: "", contextWindow: 32768, maxOutput: 8192, manual: true, vision: false,
    reasoning: [{ name: "off", mode: "off", effort: "", budgetTokens: 0 }] };
}

function levelDefault(protocol: ProviderSettings["protocol"]): ReasoningSettings {
  return { name: "", mode: "enabled", effort: "",
    budgetTokens: protocol === "anthropic" ? 1024 : 0 };
}

function windowLabel(tokens: number): string {
  if (tokens % 1000000 === 0) return `${tokens / 1000000}M`;
  if (tokens % 1000 === 0) return `${tokens / 1000}K`;
  return `${tokens.toLocaleString()} token`;
}

export function ModelSettingsPanel({ client, openExternal, onSaved, onStateChange }: {
  client: RPCClient | null;
  openExternal: (url: string) => Promise<void>;
  onSaved: () => void;
  onStateChange: (state: SettingsDraftState) => void;
}) {
  const [view, setView] = useState<ModelSettingsView | null>(null);
  const [provider, setProvider] = useState<ProviderDraft | null>(null);
  const [model, setModel] = useState<ModelSettings | null>(null);
  const [editor, setEditor] = useState<Editor>("overview");
  const [creatingProvider, setCreatingProvider] = useState(false);
  const [pendingTarget, setPendingTarget] = useState<Target | null>(null);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState("");
  const [saving, setSaving] = useState(false);
  const [reload, setReload] = useState(0);
  const [authViews, setAuthViews] = useState<Record<string, ModelAuthView>>({});
  const [modelSearch, setModelSearch] = useState("");
  const presets = view?.providerPresets ?? [];
  const accountID = provider?.id ?? "";
  const selectedProtocol = (editor === "model" ? model?.protocol : undefined) ?? provider?.protocol ?? "openai-chat";
  const accountProvider = accountProviders.includes(accountID);
  const auth = authViews[accountID];
  function setAuth(result: ModelAuthView) {
    setAuthViews((current) => ({ ...current, [accountID]: result }));
  }

  useEffect(() => {
    let active = true;
    setView(null); setProvider(null); setModel(null); setEditor("overview");
    setCreatingProvider(false); setPendingTarget(null); setError(""); setAuthViews({});
    if (!client?.connected) return;
    void Promise.all(accountProviders.map(async (providerID) => ({ providerID,
      result: await client.call("model/auth/status", { providerID }) }))).then((items) => {
        if (active) setAuthViews(Object.fromEntries(items.map((item) => [item.providerID, item.result])));
      })
      .catch((cause: unknown) => { if (active) setError(formatRPCError(cause, "读取登录状态失败")); });
    void client.call("model/config/read", {}).then((result) => {
      if (!active) return;
      setView(result);
      if (result.providers[0]) setProvider(providerDraft(result.providers[0], result.providerRevision));
    }).catch((cause: unknown) => {
      if (active) setError(formatRPCError(cause, "模型配置加载失败"));
    });
    return () => { active = false; };
  }, [client, reload]);

  useEffect(() => {
    if (!client?.connected || (auth?.state !== "waiting" && auth?.state !== "connecting")) return;
    let active = true;
    const timer = window.setInterval(() => {
      void client.call("model/auth/status", { providerID: accountID }).then(async (result) => {
        if (!active) return;
        setAuthViews((current) => ({ ...current, [accountID]: result }));
        if (result.state === "complete") {
          setView(await client.call("model/config/read", {}));
          onSaved();
        }
      }).catch((cause: unknown) => { if (active) setError(formatRPCError(cause, "读取登录状态失败")); });
    }, 1000);
    return () => { active = false; window.clearInterval(timer); };
  }, [client, accountID, auth?.state]);

  async function startAuth() {
    if (!client?.connected || saving) return;
    setSaving(true); setError("");
    try {
      const result = await client.call("model/auth/start", { providerID: accountID });
      setAuth(result);
      if (result.url) await openExternal(result.url);
    } catch (cause) { setError(formatRPCError(cause, "登录启动失败")); }
    finally { setSaving(false); }
  }

  async function cancelAuth() {
    if (!client?.connected) return;
    try { setAuth(await client.call("model/auth/cancel", { providerID: accountID })); }
    catch (cause) { setError(formatRPCError(cause, "取消登录失败")); }
  }

  async function logoutAuth() {
    if (!client?.connected || saving) return;
    setSaving(true); setError("");
    try {
      setAuth(await client.call("model/auth/logout", { providerID: accountID }));
      setView(await client.call("model/config/read", {}));
      onSaved();
    } catch (cause) { setError(formatRPCError(cause, "退出登录失败")); }
    finally { setSaving(false); }
  }

  const storedProvider = creatingProvider ? undefined : view?.providers.find((item) => item.id === provider?.id);
  const storedModel = view?.models.find((item) => item.key === model?.key);
  const providerDirty = editor === "provider" && !!provider && (!storedProvider || provider.protocol !== storedProvider.protocol ||
    provider.baseURL !== storedProvider.baseURL || provider.apiKey !== "" || provider.clearAPIKey);
  const modelDirty = editor === "model" && !!model && (!storedModel || JSON.stringify(model) !== JSON.stringify(storedModel));
  const dirty = providerDirty || modelDirty;
  const providerModels = view?.models.filter((item) => item.provider === storedProvider?.id) ?? [];
  const duplicateProviderID = creatingProvider && view?.providers.some((item) => item.id === provider?.id);
  const duplicateModelID = editor === "model" && !!model && !model.key &&
    providerModels.some((item) => item.id === model.id.trim());
  const levelValues = model?.reasoning.map((level) => level.mode === "off" ? "off" : level.effort) ?? [];
  const duplicateLevels = levelValues.some((value, index) => value !== "" && levelValues.indexOf(value) !== index);
  const invalidLevels = levelValues.some((value) => value === "") || duplicateLevels;
  useEffect(() => { onStateChange({ dirty, saving }); }, [dirty, saving, onStateChange]);

  function showTarget(target: Target, current: ModelSettingsView) {
    setModelSearch("");
    if (target.kind === "add-provider") {
      const preset = presets.find((item) => item.id === target.presetID);
      setProvider({ id: preset?.id ?? "", protocol: preset?.protocol ?? "openai-chat", baseURL: preset?.baseURL ?? "",
        apiKey: "", clearAPIKey: false, revision: current.providerRevision });
      setCreatingProvider(true); setModel(null); setEditor("provider");
      return;
    }
    if (target.kind === "add-model") {
      const selected = current.providers.find((item) => item.id === target.providerID);
      if (!selected) return;
      const preset = current.presets.find((item) => item.key === target.presetKey);
      setProvider(providerDraft(selected, current.providerRevision));
      setModel(preset ? { ...structuredClone(preset), key: "", provider: selected.id } : emptyModel(selected.id));
      setCreatingProvider(false); setEditor("model");
      return;
    }
    if (target.kind === "model") {
      const selected = current.models.find((item) => item.key === target.key);
      const owner = current.providers.find((item) => item.id === selected?.provider);
      if (!selected || !owner) return;
      setProvider(providerDraft(owner, current.providerRevision));
      setModel(structuredClone(selected)); setCreatingProvider(false); setEditor("model");
      return;
    }
    const id = target.id;
    const found = current.providers.find((item) => item.id === id);
    const selected = target.kind === "overview" ? found ?? current.providers[0] : found;
    if (!selected && target.kind !== "overview") return;
    setProvider(selected ? providerDraft(selected, current.providerRevision) : null);
    setModel(null); setCreatingProvider(false);
    setEditor(target.kind === "edit-provider" && selected ? "provider" : "overview");
  }

  function requestTarget(target: Target) {
    if (!view || saving) return;
    if (dirty) { setPendingTarget(target); return; }
    setError(""); setSaved("");
    showTarget(target, view);
  }

  async function refreshAfterFailure(cause: unknown) {
    setError(formatRPCError(cause, "保存未成功；已重新读取磁盘配置，请检查后重试"));
    if (!client?.connected) return;
    try {
      setView(await client.call("model/config/read", {}));
      onSaved();
    } catch { /* 保留原错误与草稿 */ }
  }

  async function saveProvider(): Promise<ModelSettingsView | null> {
    if (!client?.connected || !provider || !view || saving || !providerDirty || duplicateProviderID) return null;
    setSaving(true); setError(""); setSaved("");
    try {
      // 显式投影写入字段，也排除热更新前草稿中残留的只读字段。
      const result = await client.call("model/provider/save", {
        id: provider.id, protocol: provider.protocol, baseURL: provider.baseURL,
        apiKey: provider.apiKey, clearAPIKey: provider.clearAPIKey, revision: view.providerRevision,
      });
      setView(result);
      setProvider(providerDraft(result.providers.find((item) => item.id === provider.id)!, result.providerRevision));
      setCreatingProvider(false); setEditor("overview");
      onSaved(); setSaved("供应商已保存");
      return result;
    } catch (cause) { await refreshAfterFailure(cause); return null; }
    finally { setSaving(false); }
  }

  async function deleteProvider() {
    if (!client?.connected || !storedProvider || !view || saving) return;
    setSaving(true); setError(""); setSaved("");
    try {
      const result = await client.call("model/provider/delete", {
        id: storedProvider.id, providerRevision: view.providerRevision, modelRevision: view.modelRevision });
      setView(result);
      setProvider(result.providers[0] ? providerDraft(result.providers[0], result.providerRevision) : null);
      setModel(null); setEditor("overview"); onSaved(); setSaved("供应商已删除");
    } catch (cause) { await refreshAfterFailure(cause); }
    finally { setSaving(false); }
  }

  async function saveModel(): Promise<ModelSettingsView | null> {
    if (!client?.connected || !model || !view || saving || !modelDirty || invalidLevels || duplicateModelID) return null;
    setSaving(true); setError(""); setSaved("");
    try {
      const result = await client.call("model/definition/save", { model, revision: view.modelRevision });
      setView(result);
      setModel(null); setEditor("overview");
      const owner = result.providers.find((item) => item.id === model.provider);
      setProvider(owner ? providerDraft(owner, result.providerRevision) : null);
      onSaved(); setSaved("模型已保存");
      return result;
    } catch (cause) { await refreshAfterFailure(cause); return null; }
    finally { setSaving(false); }
  }

  async function deleteModel() {
    if (!client?.connected || !model?.key || !view || saving) return;
    setSaving(true); setError(""); setSaved("");
    try {
      const result = await client.call("model/definition/delete", { key: model.key, revision: view.modelRevision });
      setView(result); setModel(null); setEditor("overview"); onSaved(); setSaved("模型已删除");
    } catch (cause) { await refreshAfterFailure(cause); }
    finally { setSaving(false); }
  }

  async function saveBeforeSwitch() {
    const target = pendingTarget;
    setPendingTarget(null);
    const result = editor === "provider" ? await saveProvider() : await saveModel();
    if (target && result) { showTarget(target, result); setSaved(""); }
  }

  function discardBeforeSwitch() {
    if (pendingTarget && view) showTarget(pendingTarget, view);
    setPendingTarget(null); setError(""); setSaved("");
  }

  function updateLevel(index: number, input: string) {
    if (!model) return;
    const effort = input.trim();
    setModel({ ...model, reasoning: model.reasoning.map((level, position) => position === index ? {
      ...level,
      name: effort,
      effort: effort === "off" ? "" : effort,
      mode: effort === "off" ? "off" : level.mode === "adaptive" ? "adaptive" : "enabled",
      // 已配置的协议参数继续保留；Anthropic 手动思考沿用最小预算默认值。
      budgetTokens: selectedProtocol === "anthropic" ? level.budgetTokens || 1024 : level.budgetTokens,
    } : level) });
    setSaved("");
  }

  function moveLevel(index: number, offset: number) {
    if (!model || index + offset < 0 || index + offset >= model.reasoning.length) return;
    const reasoning = [...model.reasoning];
    [reasoning[index], reasoning[index + offset]] = [reasoning[index + offset], reasoning[index]];
    setModel({ ...model, reasoning }); setSaved("");
  }

  const selectedProvider = storedProvider;
  const protocolLabel = protocols.find((item) => item.id === selectedProtocol)?.label ?? selectedProtocol;
  const canSaveCurrent = editor === "provider"
    ? !!provider?.id && providerDirty && !duplicateProviderID
    : !!model?.id && modelDirty && !invalidLevels && !duplicateModelID;

  return <div className="model-settings-page">
    <header className="settings-heading model-settings-heading">
      <h2>模型与供应商</h2>
      {view && <DropdownMenu>
        <DropdownMenuTrigger asChild><Button size="sm" variant="outline" disabled={saving}><Plus />添加供应商</Button></DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {presets.filter((item) => !!item.id && !accountProviders.includes(item.id) && !view.providers.some((savedProvider) => savedProvider.id === item.id))
            .map((item) => <DropdownMenuItem key={item.id}
              onSelect={() => requestTarget({ kind: "add-provider", presetID: item.id })}>{item.name}</DropdownMenuItem>)}
          <DropdownMenuItem onSelect={() => requestTarget({ kind: "add-provider" })}>手动添加</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>}
    </header>
    {!client?.connected && <p className="inline-notice">连接后台后可修改。</p>}
    {error && <div className="inline-notice" role="alert">{error}
      {!view && <Button size="sm" variant="ghost" onClick={() => setReload((value) => value + 1)}>重新加载</Button>}
    </div>}
    {!view && client?.connected && !error && <p className="metadata">正在加载配置…</p>}
    {saved && <p className="settings-save-status" role="status">{saved}</p>}
    {view && <div className="model-settings-layout">
      <aside className="model-provider-nav" aria-label="供应商">
        <h3>供应商</h3>
        <div className="model-provider-list">
          {view.providers.map((item) => {
            const count = view.models.filter((entry) => entry.provider === item.id).length;
            return <Button key={item.id} variant="ghost" className="settings-subnav-item"
              aria-pressed={selectedProvider?.id === item.id} disabled={saving}
              onClick={() => requestTarget({ kind: "provider", id: item.id })}>
              <Building2 /><span className="settings-subnav-copy"><span className="settings-subnav-name">{presets.find((preset) => preset.id === item.id)?.name ?? item.id}</span>
              <span className="settings-subnav-meta">{count} 个模型</span></span>
            </Button>;
          })}
        </div>
      </aside>
      <div className="model-settings-detail">
        {editor === "overview" && selectedProvider && <>
          <div className="settings-detail-title-row">
            <div className="settings-identity"><span className="settings-identity-icon"><Building2 /></span><div><h3>{presets.find((preset) => preset.id === selectedProvider.id)?.name ?? selectedProvider.id}</h3>
              <p className="model-settings-summary">{protocolLabel} · {accountProvider
                ? auth?.authenticated ? "已登录" : "未登录" : selectedProvider.hasAPIKey ? "密钥已设置" : "未设置密钥"}</p>
            </div></div>
            {!accountProvider && <Button size="sm" variant="outline" disabled={saving}
              onClick={() => requestTarget({ kind: "edit-provider", id: selectedProvider.id })}><Pencil />编辑连接</Button>}
          </div>
          {accountProvider ? <div className="model-settings-auth">
            {auth?.message && <p className="inline-notice" role="status">{auth.message}</p>}
            {auth?.state === "waiting" || auth?.state === "connecting" ? <>
              <span className="metadata">{auth.state === "waiting" ? "等待浏览器授权…" : "正在完成登录…"}</span>
              {auth.userCode && <strong className="model-settings-device-code">{auth.userCode}</strong>}
              {auth.url && <Button size="sm" variant="outline" onClick={() => void openExternal(auth.url!)}>打开授权页</Button>}
              <Button size="sm" variant="ghost" onClick={() => void cancelAuth()}>取消</Button>
            </> : auth?.authenticated ? <Button size="sm" variant="outline" disabled={saving}
              onClick={() => void logoutAuth()}>退出账号</Button> : <Button size="sm" disabled={saving}
              onClick={() => void startAuth()}>登录账号</Button>}
          </div> : <p className="model-settings-address">{selectedProvider.baseURL || protocols.find((item) => item.id === selectedProtocol)?.address}</p>}
          <section className="model-settings-models">
            <div className="model-settings-title-row"><h4>模型 <span className="model-settings-count">{providerModels.length}</span></h4>
              <DropdownMenu>
                <DropdownMenuTrigger asChild><Button size="sm" variant="outline" disabled={saving}><Plus />添加模型</Button></DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onSelect={() => requestTarget({ kind: "add-model", providerID: selectedProvider.id })}>手动添加</DropdownMenuItem>
                  {view.presets.filter((item) => item.provider === selectedProvider.id &&
                    !providerModels.some((existing) => existing.id === item.id)).map((item) =>
                      <DropdownMenuItem key={item.key}
                        onSelect={() => requestTarget({ kind: "add-model", providerID: selectedProvider.id, presetKey: item.key })}>
                        预设 · {item.id}
                      </DropdownMenuItem>)}
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
            {providerModels.length > 8 && <Input aria-label="搜索模型" placeholder="搜索模型" value={modelSearch} onChange={(event) => setModelSearch(event.target.value)} />}
            <div className="model-entry-list">
              {providerModels.filter((item) => item.id.toLowerCase().includes(modelSearch.toLowerCase())).map((item) => <Button key={item.key} variant="ghost" className="model-entry-row" disabled={saving}
                onClick={() => requestTarget({ kind: "model", key: item.key })}>
                <Brain /><span className="model-entry-name">{item.id}</span>
                <span className="model-entry-meta">{windowLabel(item.contextWindow)}{item.vision ? " · 支持图片" : ""}</span><ChevronRight />
              </Button>)}
              {providerModels.length === 0 && <p className="metadata">暂无模型</p>}
            </div>
          </section>
        </>}
        {editor === "overview" && !selectedProvider && <p className="metadata">暂无供应商</p>}
        {editor === "provider" && provider && <>
          <Button size="sm" variant="ghost" className="model-settings-back" disabled={saving}
            onClick={() => requestTarget({ kind: "overview", id: provider.id })}><ArrowLeft />返回概况</Button>
          <h3 className="model-settings-editor-title">{creatingProvider ? "添加供应商" : `编辑连接 · ${provider.id}`}</h3>
          <div className="model-settings-form">
            <div className="settings-fields">
              <div className="settings-field"><Label htmlFor="provider-id">供应商 ID</Label><Input id="provider-id" value={provider.id}
                disabled={saving || !creatingProvider} placeholder="例如 my-openai"
                aria-invalid={!!duplicateProviderID}
                onChange={(event) => setProvider({ ...provider, id: event.target.value })} />
                {duplicateProviderID && <span className="settings-description" role="alert">供应商 ID 已存在</span>}
              </div>
              <div className="settings-field"><Label htmlFor="provider-protocol">请求协议</Label>
                <Select value={provider.protocol} disabled={saving || (!!provider.id && view.models.some((item) => item.provider === provider.id))}
                  onValueChange={(value) => setProvider({ ...provider, protocol: value as ProviderSettings["protocol"] })}>
                  <SelectTrigger id="provider-protocol" className="w-full"><SelectValue /></SelectTrigger>
                  <SelectContent>{protocols.filter((item) => !accountProviders.includes(item.id)).map((item) =>
                    <SelectItem key={item.id} value={item.id}>{item.label}</SelectItem>)}</SelectContent>
                </Select>
                {!!storedProvider && providerModels.length > 0 &&
                  <span className="settings-description">删除该供应商的模型后可更改协议。</span>}
              </div>
            </div>
            {!accountProvider && <><div className="settings-field"><Label htmlFor="provider-url">API 地址</Label><Input id="provider-url" value={provider.baseURL}
              disabled={saving} placeholder={protocols.find((item) => item.id === selectedProtocol)?.address}
              onChange={(event) => setProvider({ ...provider, baseURL: event.target.value })} />
              <span className="settings-description">{provider.id.startsWith("cloudflare-") ? "填写完整地址并替换账号／网关 ID；AI Gateway 填网关根地址。" : "留空使用供应商的默认地址。"}</span></div>
            <div className="settings-field"><Label htmlFor="provider-key">{provider.id === "github-copilot" ? "Copilot API Token" : "API 密钥"}</Label>
              <Input id="provider-key" type="password" autoComplete="off" value={provider.apiKey} disabled={saving || provider.clearAPIKey}
                placeholder={storedProvider?.hasAPIKey ? "已配置 · 留空表示保持原密钥" : "输入 API 密钥"}
                onChange={(event) => setProvider({ ...provider, apiKey: event.target.value })} />
              {provider.id === "github-copilot" && <span className="settings-description">填写已换取的 Copilot API Token，不是 GitHub PAT。</span>}
              {storedProvider?.hasAPIKey && <div className="model-settings-check"><Switch id="provider-clear-key" checked={provider.clearAPIKey}
                disabled={saving} onCheckedChange={(checked) => setProvider({ ...provider, clearAPIKey: checked, apiKey: "" })} />
                <Label htmlFor="provider-clear-key">清除已保存的密钥</Label></div>}
            </div></>}
          </div>
          {storedProvider && <div className="settings-danger-row">
            <Button variant="ghost" className="settings-delete" disabled={saving}
              onClick={() => void deleteProvider()}>
              <Trash2 />{providerModels.length > 0 ? `删除供应商及 ${providerModels.length} 个模型` : "删除供应商"}
            </Button>
          </div>}
          <div className="settings-savebar">
            <span className="settings-save-status" role="status">{providerDirty ? "有未保存的更改" : "尚无更改"}</span>
            <Button variant="ghost" disabled={saving} onClick={() => {
              showTarget({ kind: "overview", id: provider.id }, view); setError("");
            }}>{providerDirty ? "放弃更改" : "返回概况"}</Button>
            <Button disabled={saving || !canSaveCurrent} onClick={() => void saveProvider()}>保存</Button>
          </div>
        </>}
        {editor === "model" && model && provider && <>
          <Button size="sm" variant="ghost" className="model-settings-back" disabled={saving}
            onClick={() => requestTarget({ kind: "overview", id: provider.id })}><ArrowLeft />返回概况</Button>
          <h3 className="model-settings-editor-title">{model.key ? `编辑模型 · ${model.id}` : "添加模型"}</h3>
          <div className="model-settings-form">
            {view.presets.some((item) => item.key === `${model.provider}/${model.id}`) && <label className="model-settings-check">
              <Switch checked={!!model.manual} disabled={saving} onCheckedChange={(manual) => {
                const preset = view.presets.find((item) => item.key === `${model.provider}/${model.id}`);
                setModel(manual || !preset ? { ...model, manual } : { ...preset, key: model.key, manual: false });
              }} />高级手动配置
            </label>}
            {!model.manual && <p className="metadata">能力来自内置模型目录，随应用版本更新。</p>}

            <div className="settings-fields">
              <div className="settings-field"><Label htmlFor="model-id">模型 ID</Label><Input id="model-id" value={model.id}
                disabled={saving || !!model.key} aria-invalid={!!duplicateModelID}
                onChange={(event) => setModel({ ...model, id: event.target.value })} />
                {duplicateModelID && <span className="settings-description" role="alert">模型 ID 已存在</span>}
              </div>
              <div className="settings-field"><Label htmlFor="model-window">上下文窗口 · token</Label><Input id="model-window" type="number" min={1} value={model.contextWindow}
                disabled={saving || !model.manual} onChange={(event) => setModel({ ...model, contextWindow: Number(event.target.value) })} /></div>
            </div>
            {model.manual && <div className="settings-field"><Label htmlFor="model-output">最大输出 · token</Label><Input id="model-output" type="number" min={1} max={model.contextWindow} value={model.maxOutput ?? ""} disabled={saving} onChange={(event) => setModel({ ...model, maxOutput: Number(event.target.value) })} /></div>}
            <label className="model-settings-check"><Switch checked={model.vision} disabled={saving || !model.manual}
              onCheckedChange={(checked) => setModel({ ...model, vision: checked })} />支持图片输入</label>
            <div className="settings-section-header"><h4>思考档位</h4>
              <Button size="sm" variant="ghost" disabled={saving || !model.manual || levelValues.includes("")} onClick={() => {
                setModel({ ...model, reasoning: [...model.reasoning, levelDefault(selectedProtocol)] });
                setSaved("");
              }}><Plus />添加档位</Button></div>
            <ol className="model-levels" aria-label="思考档位顺序">
              {model.reasoning.map((level, index) => <li className="model-level" key={index}>
                <span className="model-level-index" aria-hidden="true">{index + 1}</span>
                <Input aria-label={`第 ${index + 1} 档思考强度`} value={levelValues[index]} disabled={saving || !model.manual}
                  placeholder="off / on / low / high" maxLength={32}
                  aria-invalid={levelValues[index] !== "" && levelValues.indexOf(levelValues[index]) !== index}
                  onChange={(event) => updateLevel(index, event.target.value)} />
                <div className="model-level-actions">
                  <Hint text="上移"><Button size="icon-sm" variant="ghost" aria-label={`上移第 ${index + 1} 档`}
                    disabled={saving || !model.manual || index === 0} onClick={() => moveLevel(index, -1)}><ArrowUp /></Button></Hint>
                  <Hint text="下移"><Button size="icon-sm" variant="ghost" aria-label={`下移第 ${index + 1} 档`}
                    disabled={saving || !model.manual || index === model.reasoning.length - 1} onClick={() => moveLevel(index, 1)}><ArrowDown /></Button></Hint>
                  <Hint text="移除"><Button size="icon-sm" variant="ghost" aria-label={`移除第 ${index + 1} 档`}
                    disabled={saving || !model.manual || model.reasoning.length === 1}
                    onClick={() => { setModel({ ...model, reasoning: model.reasoning.filter((_, position) => position !== index) }); setSaved(""); }}><X /></Button></Hint>
                </div>
                {model.manual && selectedProtocol === "anthropic" && level.mode !== "off" && level.name !== "auto" && <div className="model-level-options settings-fields">
                  <Select value={level.mode} disabled={saving} onValueChange={(mode) => setModel({ ...model,
                    reasoning: model.reasoning.map((item, position) => position === index ? { ...item, mode: mode as ReasoningSettings["mode"], budgetTokens: item.budgetTokens || 1024 } : item) })}>
                    <SelectTrigger aria-label={`第 ${index + 1} 档思考模式`}><SelectValue /></SelectTrigger>
                    <SelectContent><SelectItem value="enabled">固定预算</SelectItem><SelectItem value="adaptive">自适应</SelectItem></SelectContent>
                  </Select>
                  {level.mode === "enabled" && <Input type="number" min={1024} aria-label={`第 ${index + 1} 档思考预算`} value={level.budgetTokens}
                    disabled={saving} onChange={(event) => setModel({ ...model, reasoning: model.reasoning.map((item, position) => position === index ? { ...item, budgetTokens: Number(event.target.value) } : item) })} />}
                </div>}
              </li>)}
            </ol>
            {duplicateLevels && <p className="inline-notice" role="alert">思考强度不能重复。</p>}
          </div>
          {!!model.key && <div className="settings-danger-row">
            <Button variant="ghost" className="settings-delete" disabled={saving}
              onClick={() => void deleteModel()}><Trash2 />删除模型</Button>
          </div>}
          <div className="settings-savebar">
            <span className="settings-save-status" role="status">{modelDirty ? "有未保存的更改" : "尚无更改"}</span>
            <Button variant="ghost" disabled={saving} onClick={() => {
              showTarget({ kind: "overview", id: provider.id }, view); setError("");
            }}>{modelDirty ? "放弃更改" : "返回概况"}</Button>
            <Button disabled={saving || !canSaveCurrent} onClick={() => void saveModel()}>保存</Button>
          </div>
        </>}
      </div>
    </div>}
    <AlertDialog open={!!pendingTarget} onOpenChange={(open) => { if (!open) setPendingTarget(null); }}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>切换前保存更改？</AlertDialogTitle>
          <AlertDialogDescription>当前表单有未保存的更改。</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>继续编辑</AlertDialogCancel>
          <AlertDialogAction variant="outline" onClick={discardBeforeSwitch}>放弃并切换</AlertDialogAction>
          <AlertDialogAction disabled={!canSaveCurrent || saving} onClick={() => void saveBeforeSwitch()}>保存并切换</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>;
}
