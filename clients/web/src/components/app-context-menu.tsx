import { useEffect, useState } from "react";
import type { ContextReference } from "../context-references";
import { ContextMenu, type ContextMenuItem } from "./context-menu";

interface MenuState {
  x: number;
  y: number;
  items: ContextMenuItem[];
}

export function AppContextMenu({ onAddReference }: {
  onAddReference?: (reference: ContextReference) => void;
}) {
  const [menu, setMenu] = useState<MenuState | null>(null);

  useEffect(() => {
    setMenu(null);
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
      setMenu(createMenu(target, event.clientX, event.clientY, onAddReference));
    }
    function openFromKeyboard(event: KeyboardEvent) {
      if (event.key !== "ContextMenu" && !(event.shiftKey && event.key === "F10"))
        return;
      const target = document.activeElement;
      if (!(target instanceof HTMLElement) || target.closest(".monaco-editor")) return;
      event.preventDefault();
      const bounds = target.getBoundingClientRect();
      setMenu(createMenu(target, bounds.left + 12, bounds.top + 12, onAddReference));
    }
    window.addEventListener("contextmenu", open);
    window.addEventListener("keydown", openFromKeyboard);
    return () => {
      window.removeEventListener("contextmenu", open);
      window.removeEventListener("keydown", openFromKeyboard);
    };
  }, [onAddReference]);

  if (!menu) return null;
  return (
    <ContextMenu
      x={menu.x}
      y={menu.y}
      items={menu.items}
      onClose={() => setMenu(null)}
    />
  );
}

function createMenu(target: HTMLElement, x: number, y: number,
  onAddReference?: (reference: ContextReference) => void): MenuState {
  const editable = editableElement(target);
  const selected = window.getSelection()?.toString() ?? "";
  const pathElement = target.closest<HTMLElement>("[data-file-path]");
  const path = pathElement?.dataset.filePath;
  const items: ContextMenuItem[] = [];

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
    const kind = pathElement?.dataset.referenceKind;
    if (onAddReference && (kind === "file" || kind === "directory")) {
      items.push({ label: "添加到对话", action: () => onAddReference({ kind, path }) });
    }
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
