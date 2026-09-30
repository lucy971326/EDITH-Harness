import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Hint } from "@/components/ui/tooltip";
import {
  AlertDialog, AlertDialogContent, AlertDialogHeader, AlertDialogTitle,
  AlertDialogDescription, AlertDialogFooter, AlertDialogCancel,
} from "@/components/ui/alert-dialog";
import type { PromptCommand, PromptCommandFile } from "../../../contracts/appserver";
import { RPCClient, formatRPCError } from "../client/rpc";
import { Command, Plus, RefreshCw, Trash2 } from "../icons";
import type { SettingsDraftState } from "./types";
import { SettingsHeader, SettingsListToolbar, SettingsResourceRow, SettingsScope, SettingsEmpty } from "./settings-primitives";

type Scope = "user" | "workspace";
type Page = "list" | "create" | "edit";
const blank = (): PromptCommand => ({ name: "", description: "", argumentHint: "", prompt: "" });
const editable = (item: PromptCommand): PromptCommand => ({
  name: item.name, description: item.description ?? "", argumentHint: item.argumentHint ?? "", prompt: item.prompt,
});

export function CommandSettingsPanel({ client, currentWorkspace, onStateChange, onChanged }: {
  client: RPCClient | null;
  currentWorkspace: string;
  onStateChange: (state: SettingsDraftState) => void;
  onChanged: () => void;
}) {
  const [scope, setScope] = useState<Scope>("user");
  const [workspace, setWorkspace] = useState(currentWorkspace);
  const [file, setFile] = useState<PromptCommandFile | null>(null);
  const [page, setPage] = useState<Page>("list");
  const [selectedName, setSelectedName] = useState("");
  const [draft, setDraft] = useState<PromptCommand | null>(null);
  const [original, setOriginal] = useState<PromptCommand | null>(null);
  const [query, setQuery] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [deleteName, setDeleteName] = useState("");
  const [discardOpen, setDiscardOpen] = useState(false);
  const [reload, setReload] = useState(0);
  const dirty = !!draft && (page === "create"
    ? !!(draft.name.trim() || draft.description?.trim() || draft.argumentHint?.trim() || draft.prompt.trim())
    : page === "edit" && !!original && JSON.stringify(draft) !== JSON.stringify(editable(original)));
  const workspaceName = workspace.replace(/[\\/]+$/, "").split(/[\\/]/).pop() || "选择工作区";

  useEffect(() => { onStateChange({ dirty, saving }); }, [dirty, saving, onStateChange]);
  useEffect(() => { if (!dirty) setWorkspace(currentWorkspace); }, [currentWorkspace]);
  useEffect(() => {
    let active = true;
    if (!dirty) {
      setFile(null);
      setPage("list");
      setDraft(null);
      setOriginal(null);
    }
    setError("");
    if (!client?.connected || (scope === "workspace" && !workspace)) return;
    void client.call("command/settings/read", { scope, workspace }).then((result) => {
      if (!active) return;
      if (dirty && result.hash !== file?.hash) {
        setError("命令配置已在别处变更；取消编辑后刷新。");
        return;
      }
      setFile(result);
    }).catch((cause: unknown) => {
      if (active) setError(formatRPCError(cause, "命令读取失败"));
    });
    return () => { active = false; };
  }, [client, scope, workspace, reload]);

  function chooseScope(next: Scope) {
    if (saving || next === scope) return;
    if (dirty) { setError("先保存或取消当前编辑。"); return; }
    setPage("list");
    setDraft(null);
    setOriginal(null);
    setFile(null);
    setScope(next);
  }

  async function chooseWorkspace() {
    if (dirty) { setError("先保存或取消当前编辑。"); return; }
    if (!client?.connected || saving) return;
    try {
      const result = await client.call("workspace/select", {});
      if (!result.canceled && result.workspace) {
        setPage("list");
        setDraft(null);
        setOriginal(null);
        setFile(null);
        setWorkspace(result.workspace);
      }
    } catch (cause) { setError(formatRPCError(cause, "选择工作区失败")); }
  }

  function startNew() {
    if (saving || !file) return;
    setSelectedName("");
    setDraft(blank());
    setOriginal(null);
    setPage("create");
    setError("");
  }

  function edit(item: PromptCommand) {
    if (saving) return;
    setSelectedName(item.name);
    setDraft(editable(item));
    setOriginal(editable(item));
    setPage("edit");
    setError("");
  }

  function back() {
    if (saving) return;
    if (dirty) { setDiscardOpen(true); return; }
    cancel();
  }

  function cancel() {
    if (saving) return;
    setPage("list");
    setDraft(null);
    setOriginal(null);
    setError("");
  }

  function refresh() {
    if (saving) return;
    if (dirty) { setError("先保存或取消当前编辑。"); return; }
    setReload((value) => value + 1);
    onChanged();
  }

  async function save() {
    if (!client?.connected || !file || !draft || saving) return;
    setSaving(true);
    setError("");
    try {
      const result = await client.call("command/settings/save", {
        scope, workspace, hash: file.hash, create: page === "create", command: draft,
      });
      setFile(result);
      setDraft(null);
      setOriginal(null);
      setPage("list");
      setQuery("");
      onChanged();
    } catch (cause) { setError(formatRPCError(cause, "保存失败；请检查命令或重新加载配置")); }
    finally { setSaving(false); }
  }

  async function remove() {
    if (!client?.connected || !file || !deleteName || saving) return;
    setSaving(true);
    setError("");
    try {
      const result = await client.call("command/settings/delete", {
        scope, workspace, hash: file.hash, name: deleteName,
      });
      setFile(result);
      setDraft(null);
      setOriginal(null);
      setPage("list");
      setDeleteName("");
      onChanged();
    } catch (cause) { setError(formatRPCError(cause, "删除失败；请重新加载配置")); }
    finally { setSaving(false); }
  }

  const shown = file?.commands.filter((item) =>
    `${item.name} ${item.description ?? ""}`.toLowerCase().includes(query.toLowerCase())) ?? [];
  return <div className="command-settings-page">
    {page === "list" ? <>
      <SettingsHeader title="命令" description="将常用提示词保存为命令，在聊天中输入 / 调用。"
        actions={<Button size="sm" disabled={!file || saving || !client?.connected} onClick={startNew}><Plus />新建命令</Button>} />
      {error && <div className="inline-notice" role="alert">{error}</div>}
      <SettingsListToolbar query={query} onQueryChange={setQuery} placeholder="搜索命令"
        actions={<Hint text="刷新命令"><Button size="icon-sm" variant="ghost" aria-label="刷新命令"
          disabled={saving || !client?.connected} onClick={refresh}><RefreshCw /></Button></Hint>}>
        <SettingsScope value={scope} onChange={chooseScope} workspace={workspace} onChooseWorkspace={() => void chooseWorkspace()} disabled={saving || !client?.connected} />
      </SettingsListToolbar>
      {!client?.connected && <p className="metadata">连接后台后可管理命令。</p>}
      {client?.connected && !file && !error && <p className="metadata">{scope === "workspace" && !workspace ? "请选择工作区。" : "正在读取…"}</p>}
      {file && <section className="settings-resource-group"><h3>已添加<span>{shown.length}</span></h3>
        {shown.length === 0 ? <SettingsEmpty>{query ? "没有匹配的命令" : "还没有命令，点击新建开始。"}</SettingsEmpty> :
          <div className="settings-resource-list">{shown.map((item) => <SettingsResourceRow key={item.name} icon={<Command />}
            name={item.name} description={item.description} disabled={saving} onClick={() => edit(item)} />)}</div>}
      </section>}
    </> : <>
      <SettingsHeader title={page === "create" ? "新建命令" : selectedName}
        description={scope === "user" ? "用户命令" : `工作区命令 · ${workspaceName}`}
        back={{ label: "命令", onClick: back, disabled: saving }} />
      {error && <div className="inline-notice" role="alert">{error}</div>}
      {file && draft && <>
        <fieldset className="settings-form-card" disabled={saving || !client?.connected}>
          {page === "create" && <div className="settings-field"><Label htmlFor="command-name">名称</Label>
            <Input id="command-name" placeholder="my-command" value={draft.name} disabled={saving}
              onChange={(event) => setDraft({ ...draft, name: event.target.value })} /></div>}
          <div className="settings-field"><Label htmlFor="command-description">描述（可选）</Label>
            <Input id="command-description" placeholder="这条命令用来做什么" value={draft.description ?? ""} disabled={saving}
              onChange={(event) => setDraft({ ...draft, description: event.target.value })} /></div>
          <div className="settings-field"><Label htmlFor="command-hint">参数提示（可选）</Label>
            <Input id="command-hint" placeholder="例如 <file-path>" value={draft.argumentHint ?? ""} disabled={saving}
              onChange={(event) => setDraft({ ...draft, argumentHint: event.target.value })} /></div>
          <div className="settings-field"><Label htmlFor="command-prompt">提示词</Label>
            <Textarea id="command-prompt" rows={9} value={draft.prompt}
              placeholder="调用命令时发送的提示词；$ARGUMENTS 表示命令后输入的参数。"
              disabled={saving} onChange={(event) => setDraft({ ...draft, prompt: event.target.value })} /></div>
        </fieldset>
        <div className="settings-form-actions">
          {page === "edit" && <Button size="sm" variant="ghost" className="settings-delete" disabled={saving}
            onClick={() => setDeleteName(selectedName)}><Trash2 />永久删除</Button>}
          <Button size="sm" variant="ghost" disabled={saving} onClick={back}>取消</Button>
          <Button size="sm" disabled={saving || !client?.connected || !dirty || !draft.name.trim() || !draft.prompt.trim()}
            onClick={() => void save()}>{saving ? "保存中…" : "保存"}</Button>
        </div>
      </>}
    </>}
    <AlertDialog open={!!deleteName} onOpenChange={(open) => { if (!open && !saving) setDeleteName(""); }}>
      <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>永久删除 {deleteName}？</AlertDialogTitle>
        <AlertDialogDescription>只删除当前作用域的这条命令，已有会话不受影响。</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel disabled={saving}>取消</AlertDialogCancel>
          <Button variant="destructive" disabled={saving} onClick={() => void remove()}>永久删除</Button>
        </AlertDialogFooter></AlertDialogContent>
    </AlertDialog>
    <AlertDialog open={discardOpen} onOpenChange={setDiscardOpen}>
      <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>放弃未保存的更改？</AlertDialogTitle>
        <AlertDialogDescription>当前命令的编辑内容将丢失。</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel>继续编辑</AlertDialogCancel>
          <Button variant="destructive" onClick={() => { setDiscardOpen(false); cancel(); }}>放弃更改</Button>
        </AlertDialogFooter></AlertDialogContent>
    </AlertDialog>
  </div>;
}
