import {
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
  type RefObject,
} from "react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { ArrowUp, BookOpen, Command, Square, X, Plus } from "./icons";
import { ModelMenu, type ModelSelection } from "./model-menu";
import { AgentMenu } from "./agent-menu";
import type { ModelChoice } from "../../contracts/appserver.ts";
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
  prefix: "/" | "$";
  query: string;
  start: number;
  end: number;
}

type Suggestion =
  | { kind: "command"; name: string; description: string }
  | { kind: "skill"; name: string; description: string; scope: string };

// 候选只识别光标前最后一个独立的 / 或 $ 词段。
export function composerTrigger(
  value: string,
  cursor: number,
): ComposerTrigger | null {
  const beforeCursor = value.slice(0, cursor);
  const match = /(?:^|\s)([/$])([^\s]*)$/.exec(beforeCursor);
  if (!match) return null;
  const remaining = value.slice(cursor);
  const nextWhitespace = remaining.search(/\s/);
  return {
    prefix: match[1] as "/" | "$",
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
  agentLabel,
  agents,
  agentID,
  settingsDisabled,
  usage,
  compressingImages,
  running,
  stopping,
  busySending,
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
}: {
  draft: string;
  images: Attachment[];
  notice: string;
  agentLabel: string;
  agents: AgentView[] | null;
  agentID: string;
  settingsDisabled: boolean;
  usage?: {
    inputTokens: number;
    cacheReadTokens: number;
    contextWindow: number;
  };
  compressingImages: boolean;
  running: boolean;
  stopping: boolean;
  busySending: boolean;
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
}) {
  const input = useRef<HTMLTextAreaElement>(null);
  const imageInput = useRef<HTMLInputElement>(null);
  const [cursor, setCursor] = useState(draft.length);
  const [activeSuggestion, setActiveSuggestion] = useState(0);
  const [dismissedTrigger, setDismissedTrigger] = useState("");
  const trigger = composerTrigger(draft, Math.min(cursor, draft.length));
  const triggerKey = trigger ? `${draft}\u0000${cursor}` : "";
  const suggestions = useMemo(() => {
    if (!trigger || suggestionsDisabled) return [];
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
  }, [commandBusy, commands, running, skills, suggestionsDisabled, trigger]);
  const showSuggestions =
    suggestions.length > 0 && triggerKey !== dismissedTrigger;

  useEffect(() => {
    setActiveSuggestion(0);
  }, [triggerKey, suggestions.length]);
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
    await onCommand(item.name, {
      draft,
      start: trigger.start,
      end: trigger.end,
    });
  }

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
            <div className="suggestions" role="listbox" aria-label="输入候选">
              {suggestions.map((item, index) => (
                <button
                  key={`${item.kind}:${item.name}`}
                  type="button"
                  role="option"
                  aria-selected={index === activeSuggestion}
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={() => void selectSuggestion(item)}
                >
                  {item.kind === "command" ? <Command /> : <BookOpen />}
                  <span>
                    <strong>
                      {item.kind === "command" ? "/" : "$"}
                      {item.name}
                    </strong>
                    <small>{item.description}</small>
                  </span>
                  {item.kind === "skill" && <small>{item.scope}</small>}
                </button>
              ))}
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
            onKeyDown={(event) => {
              if (showSuggestions) {
                if (event.key === "ArrowDown" || event.key === "ArrowUp") {
                  event.preventDefault();
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
                  void selectSuggestion(
                    suggestions[activeSuggestion] ?? suggestions[0],
                  );
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
                    aria-label="最近一次模型调用的上下文用量"
                  >
                    {usage
                      ? `${usage.inputTokens + usage.cacheReadTokens}/${usage.contextWindow}`
                      : "—"}
                  </span>
                </TooltipTrigger>
                <TooltipContent>
                  最近一次模型调用：输入与缓存{" "}
                  {usage ? usage.inputTokens + usage.cacheReadTokens : "暂无"}{" "}
                  tokens
                </TooltipContent>
              </Tooltip>
              <ModelMenu
                models={models}
                value={modelSelection}
                disabled={modelDisabled}
                error={modelError}
                onRetry={onRetryModels}
                onChange={onModelChange}
                requiresVision={images.length > 0}
              />
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
        <div className="composer-caption">
          <span>
            {stopping
              ? "停止中，等待后台收尾…"
              : compressingImages
                ? "正在压缩图片…"
                : busySending
                  ? "等待后台确认…"
                  : running
                    ? "Enter 调整当前任务 · 不排队"
                    : "Enter 发送 · Shift + Enter 换行"}
          </span>
          <span>
            {modelError || (models?.length === 0 ? "尚未配置模型" : agentLabel)}
          </span>
        </div>
      </div>
    </div>
  );
}
