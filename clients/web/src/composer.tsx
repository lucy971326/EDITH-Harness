import { useImperativeHandle, useRef, type RefObject } from "react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { ArrowUp, Square, X, Plus } from "./icons";
import { ModelMenu, type ModelSelection } from "./model-menu";
import { AgentMenu } from "./agent-menu";
import type { ModelChoice } from "../../contracts/appserver.ts";
import type { AgentView } from "../../contracts/appserver.ts";

export type Attachment = {
  id: string;
  name: string;
  url: string;
  mime: "image/webp";
  data: string;
};
export type ComposerHandle = { focus: () => void };

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
  onDraftChange,
  onSend,
  onStop,
  onAddImages,
  onRemoveImage,
  onModelChange,
  onAgentChange,
  onRetryModels,
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
  onDraftChange: (text: string) => void;
  onSend: () => void;
  onStop: () => void;
  onAddImages: (files: FileList | File[] | null) => void;
  onRemoveImage: (id: string) => void;
  onModelChange: (value: ModelSelection) => void;
  onAgentChange: (agentID: string) => void;
  onRetryModels: () => void;
  onDismissNotice: () => void;
  composerRef?: RefObject<ComposerHandle | null>;
}) {
  const input = useRef<HTMLTextAreaElement>(null);
  const imageInput = useRef<HTMLInputElement>(null);
  useImperativeHandle(composerRef, () => ({
    focus() {
      input.current?.focus();
    },
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
            onChange={(event) => onDraftChange(event.target.value)}
            onKeyDown={(event) => {
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
