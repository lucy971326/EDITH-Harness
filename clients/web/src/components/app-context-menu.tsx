import { useEffect, useState } from "react";

interface MenuItem {
  label: string;
  action: () => void | Promise<void>;
  disabled?: boolean;
}

interface MenuState {
  x: number;
  y: number;
  items: MenuItem[];
}

export function AppContextMenu() {
  const [menu, setMenu] = useState<MenuState | null>(null);

  useEffect(() => {
    function open(event: MouseEvent) {
      const origin = event.target;
      const target =
        origin instanceof HTMLElement
          ? origin
          : origin instanceof Element
            ? origin.parentElement
            : null;
      if (!target) return;
      // Monaco 自带菜单使用它自己的 Command API，保留其完整编辑能力。
      if (target.closest(".monaco-editor")) return;
      event.preventDefault();
      setMenu(createMenu(target, event.clientX, event.clientY));
    }
    function openFromKeyboard(event: KeyboardEvent) {
      if (event.key !== "ContextMenu" && !(event.shiftKey && event.key === "F10"))
        return;
      const target = document.activeElement;
      if (!(target instanceof HTMLElement) || target.closest(".monaco-editor")) return;
      event.preventDefault();
      const bounds = target.getBoundingClientRect();
      setMenu(createMenu(target, bounds.left + 12, bounds.top + 12));
    }
    function close() {
      setMenu(null);
    }
    window.addEventListener("contextmenu", open);
    window.addEventListener("keydown", openFromKeyboard);
    window.addEventListener("blur", close);
    window.addEventListener("resize", close);
    return () => {
      window.removeEventListener("contextmenu", open);
      window.removeEventListener("keydown", openFromKeyboard);
      window.removeEventListener("blur", close);
      window.removeEventListener("resize", close);
    };
  }, []);

  useEffect(() => {
    if (!menu) return;
    function close() {
      setMenu(null);
    }
    window.addEventListener("pointerdown", close);
    window.addEventListener("scroll", close, true);
    return () => {
      window.removeEventListener("pointerdown", close);
      window.removeEventListener("scroll", close, true);
    };
  }, [menu]);

  if (!menu) return null;
  return (
    <div
      className="app-context-menu"
      role="menu"
      aria-label="快捷操作"
      style={{
        left: Math.min(menu.x, window.innerWidth - 190),
        top: Math.min(menu.y, window.innerHeight - menu.items.length * 34 - 12),
      }}
      onPointerDown={(event) => event.stopPropagation()}
    >
      {menu.items.map((item) => (
        <button
          key={item.label}
          role="menuitem"
          disabled={item.disabled}
          onClick={() => {
            setMenu(null);
            void item.action();
          }}
        >
          {item.label}
        </button>
      ))}
    </div>
  );
}

function createMenu(target: HTMLElement, x: number, y: number): MenuState {
  const editable = editableElement(target);
  const selected = window.getSelection()?.toString() ?? "";
  const pathElement = target.closest<HTMLElement>("[data-file-path]");
  const path = pathElement?.dataset.filePath;
  const items: MenuItem[] = [];

  if (editable) {
    items.push(
      {
        label: "撤销",
        action: () => {
          document.execCommand("undo");
        },
      },
      {
        label: "重做",
        action: () => {
          document.execCommand("redo");
        },
      },
      {
        label: "剪切",
        action: () => {
          document.execCommand("cut");
        },
      },
      {
        label: "复制",
        action: () => {
          document.execCommand("copy");
        },
      },
      { label: "粘贴", action: () => pasteInto(editable) },
      { label: "全选", action: () => selectAll(editable) },
    );
  } else if (selected) {
    items.push({
      label: "复制",
      action: () => navigator.clipboard.writeText(selected),
    });
  }

  if (path) {
    const closeButton = pathElement
      ?.closest(".workspace-tab")
      ?.querySelector<HTMLButtonElement>("[data-context-close]");
    if (pathElement?.tagName === "BUTTON") {
      items.push({ label: "打开", action: () => pathElement.click() });
    }
    if (closeButton) items.push({ label: "关闭", action: () => closeButton.click() });
    items.push({ label: "复制路径", action: () => navigator.clipboard.writeText(path) });
  }

  if (items.length === 0) {
    items.push({ label: "没有可用操作", action: () => {}, disabled: true });
  }
  return { x, y, items };
}

function editableElement(target: HTMLElement): HTMLElement | null {
  const editable = target.closest<HTMLElement>("input, textarea, [contenteditable='true']");
  return editable instanceof HTMLInputElement ||
    editable instanceof HTMLTextAreaElement ||
    editable?.isContentEditable
    ? editable
    : null;
}

async function pasteInto(target: HTMLElement) {
  const text = await navigator.clipboard.readText();
  if (target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement) {
    target.setRangeText(
      text,
      target.selectionStart ?? target.value.length,
      target.selectionEnd ?? target.value.length,
      "end",
    );
    target.dispatchEvent(new InputEvent("input", { bubbles: true, data: text }));
    return;
  }
  target.focus();
  document.execCommand("insertText", false, text);
}

function selectAll(target: HTMLElement) {
  target.focus();
  if (target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement) {
    target.select();
    return;
  }
  document.execCommand("selectAll");
}
