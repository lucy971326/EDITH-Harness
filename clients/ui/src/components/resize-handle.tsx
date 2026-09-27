import { useEffect, useRef, type KeyboardEvent, type PointerEvent } from "react";

export function ResizeHandle({
  label,
  value,
  min,
  max,
  growToward,
  className = "",
  onResize,
  onChange,
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  growToward: "left" | "right";
  className?: string;
  onResize: (value: number) => void;
  onChange: (value: number) => void;
}) {
  const resizeFrame = useRef<number | null>(null);
  const pendingValue = useRef<number | null>(null);

  useEffect(
    () => () => {
      if (resizeFrame.current !== null)
        cancelAnimationFrame(resizeFrame.current);
    },
    [],
  );

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

  function resizeOnNextFrame(next: number) {
    pendingValue.current = next;
    if (resizeFrame.current !== null) return;
    resizeFrame.current = requestAnimationFrame(() => {
      resizeFrame.current = null;
      const value = pendingValue.current;
      pendingValue.current = null;
      if (value !== null) onResize(value);
    });
  }

  function move(event: PointerEvent<HTMLDivElement>) {
    const element = event.currentTarget;
    if (!element.hasPointerCapture(event.pointerId)) return;
    resizeOnNextFrame(dragValue(element, event.clientX));
  }

  function finish(event: PointerEvent<HTMLDivElement>) {
    const element = event.currentTarget;
    if (!element.hasPointerCapture(event.pointerId)) return;
    const next = dragValue(element, event.clientX);
    if (resizeFrame.current !== null) cancelAnimationFrame(resizeFrame.current);
    resizeFrame.current = null;
    pendingValue.current = null;
    delete element.dataset.dragging;
    delete element.dataset.startX;
    delete element.dataset.startValue;
    element.releasePointerCapture(event.pointerId);
    onResize(next);
    onChange(next);
  }

  function cancel(event: PointerEvent<HTMLDivElement>) {
    const element = event.currentTarget;
    if (resizeFrame.current !== null) cancelAnimationFrame(resizeFrame.current);
    resizeFrame.current = null;
    pendingValue.current = null;
    delete element.dataset.dragging;
    delete element.dataset.startX;
    delete element.dataset.startValue;
    if (element.hasPointerCapture(event.pointerId))
      element.releasePointerCapture(event.pointerId);
    onResize(value);
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
        event.currentTarget.dataset.dragging = "true";
        event.currentTarget.setPointerCapture(event.pointerId);
      }}
      onPointerMove={move}
      onPointerUp={finish}
      onPointerCancel={cancel}
    />
  );
}
