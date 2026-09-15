import type { KeyboardEvent, PointerEvent } from "react";

export function ResizeHandle({
  label,
  value,
  min,
  max,
  growToward,
  className = "",
  onChange,
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  growToward: "left" | "right";
  className?: string;
  onChange: (value: number) => void;
}) {
  function clamp(next: number) {
    return Math.max(min, Math.min(max, next));
  }

  function dragValue(element: HTMLDivElement, clientX: number) {
    const startX = Number(element.dataset.startX);
    const startValue = Number(element.dataset.startValue);
    const distance = clientX - startX;
    return clamp(
      startValue + (growToward === "right" ? distance : -distance),
    );
  }

  function preview(event: PointerEvent<HTMLDivElement>) {
    const element = event.currentTarget;
    if (!element.hasPointerCapture(event.pointerId)) return;
    const startValue = Number(element.dataset.startValue);
    const next = dragValue(element, event.clientX);
    const offset =
      growToward === "right" ? next - startValue : startValue - next;
    element.dataset.nextValue = String(next);
    element.style.transform = `translateX(${offset}px)`;
  }

  function finish(event: PointerEvent<HTMLDivElement>) {
    const element = event.currentTarget;
    if (!element.hasPointerCapture(event.pointerId)) return;
    const next = dragValue(element, event.clientX);
    element.style.removeProperty("transform");
    delete element.dataset.dragging;
    delete element.dataset.startX;
    delete element.dataset.startValue;
    delete element.dataset.nextValue;
    element.releasePointerCapture(event.pointerId);
    // 长聊天只在松手时重新布局一次，拖动期间仅移动指示线。
    onChange(next);
  }

  function cancel(event: PointerEvent<HTMLDivElement>) {
    const element = event.currentTarget;
    element.style.removeProperty("transform");
    delete element.dataset.dragging;
    delete element.dataset.startX;
    delete element.dataset.startValue;
    delete element.dataset.nextValue;
    if (element.hasPointerCapture(event.pointerId))
      element.releasePointerCapture(event.pointerId);
  }

  function moveWithKeyboard(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    event.preventDefault();
    const distance = event.key === "ArrowRight" ? 20 : -20;
    onChange(clamp(value + (growToward === "right" ? distance : -distance)));
  }

  return (
    <div
      role="separator"
      aria-label={label}
      aria-orientation="vertical"
      aria-valuemin={min}
      aria-valuemax={max}
      aria-valuenow={value}
      tabIndex={0}
      className={`resize-handle ${className}`}
      onKeyDown={moveWithKeyboard}
      onPointerDown={(event) => {
        event.preventDefault();
        event.currentTarget.dataset.startX = String(event.clientX);
        event.currentTarget.dataset.startValue = String(value);
        event.currentTarget.dataset.nextValue = String(value);
        event.currentTarget.dataset.dragging = "true";
        event.currentTarget.setPointerCapture(event.pointerId);
      }}
      onPointerMove={preview}
      onPointerUp={finish}
      onPointerCancel={cancel}
    />
  );
}
