import {
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type RefObject,
} from "react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { ArrowUp, BookOpen, Command, Square, X, Plus, FileText, Folder } from "./icons";
import type { ContextReference, ReferenceAttachment } from "./context-references";
import { ContextReferenceTag } from "./context-reference-tags";
import { ModelMenu, type ModelSelection } from "./model-menu";
import { PermissionMenu } from "./permission-menu";
import type { PermissionModeChoice } from "../../contracts/approvals.ts";
import type { PermissionMode } from "../../contracts/harness.ts";
import { AgentMenu } from "./agent-menu";
import type { ModelChoice, PathSearchResult } from "../../contracts/appserver.ts";
import type {
  AgentView,
  CommandView,
  SkillView,
} from "../../contracts/appserver.ts";

export type Attachment = {
  id: string;
  name: string;
  url: string;
  mime: "image/webp";
  data: string;
};
export type ComposerHandle = { focus: () => void };
export type CommandSelection = {
  draft: string;
  start: number;
  end: number;
};

export interface ComposerTrigger {
  prefix: "/" | "$" | "@";
  query: string;
  start: number;
  end: number;
}

type Suggestion =
  | { kind: "command"; name: string; description: string }
  | { kind: "skill"; name: string; description: string; scope: string }
  | { kind: "path"; name: string; description: string; reference: ContextReference };

// 候选只识别光标前最后一个独立的 /、$ 或 @ 词段。
export function composerTrigger(
  value: string,
  cursor: number,
): ComposerTrigger | null {
  const beforeCursor = value.slice(0, cursor);
  const match = /(?:^|\s)([/$@])([^\s]*)$/.exec(beforeCursor);
  if (!match) return null;
  const remaining = value.slice(cursor);
  const nextWhitespace = remaining.search(/\s/);
  return {
    prefix: match[1] as "/" | "$" | "@",
    query: match[2].toLowerCase(),
    start: cursor - match[2].length - 1,
    end: nextWhitespace < 0 ? value.length : cursor + nextWhitespace,
  };
}

function scopeLabel(scope: SkillView["scope"]): string {
  if (scope === "system") return "系统";
  if (scope === "user") return "个人";
  return "项目";
}

export function isComposerSubmitKey(event: {
  key: string;
  shiftKey: boolean;
  keyCode: number;
  nativeEvent: { isComposing: boolean };
}): boolean {
  return (
    event.key === "Enter" &&
    !event.shiftKey &&
    !event.nativeEvent.isComposing &&
    event.keyCode !== 229
  );
}

export function Composer({
  draft,
  images,
  notice,
  agents,
  agentID,
  settingsDisabled,
  permissionMode,
  permissionModes = [],
  onPermissionChange,
  usage,
  running,
  stopping,
  canSend,
  stopDisabled,
  modelDisabled,
  validModel,
  models,
  modelSelection,
  modelError,
  imageDisabled,
  skills,
  commands,
  suggestionsDisabled,
  commandBusy,
  onDraftChange,
  onSend,
  onStop,
  onAddImages,
  onRemoveImage,
  onModelChange,
  onAgentChange,
  onRetryModels,
  onCommand,
  onDismissNotice,
  composerRef,
  references = [],
  referenceWorkspace,
  onSearchPaths,
  onAddReference,
  onRemoveReference,
}: {
  draft: string;
  images: Attachment[];
  notice: string;
  agents: AgentView[] | null;
  agentID: string;
  settingsDisabled: boolean;
  permissionMode?: PermissionMode;
  permissionModes?: PermissionModeChoice[];
  onPermissionChange?: (mode: PermissionMode) => void;
  usage?: {
    inputTokens: number;
    cacheReadTokens: number;
    contextWindow: number;
  };
  running: boolean;
  stopping: boolean;
  canSend: boolean;
  stopDisabled: boolean;
  modelDisabled: boolean;
  validModel: boolean;
  models: ModelChoice[] | null;
  modelSelection: ModelSelection;
  modelError: string;
  imageDisabled: boolean;
  skills: SkillView[];
  commands: CommandView[];
  suggestionsDisabled: boolean;
  commandBusy: boolean;
  onDraftChange: (text: string) => void;
  onSend: () => void;
  onStop: () => void;
  onAddImages: (files: FileList | File[] | null) => void;
  onRemoveImage: (id: string) => void;
  onModelChange: (value: ModelSelection) => void;
  onAgentChange: (agentID: string) => void;
  onRetryModels: () => void;
  onCommand: (name: string, selection: CommandSelection) => Promise<void>;
  onDismissNotice: () => void;
  composerRef?: RefObject<ComposerHandle | null>;
  references?: ReferenceAttachment[];
  referenceWorkspace?: string;
  onSearchPaths?: (workspace: string, query: string) => Promise<PathSearchResult>;
  onAddReference?: (reference: ContextReference) => void;
  onRemoveReference?: (id: string) => void;
}) {
  const input = useRef<HTMLTextAreaElement>(null);
  const imageInput = useRef<HTMLInputElement>(null);
  const suggestionList = useRef<HTMLDivElement>(null);
  const [cursor, setCursor] = useState(draft.length);
  const [activeSuggestion, setActiveSuggestion] = useState(0);
  const [dismissedTrigger, setDismissedTrigger] = useState("");
  const [composing, setComposing] = useState(false);
  const [pathSearch, setPathSearch] = useState<(PathSearchResult & {
    key: string;
    loading: boolean;
    error: string;
  }) | null>(null);
  const trigger = composerTrigger(draft, Math.min(cursor, draft.length));
  const triggerKey = trigger ? `${draft}\u0000${cursor}` : "";
  const searchQuery = trigger?.prefix === "@" ? trigger.query : null;
  const searchKey = JSON.stringify([referenceWorkspace, searchQuery]);
  const showPathSuggestions = searchQuery !== null && !!referenceWorkspace && !!onAddReference &&
    !suggestionsDisabled && !composing && triggerKey !== dismissedTrigger;
  const search = pathSearch?.key === searchKey ? pathSearch : null;
  useEffect(() => {
    if (!showPathSuggestions || !onSearchPaths || !referenceWorkspace || searchQuery === null)
      return;
    let current = true;
    setPathSearch({ key: searchKey, loading: true, error: "", entries: [], truncated: false });
    const timer = setTimeout(() => {
      void onSearchPaths(referenceWorkspace, searchQuery).then((result) => {
        if (current) setPathSearch({ ...result, key: searchKey, loading: false, error: "" });
      }).catch(() => {
        if (current) setPathSearch({
          key: searchKey, loading: false, entries: [], truncated: false,
          error: "文件搜索失败，请修改搜索词重试；也可从文件树右键添加。",
        });
      });
    }, 200);
    return () => {
      current = false;
      clearTimeout(timer);
    };
  }, [showPathSuggestions, onSearchPaths, referenceWorkspace, searchQuery, searchKey]);
  const suggestions = useMemo(() => {
    if (!trigger || suggestionsDisabled) return [];
    if (trigger.prefix === "@") {
      if (!showPathSuggestions) return [];
      return (search?.entries ?? []).map((entry): Suggestion => ({
        kind: "path", name: entry.path,
        description: entry.kind === "directory" ? "目录" : "文件",
        reference: { kind: entry.kind, path: entry.path },
      }));
    }
    const matches = (name: string) =>
      name.toLowerCase().includes(trigger.query);
    const items: Suggestion[] = [];
    if (trigger.prefix === "/" && !running && !commandBusy) {
      for (const command of commands) {
        if (matches(command.name)) items.push({ kind: "command", ...command });
      }
    }
    for (const skill of skills) {
      if (matches(skill.name))
        items.push({
          kind: "skill",
          name: skill.name,
          description: skill.description,
          scope: scopeLabel(skill.scope),
        });
    }
    return items;
  }, [commandBusy, commands, running, skills, suggestionsDisabled, trigger, showPathSuggestions, search]);
  const showSuggestions =
    !composing && (suggestions.length > 0 || showPathSuggestions) && triggerKey !== dismissedTrigger;
  const usedTokens = usage
    ? usage.inputTokens + usage.cacheReadTokens
    : 0;
  const usagePercent = usage?.contextWindow
    ? Math.min(100, (usedTokens / usage.contextWindow) * 100)
    : 0;
  const usageLabel = usage
    ? `${Math.round(usagePercent * 10) / 10}%`
    : "暂无";

  useEffect(() => {
    setActiveSuggestion(0);
  }, [triggerKey, suggestions.length]);
  useEffect(() => {
    suggestionList.current?.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: "nearest" });
  }, [activeSuggestion]);
  useImperativeHandle(composerRef, () => ({
    focus() {
      input.current?.focus();
    },
  }));

  function replaceTrigger(value: string, current: ComposerTrigger) {
    const next = `${draft.slice(0, current.start)}${value}${draft.slice(current.end)}`;
    const nextCursor = current.start + value.length;
    onDraftChange(next);
    setCursor(nextCursor);
    setDismissedTrigger("");
    requestAnimationFrame(() => {
      input.current?.focus();
      input.current?.setSelectionRange(nextCursor, nextCursor);
    });
  }

  async function selectSuggestion(item: Suggestion) {
    if (!trigger || commandBusy) return;
    setDismissedTrigger(triggerKey);
    if (item.kind === "skill") {
      replaceTrigger(`$${item.name} `, trigger);
      return;
    }
    if (item.kind === "path") {
      onAddReference?.(item.reference);
      replaceTrigger("", trigger);
      return;
    }
    await onCommand(item.name, {
      draft,
      start: trigger.start,
      end: trigger.end,
    });
  }

  let assistantReferenceNumber = 0;
  const numberedReferences = references.map((item) => ({
    ...item,
    assistantNumber: item.reference.kind === "assistant-selection"
      ? ++assistantReferenceNumber : undefined,
  }));

  return (
    <div className="composer-area">
      <div className="composer-column">
        {notice && (
          <div role="status" className="inline-notice">
            {notice}
            <button aria-label="关闭提示" onClick={onDismissNotice}>
              <X />
            </button>
          </div>
        )}
        <div className="composer">
          {showSuggestions && (
            <div ref={suggestionList} className="suggestions" role="listbox" aria-label="输入候选">
              {suggestions.map((item, index) => (
                <button
                  key={`${item.kind}:${item.name}`}
                  type="button"
                  role="option"
                  aria-selected={index === activeSuggestion}
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={() => void selectSuggestion(item)}
                >
                  {item.kind === "command" ? <Command /> : item.kind === "skill" ? <BookOpen /> :
                    item.reference.kind === "directory" ? <Folder /> : <FileText />}
                  <span>
                    <strong>
                      {item.kind === "command" ? "/" : item.kind === "skill" ? "$" : ""}
                      {item.name}
                    </strong>
                    <small>{item.description}</small>
                  </span>
                  {item.kind === "skill" && <small>{item.scope}</small>}
                </button>
              ))}
              {showPathSuggestions && (!search || search.loading || search.error ||
                search.entries.length === 0 || search.truncated) &&
                <div className="suggestion-status" role="status">
                  {!search || search.loading ? "正在搜索…" : search.error ||
                    (search.entries.length === 0 ? "没有匹配的文件或目录" :
                      "仅显示前 50 项，请输入更具体的路径")}
                </div>}
            </div>
          )}
          {images.length > 0 && (
            <div className="attachments">
              {images.map((image) => (
                <div key={image.id}>
                  <img src={image.url} alt={image.name} />
                  <button
                    aria-label={`移除图片 ${image.name}`}
                    onClick={() => onRemoveImage(image.id)}
                  >
                    <X />
                  </button>
                </div>
              ))}
            </div>
          )}
          {references.length > 0 && <div className="context-references" aria-label="待发送引用">
            {numberedReferences.map((item) => <ContextReferenceTag key={item.id} reference={item.reference}
              assistantNumber={item.assistantNumber}
              onRemove={() => onRemoveReference?.(item.id)} />)}
          </div>}
          <Textarea
            ref={input}
            aria-label="消息输入"
            placeholder={running ? "发送以调整当前任务" : "说说你的想法"}
            value={draft}
            onChange={(event) => {
              setCursor(event.target.selectionStart);
              setDismissedTrigger("");
              onDraftChange(event.target.value);
            }}
            onSelect={(event) => setCursor(event.currentTarget.selectionStart)}
            onCompositionStart={() => setComposing(true)}
            onCompositionEnd={(event) => {
              setComposing(false);
              setCursor(event.currentTarget.selectionStart);
            }}
            onKeyDown={(event) => {
              if (composing || event.nativeEvent.isComposing || event.keyCode === 229) return;
              if (showSuggestions) {
                if (event.key === "ArrowDown" || event.key === "ArrowUp") {
                  event.preventDefault();
                  if (suggestions.length === 0) return;
                  const direction = event.key === "ArrowDown" ? 1 : -1;
                  setActiveSuggestion(
                    (activeSuggestion + direction + suggestions.length) %
                      suggestions.length,
                  );
                  return;
                }
                if (event.key === "Escape") {
                  event.preventDefault();
                  setDismissedTrigger(triggerKey);
                  return;
                }
                if (isComposerSubmitKey(event)) {
                  event.preventDefault();
                  const item = suggestions[activeSuggestion] ?? suggestions[0];
                  if (item) void selectSuggestion(item);
                  return;
                }
              }
              if (isComposerSubmitKey(event)) {
                event.preventDefault();
                onSend();
              }
            }}
            onPaste={(event) => {
              const files = Array.from(event.clipboardData.files);
              if (files.length) {
                event.preventDefault();
                onAddImages(files);
              }
            }}
          />
          <div className="composer-toolbar">
            <div className="composer-left">
              <input
                ref={imageInput}
                type="file"
                hidden
                accept="image/png,image/jpeg,image/webp,image/gif"
                multiple
                onChange={(event) => {
                  onAddImages(event.target.files);
                  event.target.value = "";
                }}
              />
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="添加图片"
                    disabled={imageDisabled}
                    onClick={() => imageInput.current?.click()}
                  >
                    <Plus />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>
                  {imageDisabled
                    ? "请先选择支持图片的模型"
                    : "添加图片，也可以粘贴"}
                </TooltipContent>
              </Tooltip>
              {permissionMode && onPermissionChange && (
                <PermissionMenu modes={permissionModes} value={permissionMode} disabled={settingsDisabled}
                  onChange={onPermissionChange} />
              )}
              <AgentMenu
                agents={agents}
                value={agentID}
                disabled={settingsDisabled}
                onChange={onAgentChange}
              />
            </div>
            <div className="composer-right">
              <Tooltip>
                <TooltipTrigger asChild>
                  <span
                    className="usage"
                    aria-label={`上下文已使用 ${usageLabel}`}
                  >
                    <span
                      className="usage-ring"
                      style={
                        {
                          "--usage-percent": `${usagePercent}%`,
                        } as CSSProperties
                      }
                    />
                  </span>
                </TooltipTrigger>
                <TooltipContent>上下文已使用 {usageLabel}</TooltipContent>
              </Tooltip>
              <div className="composer-model">
                <ModelMenu
                  models={models}
                  value={modelSelection}
                  disabled={modelDisabled}
                  error={modelError}
                  onRetry={onRetryModels}
                  onChange={onModelChange}
                  requiresVision={images.length > 0}
                />
              </div>
              {running && (
                <Button
                  variant="outline"
                  size="icon"
                  aria-label={stopping ? "停止中" : "停止任务"}
                  disabled={stopDisabled}
                  onClick={onStop}
                >
                  <Square className="stop-icon" />
                </Button>
              )}
              <Tooltip>
                <TooltipTrigger asChild>
                  <span>
                    <Button
                      size="icon"
                      className="send-button"
                      aria-label={running ? "调整当前任务" : "发送消息"}
                      disabled={!canSend}
                      onClick={onSend}
                    >
                      <ArrowUp />
                    </Button>
                  </span>
                </TooltipTrigger>
                <TooltipContent>
                  {running
                    ? "直接插话，不排队"
                    : validModel
                      ? "发送消息"
                      : "请先选择模型和思考档位"}
                </TooltipContent>
              </Tooltip>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
