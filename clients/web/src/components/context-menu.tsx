import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";

export type ContextMenuItem =
  | {
      type?: "item";
      label: string;
      action: () => void | Promise<void>;
      disabled?: boolean;
    }
  | { type: "separator" };

export function ContextMenu({
  x,
  y,
  items,
  label = "快捷操作",
  onClose,
}: {
  x: number;
  y: number;
  items: ContextMenuItem[];
  label?: string;
  onClose: () => void;
}) {
  const menuRef = useRef<HTMLDivElement>(null);
  const closeRef = useRef(onClose);
  const [position, setPosition] = useState({ left: x, top: y });
  closeRef.current = onClose;

  useLayoutEffect(() => {
    const menu = menuRef.current;
    if (!menu) return;
    const bounds = menu.getBoundingClientRect();
    setPosition({
      left: Math.max(8, Math.min(x, window.innerWidth - bounds.width - 8)),
      top: Math.max(8, Math.min(y, window.innerHeight - bounds.height - 8)),
    });
  }, [x, y, items.length]);

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
      ref={menuRef}
      className="app-context-menu"
      role="menu"
      aria-label={label}
      style={position}
      onPointerDown={(event) => event.stopPropagation()}
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
              closeRef.current();
              void item.action();
            }}
          >
            {item.label}
          </button>
        ),
      )}
    </div>,
    document.body,
  );
}
