import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import type { ApprovalSettings, ApprovalSettingsView } from "../../contracts/approvals";
import type { ModelChoice } from "../../contracts/appserver";
import { RPCClient, formatRPCError } from "./client/rpc";
import { ModelMenu } from "./model-menu";

export function ApprovalSettingsPanel({ client, models, modelError, onReloadModels, onSaved }: {
  client: RPCClient | null;
  models: ModelChoice[] | null;
  modelError: string;
  onReloadModels: () => void;
  onSaved: () => void;
}) {
  const [view, setView] = useState<ApprovalSettingsView | null>(null);
  const [draft, setDraft] = useState<ApprovalSettings | null>(null);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [reload, setReload] = useState(0);

  useEffect(() => {
    let active = true;
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
  return <>
    <h2>智能审批</h2>
    <p className="muted">所有会话共用。保存后用于新审批，已开始的审核保持原设置。</p>
    {!client?.connected && <p className="inline-notice">连接后台后可修改。</p>}
    {error && <p className="inline-notice" role="alert">{error}</p>}
    {!view && client?.connected && !error && <p className="metadata">正在加载…</p>}
    {view && draft && <>
      <h3 className="section-label">审核方式</h3>
      <div className="effort-options" role="group" aria-label="审核方式">
        {(["llm", "jev"] as const).map((engine) => <Button key={engine}
          variant={draft.engine === engine ? "default" : "outline"} size="sm"
          aria-pressed={draft.engine === engine} disabled={saving || !client?.connected}
          onClick={() => edit({ ...draft, engine })}>
          {engine === "llm" ? "常规 LLM" : "Jev"}
        </Button>)}
      </div>
      <h3 className="section-label">{draft.engine === "llm" ? "审核模型" : "Jev 配置"}</h3>
      {draft.engine === "llm" ? <ModelMenu
        models={models} value={draft} disabled={saving || !client?.connected}
        error={modelError} onRetry={onReloadModels} requiresVision={false}
        description="选择独立的审核模型，不改变聊天使用的模型。"
        onChange={(value) => edit({ ...draft, ...value })}
      /> : <p className="metadata">{view.jevConfigured
        ? "密钥已配置 · 批准置信度不足时转人工"
        : "尚未配置密钥：在 ~/.harness/config.yaml 添加 jev.apiKey，然后重启后台。"}</p>}
      {!view.available && <p className="metadata">当前保存的配置不可用，请完成配置后开启智能审批。</p>}
      <p className="muted">审核信息不足或调用失败时交给你批准；停止任务会取消审批。</p>
      <Button disabled={!client?.connected || saving || !canSave} onClick={() => void save()}>
        {saving ? "正在保存…" : "保存"}
      </Button>
      {saved && <span className="metadata" role="status"> 已保存</span>}
    </>}
    {error && <Button variant="ghost" disabled={saving || !client?.connected} onClick={() => setReload((value) => value + 1)}>重新加载</Button>}
  </>;
}
