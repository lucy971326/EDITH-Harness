import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Check, ChevronDown } from "./icons";
import type { ModelChoice } from "../../contracts/appserver.ts";

export interface ModelSelection {
  model: string;
  reasoningEffort: string;
}

export function ModelMenu({
  models,
  value,
  disabled,
  error,
  onChange,
  onRetry,
  requiresVision,
}: {
  models: ModelChoice[] | null;
  value: ModelSelection;
  disabled: boolean;
  error: string;
  onChange: (value: ModelSelection) => void;
  onRetry: () => void;
  requiresVision: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [pendingModel, setPendingModel] = useState(value.model);
  useEffect(() => setPendingModel(value.model), [value.model]);
  const model = models?.find((item) => item.id === pendingModel);
  return (
    <Popover
      open={open && !disabled}
      onOpenChange={(next) => {
        setOpen(next);
        if (next) setPendingModel(value.model);
      }}
    >
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="model-trigger"
          disabled={disabled}
          aria-label="模型与思考"
        >
          <span className="model-name">{value.model || "选择模型"}</span>
          <span className="muted">· {value.reasoningEffort || "思考"}</span>
          <ChevronDown />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="model-popover" align="end" side="top">
        <h3>模型与思考</h3>
        <p className="metadata">用于下一轮。运行中不能修改。</p>
        {error ? (
          <div role="status">
            <p className="inline-error">{error}</p>
            <Button variant="ghost" onClick={onRetry}>
              重新加载模型
            </Button>
          </div>
        ) : models === null ? (
          <p className="metadata">正在加载模型…</p>
        ) : models.length === 0 ? (
          <p className="metadata">尚未配置模型，请先配置后台 Provider。</p>
        ) : null}
        <div className="model-options" role="group" aria-label="模型">
          {models?.map((item) => (
            <button
              key={item.id}
              aria-pressed={pendingModel === item.id}
              disabled={requiresVision && !item.vision}
              title={
                requiresVision && !item.vision
                  ? "当前图片需要视觉模型"
                  : undefined
              }
              onClick={() => setPendingModel(item.id)}
            >
              <strong>{item.id}</strong>
              {pendingModel === item.id && <Check />}
            </button>
          ))}
        </div>
        {model && (
          <>
            <h3>思考档位</h3>
            <div className="effort-options" role="group" aria-label="思考档位">
              {model.reasoningEfforts.map((effort) => (
                <Button
                  key={effort}
                  size="sm"
                  variant={
                    pendingModel === value.model &&
                    value.reasoningEffort === effort
                      ? "default"
                      : "outline"
                  }
                  aria-pressed={
                    pendingModel === value.model &&
                    value.reasoningEffort === effort
                  }
                  onClick={() => {
                    onChange({ model: model.id, reasoningEffort: effort });
                    setOpen(false);
                  }}
                >
                  {effort}
                </Button>
              ))}
            </div>
          </>
        )}
      </PopoverContent>
    </Popover>
  );
}
