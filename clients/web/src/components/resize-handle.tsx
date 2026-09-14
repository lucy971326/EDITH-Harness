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

  function move(event: PointerEvent<HTMLDivElement>) {
    if (!event.currentTarget.hasPointerCapture(event.pointerId)) return;
    const startX = Number(event.currentTarget.dataset.startX);
    const startValue = Number(event.currentTarget.dataset.startValue);
    const distance = event.clientX - startX;
    onChange(
      clamp(startValue + (growToward === "right" ? distance : -distance)),
    );
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
        event.currentTarget.dataset.startX = String(event.clientX);
        event.currentTarget.dataset.startValue = String(value);
        event.currentTarget.setPointerCapture(event.pointerId);
      }}
      onPointerMove={move}
      onPointerUp={(event) =>
        event.currentTarget.releasePointerCapture(event.pointerId)
      }
    />
  );
}
