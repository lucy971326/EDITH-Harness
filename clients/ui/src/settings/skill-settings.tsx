import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Hint } from "@/components/ui/tooltip";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
import {
  AlertDialog, AlertDialogContent, AlertDialogHeader, AlertDialogTitle,
  AlertDialogDescription, AlertDialogFooter, AlertDialogCancel,
} from "@/components/ui/alert-dialog";
import type { SkillDocument, SkillSettingsItem } from "../../../contracts/appserver";
import { MessageMarkdown } from "../chat/message-markdown";
import { RPCClient, formatRPCError } from "../client/rpc";
import { workspaceName } from "../state/projects";
import { BookOpenCheck, ChevronRight, Copy, Pencil, Plus, RefreshCw, Trash2 } from "../icons";
import type { SettingsDraftState } from "./types";
import { SettingsHeader, SettingsListToolbar, SettingsResourceRow, SettingsEmpty } from "./settings-primitives";

type Mode = "list" | "overview" | "edit" | "new";

function itemKey(item: SkillSettingsItem): string {
  return `${item.source}:${item.name}`;
}

function sourceLabel(item: SkillSettingsItem): string {
  switch (item.source) {
    case "personal": return "个人";
    case "project":
    case "project-agents": return "项目";
    case "agents": return ".agents 目录";
    case "system": return "内置";
  }
}

function statusLabel(item: SkillSettingsItem): string {
  if (item.error) return "配置错误";
  if (item.overridden) return "被同名 Skill 覆盖";
  if (!item.enabled) return "已关闭";
  return item.source === "personal" ? "已启用" : "只读";
}

function previewBody(content: string): string {
  return content.replace(/^---\r?\n[\s\S]*?\r?\n---\r?\n?/, "").trim();
}

function template(name: string): string {
  return `---\nname: ${name || "skill-name"}\ndescription: 简述这个 Skill 的用途。\n---\n\n# 使用说明\n\n在这里写使用步骤。\n`;
}

export function SkillSettingsPanel({ client, currentWorkspace, onStateChange, onChanged }: {
  client: RPCClient | null;
  currentWorkspace: string;
  onStateChange: (state: SettingsDraftState) => void;
  onChanged: () => void;
}) {
  const [items, setItems] = useState<SkillSettingsItem[]>([]);
  const [selectedKey, setSelectedKey] = useState("");
  const [document, setDocument] = useState<SkillDocument | null>(null);
  const [mode, setMode] = useState<Mode>("list");
  const [draft, setDraft] = useState("");
  const [newName, setNewName] = useState("");
  const [query, setQuery] = useState("");
  const [previewOpen, setPreviewOpen] = useState(false);
  const [resourcesOpen, setResourcesOpen] = useState(false);
  const [pending, setPending] = useState<(() => void) | null>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [reload, setReload] = useState(0);
  const [documentReload, setDocumentReload] = useState(0);
  const selected = items.find((item) => itemKey(item) === selectedKey);
  const dirty = mode === "new" || mode === "edit" && (!document || draft !== document.content);

  useEffect(() => { onStateChange({ dirty, saving }); }, [dirty, saving, onStateChange]);

  useEffect(() => {
    let active = true;
    if (mode === "edit" || mode === "new") return;
    if (!client?.connected) { setLoading(false); return; }
    setLoading(true);
    void client.call("skill/settings/read", { workspace: currentWorkspace }).then((result) => {
      if (!active) return;
      setItems(result.items);
      setSelectedKey((key) => result.items.some((item) => itemKey(item) === key) ? key : "");
      setError("");
    }).catch((cause: unknown) => {
      if (active) setError(formatRPCError(cause, "Skill 列表读取失败"));
    }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [client, currentWorkspace, reload]);

  useEffect(() => {
    let active = true;
    if (mode === "edit" || mode === "new") return;
    if (!client?.connected || !selected) return;
    void client.call("skill/settings/document", {
      workspace: currentWorkspace, source: selected.source, name: selected.name,
    }).then((result) => {
      if (active) setDocument(result);
    }).catch((cause: unknown) => {
      if (active) setError(formatRPCError(cause, "SKILL.md 读取失败"));
    });
    return () => { active = false; };
  }, [client, currentWorkspace, selectedKey, documentReload]);

  function show(key: string) {
    setSelectedKey(key);
    setMode(key ? "overview" : "list");
    if (key !== selectedKey) setDocument(null);
    setDraft("");
    setPreviewOpen(false);
    setResourcesOpen(false);
    setError("");
  }

  function request(action: () => void) {
    if (saving) return;
    if (dirty) { setPending(() => action); return; }
    action();
  }

  function startNew() {
    request(() => {
      setMode("new");
      setNewName("");
      setDraft(template(""));
      setError("");
    });
  }

  async function refresh(): Promise<void> {
    setReload((value) => value + 1);
    onChanged();
  }

  async function save(): Promise<boolean> {
    if (!client?.connected || saving || (mode !== "new" && mode !== "edit") || mode === "edit" && !document) return false;
    const name = mode === "new" ? newName.trim() : selected?.name ?? "";
    if (!name) { setError("填写 Skill 名称"); return false; }
    setSaving(true);
    setError("");
    try {
      const result = await client.call("skill/settings/save", {
        name, content: draft, version: mode === "new" ? "" : document?.version ?? "", create: mode === "new",
      });
      setDocument(result);
      setSelectedKey("");
      setMode("list");
      setPreviewOpen(false);
      await refresh();
      return true;
    } catch (cause) {
      setError(formatRPCError(cause, "Skill 保存失败"));
      return false;
    } finally { setSaving(false); }
  }

  async function toggle(enabled: boolean) {
    if (!client?.connected || !selected || selected.source !== "personal" || saving) return;
    setSaving(true);
    setError("");
    try {
      await client.call("skill/settings/toggle", { name: selected.name, enabled });
      await refresh();
    } catch (cause) { setError(formatRPCError(cause, "Skill 开关保存失败")); }
    finally { setSaving(false); }
  }

  async function remove() {
    if (!client?.connected || !selected || selected.source !== "personal" || !document || saving) return;
    setSaving(true);
    setError("");
    try {
      await client.call("skill/settings/delete", { name: selected.name, version: document.version });
      setDeleteOpen(false);
      setDocument(null);
      setMode("list");
      setSelectedKey("");
      await refresh();
    } catch (cause) {
      setError(formatRPCError(cause, "Skill 删除失败"));
      await refresh();
    }
    finally { setSaving(false); }
  }

  const filtered = items.filter((item) => `${item.name} ${item.description}`.toLowerCase().includes(query.toLowerCase()));
  const groups = [
    { title: "个人", items: filtered.filter((item) => item.source === "personal") },
    { title: `项目 · ${workspaceName(currentWorkspace)}`, items: filtered.filter((item) => item.source === "project" || item.source === "project-agents") },
    { title: ".agents", items: filtered.filter((item) => item.source === "agents") },
    { title: "系统", items: filtered.filter((item) => item.source === "system") },
  ].filter((group) => group.items.length);
  const personal = selected?.source === "personal";

  return <div className="skill-settings-page">
    <SettingsHeader title={mode === "list" ? "Skills" : mode === "new" ? "新建 Skill" : selected?.name ?? "Skill"}
      description={mode === "list" ? "管理 Agent 可以使用的技能与工作流程。" : mode === "new" ? "创建个人 Skill，保存后可在聊天中使用。" : undefined}
      back={mode !== "list" ? { label: mode === "edit" ? selected?.name ?? "Skill" : "Skills",
        onClick: () => request(() => show(mode === "edit" ? selectedKey : "")), disabled: saving } : undefined}
      actions={mode === "list" ? <Button size="sm" disabled={!client?.connected || saving} onClick={startNew}><Plus />新建 Skill</Button> :
        mode === "overview" && personal ? <Button variant="outline" size="sm" disabled={!document || saving || !client?.connected}
          onClick={() => { setDraft(document?.content ?? ""); setMode("edit"); setError(""); }}><Pencil />编辑</Button> : undefined} />
    {error && <div className="inline-notice" role="alert">{error}<Button size="sm" variant="ghost" disabled={dirty || saving || !client?.connected} onClick={() => {
      setReload((value) => value + 1);
      setDocumentReload((value) => value + 1);
    }}>重新读取</Button></div>}
    {mode === "list" ? <>
        <SettingsListToolbar query={query} onQueryChange={setQuery} placeholder="搜索 Skill"
          actions={<Hint text="刷新 Skills"><Button size="icon-sm" variant="ghost" aria-label="刷新 Skills" disabled={loading || !client?.connected}
            onClick={() => void refresh()}><RefreshCw /></Button></Hint>}>
          <span className="settings-description">{items.length} 个 Skill</span>
        </SettingsListToolbar>
        {loading && <p className="metadata">正在读取…</p>}
        {!client?.connected && <p className="inline-notice">连接后台后可管理 Skills。</p>}
        {!loading && !groups.length && !error && <SettingsEmpty>{query ? "没有匹配的 Skill" : "还没有 Skill，点击新建开始。"}</SettingsEmpty>}
        {groups.map((group) => <section className="settings-resource-group" key={group.title}>
          <h3>{group.title}<span>{group.items.length}</span></h3>
          <div className="settings-resource-list">{group.items.map((item) => <SettingsResourceRow key={itemKey(item)}
            icon={<BookOpenCheck />} name={item.name} description={item.description} meta={statusLabel(item)}
            disabled={saving} onClick={() => request(() => show(itemKey(item)))} />)}</div>
        </section>)}
    </> : <div className="settings-editor">
        {mode === "new" ? <>
          <div className="settings-field"><label htmlFor="skill-name">名称</label><Input id="skill-name" value={newName}
            placeholder="例如 architecture-review" disabled={saving} onChange={(event) => {
              const next = event.target.value;
              setNewName(next);
              setDraft((old) => old.replace(/^name: .*$/m, `name: ${next || "skill-name"}`));
            }} /></div>
          <div className="settings-field skill-source-field"><label htmlFor="skill-source">SKILL.md</label>
            <Textarea id="skill-source" className="skill-source" spellCheck={false} value={draft} disabled={saving}
              onChange={(event) => setDraft(event.target.value)} /></div>
          <div className="settings-savebar">
            <Button variant="ghost" size="sm" disabled={saving} onClick={() => request(() => show(""))}>取消</Button>
            <Button size="sm" disabled={saving || !client?.connected || !newName.trim()} onClick={() => void save()}>{saving ? "保存中…" : "保存"}</Button></div>
        </> : selected && mode === "edit" ? <>
          <div className="settings-field skill-source-field"><label htmlFor="skill-source">SKILL.md</label>
            <Textarea id="skill-source" className="skill-source" spellCheck={false} value={draft} disabled={saving}
              onChange={(event) => setDraft(event.target.value)} /></div>
          <div className="settings-savebar">
            <Button variant="ghost" size="sm" disabled={saving} onClick={() => request(() => show(selectedKey))}>取消</Button>
            <Button size="sm" disabled={saving || !dirty || !client?.connected} onClick={() => void save()}>{saving ? "保存中…" : "保存"}</Button></div>
        </> : selected ? <>
          {(selected.error || selected.overridden || !selected.enabled) && <p className="settings-notice">{statusLabel(selected)}</p>}
          <section className="settings-section"><div className="settings-section-header"><h3>概况</h3></div>
            {selected.description && <Hint text={selected.description}><p className="skill-description">{selected.description}</p></Hint>}
            {selected.error && <p className="skill-error">{selected.error}</p>}
            <div className="skill-facts"><span>来源</span><strong>{sourceLabel(selected)}</strong>
              <span>文件</span><div className="skill-path"><code>{selected.path}</code>
                <Hint text="复制文件位置"><Button variant="ghost" size="icon-sm" aria-label="复制文件位置"
                  onClick={() => void navigator.clipboard.writeText(selected.path).catch(() => setError("复制失败"))}><Copy /></Button></Hint></div></div>
          </section>
          <section className="settings-section"><button type="button" className="skill-preview-trigger ui-focus"
            aria-expanded={previewOpen} onClick={() => setPreviewOpen(!previewOpen)}>
            <ChevronRight className={previewOpen ? "skill-chevron-open" : ""} /><span>正文预览</span>
            <span className="metadata">SKILL.md</span>
          </button>
            {previewOpen && <div className="skill-preview">
              {!document ? <p className="metadata">正在读取…</p> : previewBody(document.content) ?
                <MessageMarkdown text={previewBody(document.content)} workspace={currentWorkspace} /> :
                <p className="metadata">没有正文</p>}
            </div>}
          </section>
          {!!document?.resources.length && <section className="settings-section"><button type="button" className="skill-preview-trigger ui-focus"
            aria-expanded={resourcesOpen} onClick={() => setResourcesOpen(!resourcesOpen)}>
            <ChevronRight className={resourcesOpen ? "skill-chevron-open" : ""} /><span>附属资源</span>
            <span className="metadata">{document.resources.length} 项</span>
          </button>{resourcesOpen && <div className="skill-resources">{document.resources.map((name) => <code key={name}>{name}</code>)}</div>}</section>}
          {personal && <section className="settings-section skill-management">
            <div className="skill-toggle"><span>启用</span><Switch checked={selected.enabled} disabled={saving}
              aria-label={`启用 ${selected.name}`} onCheckedChange={(checked) => void toggle(checked)} /></div>
            <Button variant="ghost" size="sm" className="settings-delete" disabled={!document || saving}
              onClick={() => setDeleteOpen(true)}><Trash2 />永久删除</Button>
          </section>}
        </> : !loading && <SettingsEmpty>这个 Skill 已不可用，请返回列表刷新。</SettingsEmpty>}
    </div>}
    <AlertDialog open={!!pending} onOpenChange={(open) => { if (!open) setPending(null); }}><AlertDialogContent>
      <AlertDialogHeader><AlertDialogTitle>保存当前修改？</AlertDialogTitle>
        <AlertDialogDescription>还有未保存的 SKILL.md。</AlertDialogDescription></AlertDialogHeader>
      <AlertDialogFooter><AlertDialogCancel>继续编辑</AlertDialogCancel>
        <Button variant="outline" disabled={saving} onClick={() => { const action = pending; setPending(null); action?.(); }}>放弃</Button>
        <Button disabled={saving} onClick={() => void save().then((ok) => { if (ok) { const action = pending; setPending(null); action?.(); } })}>保存</Button>
      </AlertDialogFooter>
    </AlertDialogContent></AlertDialog>
    <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}><AlertDialogContent>
      <AlertDialogHeader><AlertDialogTitle>永久删除 {selected?.name}？</AlertDialogTitle>
        <AlertDialogDescription>将删除整个个人 Skill 文件夹及其附属资源。</AlertDialogDescription></AlertDialogHeader>
      <AlertDialogFooter><AlertDialogCancel disabled={saving}>取消</AlertDialogCancel>
        <Button variant="destructive" disabled={saving} onClick={() => void remove()}>永久删除</Button>
      </AlertDialogFooter>
    </AlertDialogContent></AlertDialog>
  </div>;
}
