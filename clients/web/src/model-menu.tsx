import { useState } from "react";
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
}: {
  models: ModelChoice[] | null;
  value: ModelSelection;
  disabled: boolean;
  error: string;
  onChange: (value: ModelSelection) => void;
  onRetry: () => void;
}) {
  const [open, setOpen] = useState(false);
  const model = models?.find((item) => item.id === value.model);
  return (
    <Popover open={open && !disabled} onOpenChange={setOpen}>
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
              aria-pressed={value.model === item.id}
              onClick={() =>
                onChange({
                  model: item.id,
                  reasoningEffort:
                    item.id === value.model ? value.reasoningEffort : "",
                })
              }
            >
              <strong>{item.id}</strong>
              {value.model === item.id && <Check />}
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
                    value.reasoningEffort === effort ? "default" : "outline"
                  }
                  aria-pressed={value.reasoningEffort === effort}
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
