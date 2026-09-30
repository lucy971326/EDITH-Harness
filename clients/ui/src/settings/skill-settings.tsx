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
import { ArrowLeft, BookOpenCheck, ChevronRight, Copy, Pencil, Plus, Trash2 } from "../icons";
import type { SettingsDraftState } from "./types";

type Mode = "overview" | "edit" | "new";

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
  const [mode, setMode] = useState<Mode>("overview");
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
    if (!client?.connected) { setLoading(false); return; }
    setLoading(true);
    void client.call("skill/settings/read", { workspace: currentWorkspace }).then((result) => {
      if (!active) return;
      setItems(result.items);
      setSelectedKey((key) => result.items.some((item) => itemKey(item) === key) ? key :
        result.items.length ? itemKey(result.items[0]) : "");
      setError("");
    }).catch((cause: unknown) => {
      if (active) setError(formatRPCError(cause, "Skill 列表读取失败"));
    }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [client, currentWorkspace, reload]);

  useEffect(() => {
    let active = true;
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
    setMode("overview");
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
    if (!client?.connected || saving || mode === "overview" || mode === "edit" && !document) return false;
    const name = mode === "new" ? newName.trim() : selected?.name ?? "";
    if (!name) { setError("填写 Skill 名称"); return false; }
    setSaving(true);
    setError("");
    try {
      const result = await client.call("skill/settings/save", {
        name, content: draft, version: mode === "new" ? "" : document?.version ?? "", create: mode === "new",
      });
      setDocument(result);
      setSelectedKey(`personal:${name}`);
      setMode("overview");
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
      setMode("overview");
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

  return <>
    <header className="settings-heading settings-split-heading">
      <h2>Skills</h2>
      <Button size="sm" variant="outline" disabled={!client?.connected || saving} onClick={startNew}><Plus />新建 Skill</Button>
    </header>
    {error && <div className="inline-notice" role="alert">{error}<Button size="sm" variant="ghost" onClick={() => {
      setReload((value) => value + 1);
      setDocumentReload((value) => value + 1);
    }}>重新读取</Button></div>}
    <div className="settings-two-pane skill-settings-page">
      <aside className="settings-subnav" aria-label="Skill 列表">
        <Input aria-label="搜索 Skill" placeholder="搜索 Skill" value={query} onChange={(event) => setQuery(event.target.value)} />
        {loading && <p className="metadata">正在读取…</p>}
        {!loading && !groups.length && <p className="metadata">{query ? "没有匹配的 Skill" : "还没有 Skill"}</p>}
        {groups.map((group) => <div className="skill-group" key={group.title}>
          <h3>{group.title} · {group.items.length}</h3>
          <div className="settings-subnav-list">{group.items.map((item) => <Button key={itemKey(item)} variant="ghost"
            className="settings-subnav-item" aria-pressed={mode !== "new" && selectedKey === itemKey(item)}
            onClick={() => request(() => show(itemKey(item)))}>
            <BookOpenCheck /><span className="settings-subnav-copy"><span className="settings-subnav-name">{item.name}</span>
              <span className="settings-subnav-meta">{statusLabel(item)}</span></span>
          </Button>)}</div>
        </div>)}
      </aside>
      <div className="settings-detail-pane">
        {mode === "new" ? <>
          <Button variant="ghost" size="sm" className="model-settings-back" onClick={() => request(() => show(selectedKey))}><ArrowLeft />返回</Button>
          <h3 className="model-settings-editor-title">新建 Skill</h3>
          <div className="settings-field"><label htmlFor="skill-name">名称</label><Input id="skill-name" value={newName}
            placeholder="例如 architecture-review" disabled={saving} onChange={(event) => {
              const next = event.target.value;
              setNewName(next);
              setDraft((old) => old.replace(/^name: .*$/m, `name: ${next || "skill-name"}`));
            }} /></div>
          <div className="settings-field skill-source-field"><label htmlFor="skill-source">SKILL.md</label>
            <Textarea id="skill-source" className="skill-source" spellCheck={false} value={draft} disabled={saving}
              onChange={(event) => setDraft(event.target.value)} /></div>
          <div className="settings-savebar"><span className="settings-save-status">未保存</span>
            <Button variant="ghost" size="sm" disabled={saving} onClick={() => show(selectedKey)}>放弃</Button>
            <Button size="sm" disabled={saving} onClick={() => void save()}>保存</Button></div>
        </> : selected && mode === "edit" ? <>
          <Button variant="ghost" size="sm" className="model-settings-back" onClick={() => request(() => show(selectedKey))}><ArrowLeft />返回概况</Button>
          <h3 className="model-settings-editor-title">编辑 · {selected.name}</h3>
          <div className="settings-field skill-source-field"><label htmlFor="skill-source">SKILL.md</label>
            <Textarea id="skill-source" className="skill-source" spellCheck={false} value={draft} disabled={saving}
              onChange={(event) => setDraft(event.target.value)} /></div>
          <div className="settings-savebar"><span className="settings-save-status">{dirty ? "未保存" : "所有更改已保存"}</span>
            <Button variant="ghost" size="sm" disabled={saving} onClick={() => show(selectedKey)}>放弃</Button>
            <Button size="sm" disabled={saving || !dirty} onClick={() => void save()}>保存</Button></div>
        </> : selected ? <>
          <div className="settings-detail-title-row"><div className="settings-identity">
            <span className="settings-identity-icon"><BookOpenCheck /></span>
            <div><h3>{selected.name}</h3>
              {(selected.error || selected.overridden || !selected.enabled) && <p className="metadata">{statusLabel(selected)}</p>}</div>
          </div>{personal && <Button variant="outline" size="sm" disabled={!document || saving}
            onClick={() => { setDraft(document?.content ?? ""); setMode("edit"); setError(""); }}><Pencil />编辑</Button>}</div>
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
        </> : !loading && <p className="metadata">选择一个 Skill 查看详情</p>}
      </div>
    </div>
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
  </>;
}
