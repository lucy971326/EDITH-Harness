import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";

export interface FloatingAnchor {
  left: number;
  top: number;
  right: number;
  bottom: number;
}

export type ContextMenuItem =
  | {
      type?: "item";
      label: string;
      action: () => void | Promise<void>;
      disabled?: boolean;
    }
  | { type: "separator" };

export function FloatingSurface({
  x,
  y,
  anchor,
  className,
  role,
  label,
  onClose,
  children,
}: {
  x?: number;
  y?: number;
  anchor?: FloatingAnchor;
  className: string;
  role: string;
  label: string;
  onClose: () => void;
  children: ReactNode;
}) {
  const surfaceRef = useRef<HTMLDivElement>(null);
  const closeRef = useRef(onClose);
  const [position, setPosition] = useState({ left: x ?? anchor?.left ?? 0, top: y ?? anchor?.top ?? 0 });
  closeRef.current = onClose;

  useLayoutEffect(() => {
    const surface = surfaceRef.current;
    if (!surface) return;
    const bounds = surface.getBoundingClientRect();
    let left = x ?? 0;
    let top = y ?? 0;
    if (anchor) {
      left = anchor.left + (anchor.right - anchor.left - bounds.width) / 2;
      top = anchor.top - bounds.height - 8;
      if (top < 8) top = anchor.bottom + 8;
    }
    setPosition({
      left: Math.max(8, Math.min(left, window.innerWidth - bounds.width - 8)),
      top: Math.max(8, Math.min(top, window.innerHeight - bounds.height - 8)),
    });
  }, [x, y, anchor?.left, anchor?.top, anchor?.right, anchor?.bottom, className]);

  useEffect(() => {
    const close = () => closeRef.current();
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") close();
    };
    window.addEventListener("pointerdown", close);
    window.addEventListener("scroll", close, true);
    window.addEventListener("blur", close);
    window.addEventListener("resize", close);
    window.addEventListener("keydown", closeOnEscape);
    return () => {
      window.removeEventListener("pointerdown", close);
      window.removeEventListener("scroll", close, true);
      window.removeEventListener("blur", close);
      window.removeEventListener("resize", close);
      window.removeEventListener("keydown", closeOnEscape);
    };
  }, []);

  return createPortal(
    <div
      ref={surfaceRef}
      className={className}
      role={role}
      aria-label={label}
      style={{ position: "fixed", zIndex: 100, ...position }}
      onPointerDown={(event) => event.stopPropagation()}
    >
      {children}
    </div>,
    document.body,
  );
}

export function ContextMenu({
  x,
  y,
  anchor,
  compact = false,
  items,
  label = "快捷操作",
  onClose,
}: {
  x?: number;
  y?: number;
  anchor?: FloatingAnchor;
  compact?: boolean;
  items: ContextMenuItem[];
  label?: string;
  onClose: () => void;
}) {
  return (
    <FloatingSurface
      x={x}
      y={y}
      anchor={anchor}
      className={`app-context-menu${compact ? " app-context-menu-compact" : ""}`}
      role="menu"
      label={label}
      onClose={onClose}
    >
      {items.map((item, index) =>
        item.type === "separator" ? (
          <div key={`separator:${index}`} className="app-context-menu-separator" role="separator" />
        ) : (
          <button
            key={`${item.label}:${index}`}
            role="menuitem"
            disabled={item.disabled}
            onClick={() => {
              onClose();
              void item.action();
            }}
          >
            {item.label}
          </button>
        ),
      )}
    </FloatingSurface>
  );
}
