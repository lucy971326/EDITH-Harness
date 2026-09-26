import { useEffect, useState } from "react";
import { Slider } from "radix-ui";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Brain, Check, ChevronRight, Globe } from "../icons";
import type { ModelChoice } from "../../../contracts/appserver.ts";

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
  description = "用于下一轮。运行中不能修改。",
}: {
  models: ModelChoice[] | null;
  value: ModelSelection;
  disabled: boolean;
  error: string;
  onChange: (value: ModelSelection) => void;
  onRetry: () => void;
  requiresVision: boolean;
  description?: string;
}) {
  const [open, setOpen] = useState(false);
  const [providerOpen, setProviderOpen] = useState<string | null>(null);
  const [previewEffort, setPreviewEffort] = useState<string | null>(null);
  useEffect(() => {
    setPreviewEffort(null);
  }, [value.model, value.reasoningEffort]);
  useEffect(() => {
    if (disabled) setOpen(false);
  }, [disabled]);

  const model = models?.find((item) => item.id === value.model);
  const modelName = value.model.slice(value.model.indexOf("/") + 1);
  const efforts = model?.reasoningEfforts ?? [];
  const effort = previewEffort ?? value.reasoningEffort;
  const effortIndex = efforts.indexOf(effort);
  const providers = new Map<string, ModelChoice[]>();
  for (const item of models ?? []) {
    const group = providers.get(item.provider) ?? [];
    group.push(item);
    providers.set(item.provider, group);
  }

  function selectModel(item: ModelChoice) {
    const reasoningEffort = item.reasoningEfforts.includes(value.reasoningEffort)
      ? value.reasoningEffort
      : item.reasoningEfforts[0];
    if (reasoningEffort === undefined) return;
    setProviderOpen(null);
    setPreviewEffort(null);
    if (item.id !== value.model || reasoningEffort !== value.reasoningEffort) {
      onChange({ model: item.id, reasoningEffort });
    }
  }

  return (
    <Popover
      open={open && !disabled}
      onOpenChange={(next) => {
        setOpen(next);
        setProviderOpen(null);
        setPreviewEffort(null);
      }}
    >
      <PopoverTrigger asChild>
        <Button variant="ghost" size="sm" className="model-trigger" disabled={disabled} aria-label="模型与思考" title={value.model ? `${value.model} · ${value.reasoningEffort}` : undefined}>
          <span className="model-name">{modelName || "选择模型"}</span>
          <span className="muted">· {value.reasoningEffort || "思考"}</span>
        </Button>
      </PopoverTrigger>
      <PopoverContent className="model-popover" align="end" side="top" collisionPadding={12} aria-label="模型与思考">
        <div className="picker-heading">
          <h3>模型</h3>
          <p>{description}</p>
        </div>
        {error ? (
          <div role="status" className="model-picker-status">
            <p className="inline-error">{error}</p>
            <Button variant="ghost" size="sm" onClick={onRetry}>重新加载模型</Button>
          </div>
        ) : models === null ? (
          <p className="model-picker-status metadata">正在加载模型…</p>
        ) : models.length === 0 ? (
          <p className="model-picker-status metadata">尚未配置可用模型。</p>
        ) : null}
        <div className="model-providers" role="group" aria-label="供应商">
          {[...providers].map(([provider, items]) => (
            <Popover key={provider} open={providerOpen === provider} onOpenChange={(next) => setProviderOpen(next ? provider : null)}>
              <PopoverTrigger asChild>
                <button className="ui-menu-item model-provider" aria-label={`${provider} 模型`}>
                  <Globe />
                  <span>{provider}</span>
                  {model?.provider === provider && <Check className="model-provider-check" />}
                  <ChevronRight />
                </button>
              </PopoverTrigger>
              <PopoverContent className="model-submenu" side="right" align="start" sideOffset={10} collisionPadding={12} aria-label={`${provider} 模型`}>
                <div className="model-submenu-heading">{provider}</div>
                <div className="model-options" role="group" aria-label="模型">
                  {items.map((item) => {
                    const blocked = requiresVision && !item.vision;
                    const name = item.id.startsWith(`${provider}/`) ? item.id.slice(provider.length + 1) : item.id;
                    return (
                      <button className="ui-menu-item" key={item.id} aria-pressed={item.id === value.model} disabled={blocked || item.reasoningEfforts.length === 0} onClick={() => selectModel(item)}>
                        <span>
                          <strong>{name}</strong>
                          {blocked && <small>当前图片需要视觉模型</small>}
                          {item.reasoningEfforts.length === 0 && <small>未配置思考档位</small>}
                        </span>
                        {item.id === value.model && <Check />}
                      </button>
                    );
                  })}
                </div>
              </PopoverContent>
            </Popover>
          ))}
        </div>
        <div className="model-reasoning">
          <div className="model-reasoning-heading">
            <Brain />
            <h3>思考档位</h3>
            {effortIndex >= 0 && <span className="model-effort-value">{effort}</span>}
          </div>
          {efforts.length > 0 ? (
            <>
              {effortIndex < 0 && <p className="metadata">请选择思考档位</p>}
              <Slider.Root
                className="effort-slider"
                min={0}
                max={Math.max(1, efforts.length - 1)}
                step={1}
                value={[Math.max(0, effortIndex)]}
                disabled={disabled || efforts.length === 1 || (requiresVision && !model?.vision)}
                onValueChange={([index]) => setPreviewEffort(efforts[index])}
                onValueCommit={([index]) => {
                  if (model && efforts[index] !== value.reasoningEffort) {
                    onChange({ model: model.id, reasoningEffort: efforts[index] });
                  }
                }}
              >
                <Slider.Track className="effort-slider-track">
                  <Slider.Range className="effort-slider-range" />
                  <span className="effort-slider-stops" aria-hidden="true">
                    {efforts.map((name, index) => <i key={name} data-filled={index <= effortIndex} />)}
                  </span>
                </Slider.Track>
                <Slider.Thumb className="effort-slider-thumb" aria-label="思考档位" aria-valuetext={effortIndex >= 0 ? effort : "请选择档位"} />
              </Slider.Root>
              <div className="effort-slider-labels" aria-hidden="true">
                {efforts.map((name) => <span key={name} data-active={name === effort}>{name}</span>)}
              </div>
              {efforts.length === 1 && <p className="metadata">此模型仅支持该档位</p>}
            </>
          ) : <p className="metadata">{model ? "此模型未配置思考档位" : "选择模型后可调节"}</p>}
        </div>
      </PopoverContent>
    </Popover>
  );
}
