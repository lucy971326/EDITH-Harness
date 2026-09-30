import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import type { ApprovalSettings, ApprovalSettingsView } from "../../../contracts/approvals";
import type { ModelChoice } from "../../../contracts/appserver";
import { RPCClient, formatRPCError } from "../client/rpc";
import { ModelMenu } from "../components/model-menu";
import { Brain, Plug, Check } from "../icons";
import { SettingsHeader } from "./settings-primitives";
import type { SettingsDraftState } from "./types";

export function ApprovalSettingsPanel({ client, models, modelError, onReloadModels, onSaved, onStateChange }: {
  client: RPCClient | null;
  models: ModelChoice[] | null;
  modelError: string;
  onReloadModels: () => void;
  onSaved: () => void;
  onStateChange: (state: SettingsDraftState) => void;
}) {
  const [view, setView] = useState<ApprovalSettingsView | null>(null);
  const [draft, setDraft] = useState<ApprovalSettings | null>(null);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [reload, setReload] = useState(0);

  useEffect(() => {
    let active = true;
    if (draft && view && (draft.engine !== view.settings.engine || draft.model !== view.settings.model || draft.reasoningEffort !== view.settings.reasoningEffort)) return;
    setView(null);
    setDraft(null);
    setError("");
    setSaved(false);
    if (!client?.connected) return;
    void client.call("approval/settings/read", {}).then((result) => {
      if (!active) return;
      setView(result);
      setDraft(result.settings);
    }).catch((cause: unknown) => {
      if (active) setError(formatRPCError(cause, "审核设置加载失败"));
    });
    return () => { active = false; };
  }, [client, reload]);

  function edit(next: ApprovalSettings) {
    setDraft(next);
    setSaved(false);
  }

  async function save() {
    if (!client?.connected || !draft || saving) return;
    setSaving(true);
    setError("");
    setSaved(false);
    try {
      const result = await client.call("approval/settings/update", draft);
      if (!client.connected) return;
      setView(result);
      setDraft(result.settings);
      setSaved(true);
      onSaved();
    } catch (cause) {
      if (client.connected) setError(formatRPCError(cause, "保存结果未确认，请重新加载设置"));
    } finally {
      setSaving(false);
    }
  }

  const modelValid = models?.some((model) => model.id === draft?.model && model.reasoningEfforts.includes(draft.reasoningEffort));
  const canSave = draft?.engine === "jev" ? view?.jevConfigured : modelValid;
  const dirty = !!draft && !!view && (draft.engine !== view.settings.engine || draft.model !== view.settings.model || draft.reasoningEffort !== view.settings.reasoningEffort);
  useEffect(() => { onStateChange({ dirty, saving }); }, [dirty, saving, onStateChange]);
  return <>
    <SettingsHeader title="智能审批" description="选择审核工具操作的方式与模型。" />
    {!client?.connected && <p className="inline-notice">连接后台后可修改。</p>}
    {error && <p className="inline-notice" role="alert">{error}</p>}
    {!view && client?.connected && !error && <p className="metadata">正在加载…</p>}
    {view && draft && <>
      <section className="settings-section">
      <div className="settings-section-header"><h3>审核方式</h3></div>
      <div className="settings-choice-grid" role="group" aria-label="审核方式">
        {(["llm", "jev"] as const).map((engine) => <button key={engine}
          className="settings-choice ui-focus"
          aria-pressed={draft.engine === engine} disabled={saving || !client?.connected}
          onClick={() => edit({ ...draft, engine })}>
          {engine === "llm" ? <Brain /> : <Plug />}
          <span><strong>{engine === "llm" ? "常规 LLM" : "Jev"}</strong>
            <span className="settings-description">{engine === "llm" ? "使用已配置的模型进行审核" : "使用 Jev 服务，需要独立密钥"}</span>
          </span>
          <span className="theme-check">{draft.engine === engine && <Check />}</span>
        </button>)}
      </div>
      </section>
      <section className="settings-section">
      <div className="settings-section-header"><h3>{draft.engine === "llm" ? "审核模型" : "服务连接"}</h3></div>
      {draft.engine === "llm" ? <div className="settings-model-field"><ModelMenu
        models={models} value={draft} disabled={saving || !client?.connected}
        error={modelError} onRetry={onReloadModels} requiresVision={false}
        description="选择独立的审核模型，不改变聊天使用的模型。"
        onChange={(value) => edit({ ...draft, ...value })}
      /></div> : <p className={view.jevConfigured ? "settings-description" : "settings-notice"}>{view.jevConfigured
        ? "Jev 密钥已配置，可以使用。"
        : "尚未配置密钥：在 ~/.harness/config.yaml 添加 jev.apiKey，然后重启后台。"}</p>}
      {draft.engine === "llm" && !modelValid && <p className="settings-notice">{models === null ? "正在加载可用模型…" : "请选择可用的审核模型与思考档位。"}</p>}
      </section>
      {(dirty || saving) && <div className="settings-savebar">
      <span className="settings-save-status" role="status">有未保存的更改</span>
      <Button variant="ghost" disabled={saving || !dirty} onClick={() => edit(view.settings)}>放弃</Button>
      <Button disabled={!client?.connected || saving || !dirty || !canSave} onClick={() => void save()}>
        {saving ? "保存中…" : "保存"}
      </Button>
      </div>}
      {!dirty && saved && <p className="settings-description" role="status">已保存</p>}
      {!dirty && !view.available && <p className="settings-notice" role="status">当前审核配置尚未就绪，请检查以上设置。</p>}
    </>}
    {error && <Button variant="ghost" disabled={saving || !client?.connected} onClick={() => setReload((value) => value + 1)}>重新加载</Button>}
  </>;
}
