import {
  forwardRef,
  useImperativeHandle,
  useLayoutEffect,
  useRef,
  type ClipboardEvent,
  type KeyboardEvent,
} from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { BookOpenCheck, Command } from "../icons";
import { replaceSkillText, skillParts } from "./skill-mentions";

type CommandToken = { id: string; name: string; scope: string };

export interface SkillEditorHandle {
  focus(): void;
  replace(start: number, end: number, value: string, skillName?: string): void;
}

const iconHTML = renderToStaticMarkup(<BookOpenCheck aria-hidden="true" />);
const commandIconHTML = renderToStaticMarkup(<Command aria-hidden="true" />);

function commandNode(command: CommandToken): HTMLSpanElement {
  const chip = document.createElement("span");
  chip.className = "skill-token command-token";
  chip.dataset.commandName = command.name;
  chip.contentEditable = "false";
  const scope = command.scope === "workspace" ? "工作区" : "用户";
  chip.setAttribute("aria-label", `${scope}命令 ${command.name}`);
  chip.innerHTML = commandIconHTML;
  const label = document.createElement("span");
  label.textContent = command.name;
  const source = document.createElement("span");
  source.className = "command-token-scope";
  source.textContent = scope;
  chip.append(label, source);
  return chip;
}

function skillNode(name: string): HTMLSpanElement {
  const chip = document.createElement("span");
  chip.className = "skill-token";
  chip.dataset.skillName = name;
  chip.contentEditable = "false";
  chip.setAttribute("aria-label", `Skill ${name}`);
  chip.innerHTML = iconHTML;
  const label = document.createElement("span");
  label.textContent = name;
  chip.append(label);
  return chip;
}

function tokenText(node: HTMLElement): string | null {
  if (node.dataset.commandName) return `/${node.dataset.commandName}`;
  if (node.dataset.skillName) return `$${node.dataset.skillName}`;
  return null;
}

function plainText(node: Node): string {
  if (node.nodeType === Node.TEXT_NODE)
    return (node.textContent ?? "").replace(/\u00a0/g, " ");
  if (node instanceof HTMLElement) {
    const token = tokenText(node);
    if (token) return token;
    if (node.tagName === "BR") return "\n";
  }
  return Array.from(node.childNodes, plainText).join("");
}

function editorText(root: HTMLElement): string {
  const only = root.childNodes.length === 1 ? root.firstChild : null;
  const filler = only instanceof HTMLElement && only.tagName === "DIV" &&
    only.childNodes.length === 1 ? only.firstChild : only;
  if (filler instanceof HTMLElement && filler.tagName === "BR" &&
    !filler.hasAttribute("data-skill-break")) return "";
  return plainText(root);
}

function renderDraft(root: HTMLElement, text: string, names: ReadonlySet<string>, command?: CommandToken) {
  const fragment = document.createDocumentFragment();
  const invocation = command ? `/${command.name}` : "";
  if (command && (text === invocation || text.startsWith(`${invocation} `) || text.startsWith(`${invocation}\n`))) {
    fragment.append(commandNode(command));
    text = text.slice(invocation.length);
  }
  for (const part of skillParts(text, names))
    fragment.append(part.kind === "skill" ? skillNode(part.name) : document.createTextNode(part.value));
  root.replaceChildren(fragment);
}

// 光标位置用原始 $name 或 /name 文本的偏移量表示，与草稿、候选范围保持一致。
function pointAt(root: HTMLElement, target: number, endOfSelection = false): [Node, number] {
  let passed = 0;
  function visit(node: Node): [Node, number] | null {
    if (node.nodeType === Node.TEXT_NODE) {
      const length = plainText(node).length;
      if (target <= passed + length) return [node, Math.max(0, target - passed)];
      passed += length;
      return null;
    }
    const token = node instanceof HTMLElement ? tokenText(node) : null;
    if (token) {
      const index = Array.prototype.indexOf.call(node.parentNode?.childNodes, node) as number;
      const length = token.length;
      if (target <= passed + length)
        return [node.parentNode!, index + (target === passed ? 0 :
          target === passed + length || endOfSelection ? 1 : 0)];
      passed += length;
      return null;
    }
    if (node instanceof HTMLElement && node.tagName === "BR") {
      const index = Array.prototype.indexOf.call(node.parentNode?.childNodes, node) as number;
      if (target <= passed + 1) return [node.parentNode!, index + (target > passed ? 1 : 0)];
      passed++;
      return null;
    }
    for (const child of node.childNodes) {
      const point = visit(child);
      if (point) return point;
    }
    return null;
  }
  return visit(root) ?? [root, root.childNodes.length];
}

function cursorOffset(root: HTMLElement): number {
  const selection = window.getSelection();
  if (!selection?.focusNode || !root.contains(selection.focusNode)) return editorText(root).length;
  const before = document.createRange();
  before.selectNodeContents(root);
  before.setEnd(selection.focusNode, selection.focusOffset);
  return Math.min(plainText(before.cloneContents()).length, editorText(root).length);
}

function selectedOffsets(root: HTMLElement): [number, number] {
  const selection = window.getSelection();
  if (!selection?.rangeCount || !root.contains(selection.getRangeAt(0).commonAncestorContainer)) {
    const end = editorText(root).length;
    return [end, end];
  }
  const range = selection.getRangeAt(0);
  const before = document.createRange();
  before.selectNodeContents(root);
  before.setEnd(range.startContainer, range.startOffset);
  const start = Math.min(plainText(before.cloneContents()).length, editorText(root).length);
  before.setEnd(range.endContainer, range.endOffset);
  return [start, Math.min(plainText(before.cloneContents()).length, editorText(root).length)];
}

function selectRange(root: HTMLElement, start: number, end: number) {
  const range = document.createRange();
  const [startNode, startOffset] = pointAt(root, start);
  const [endNode, endOffset] = pointAt(root, end, true);
  range.setStart(startNode, startOffset);
  range.setEnd(endNode, endOffset);
  const selection = window.getSelection();
  selection?.removeAllRanges();
  selection?.addRange(range);
  return range;
}

function escapeHTML(value: string): string {
  return value.replace(/[&<>"']/g, (character) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  })[character]!);
}

export const SkillEditor = forwardRef<SkillEditorHandle, {
  value: string;
  names: string[];
  command?: CommandToken;
  placeholder: string;
  onChange(value: string, cursor: number): void;
  onCursor(cursor: number): void;
  onKeyDown(event: KeyboardEvent<HTMLDivElement>): void;
  onCompositionChange(composing: boolean, cursor: number): void;
  onPasteImages(files: File[]): void;
}>(function SkillEditor({
  value, names, command, placeholder, onChange, onCursor, onKeyDown, onCompositionChange, onPasteImages,
}, ref) {
  const root = useRef<HTMLDivElement>(null);
  const composing = useRef(false);
  const reported = useRef(value);
  const namesKey = names.join("\0");
  const renderedNames = useRef("");
  const commandKey = command?.id ?? "";
  const renderedCommand = useRef("");

  function emit() {
    if (!root.current) return;
    const next = editorText(root.current);
    const cursor = cursorOffset(root.current);
    onCursor(cursor);
    if (next !== reported.current) {
      reported.current = next;
      onChange(next, cursor);
    }
  }

  function replace(start: number, end: number, text: string, skillName?: string) {
    const element = root.current;
    if (!element) return;
    element.focus();
    const before = editorText(element);
    const range = selectRange(element, start, end);
    const html = skillName
      ? `<span class="skill-token" data-skill-name="${escapeHTML(skillName)}" contenteditable="false" aria-label="Skill ${escapeHTML(skillName)}">${iconHTML}<span>${escapeHTML(skillName)}</span></span>&nbsp;`
      : text.includes("\n") ? escapeHTML(text).replace(/ /g, "&nbsp;").replace(/\n/g, "<br data-skill-break>") : "";
    const inserted = document.execCommand(skillName || text.includes("\n") ? "insertHTML" : "insertText", false,
      html || text);
    if (!inserted) {
      range.deleteContents();
      if (skillName) {
        const space = document.createTextNode(" ");
        range.insertNode(space);
        range.insertNode(skillNode(skillName));
        range.setStartAfter(space);
      } else if (text) {
        const node = document.createTextNode(text);
        range.insertNode(node);
        range.setStartAfter(node);
      }
      range.collapse(true);
      window.getSelection()?.removeAllRanges();
      window.getSelection()?.addRange(range);
    }
    const expected = replaceSkillText(before, start, end, skillName ? `$${skillName} ` : text);
    if (editorText(element) !== expected) {
      renderDraft(element, expected, new Set(names), command);
      const position = start + (skillName ? skillName.length + 2 : text.length);
      selectRange(element, position, position);
    }
    emit();
  }

  useLayoutEffect(() => {
    const element = root.current;
    if (!element || composing.current) return;
    const current = editorText(element);
    if (current === value && renderedNames.current === namesKey && renderedCommand.current === commandKey) return;
    const focused = document.activeElement === element;
    const cursor = focused && command && renderedCommand.current !== commandKey
      ? Math.min(value.length, command.name.length + 2)
      : focused ? cursorOffset(element) : value.length;
    renderDraft(element, value, new Set(names), command);
    reported.current = value;
    renderedNames.current = namesKey;
    renderedCommand.current = commandKey;
    if (focused) {
      selectRange(element, Math.min(cursor, value.length), Math.min(cursor, value.length));
      onCursor(cursorOffset(element));
    }
  }, [value, namesKey, commandKey]);

  useImperativeHandle(ref, () => ({
    focus: () => {
      const element = root.current;
      if (!element) return;
      element.focus();
      const selection = window.getSelection();
      if (!selection?.focusNode || !element.contains(selection.focusNode)) {
        const end = editorText(element).length;
        selectRange(element, end, end);
      }
    },
    replace,
  }));

  function handlePaste(event: ClipboardEvent<HTMLDivElement>) {
    const files = Array.from(event.clipboardData.files);
    if (files.length) {
      event.preventDefault();
      onPasteImages(files);
      return;
    }
    event.preventDefault();
    const text = event.clipboardData.getData("text/plain");
    if (text) {
      const [start, end] = selectedOffsets(event.currentTarget);
      replace(start, end, text);
    }
  }

  function deleteAdjacentToken(element: HTMLElement, backward: boolean): boolean {
    const [start, end] = selectedOffsets(element);
    if (start !== end) return false;
    for (const chip of element.querySelectorAll<HTMLElement>("[data-skill-name], [data-command-name]")) {
      const before = document.createRange();
      before.selectNodeContents(element);
      before.setEndBefore(chip);
      const position = plainText(before.cloneContents()).length;
      const length = tokenText(chip)?.length ?? 0;
      if ((backward && end === position + length) || (!backward && start === position)) {
        replace(position, position + length, "");
        return true;
      }
    }
    return false;
  }

  return (
    <div
      ref={root}
      className="skill-editor"
      role="textbox"
      aria-label="消息输入"
      aria-multiline="true"
      contentEditable
      suppressContentEditableWarning
      spellCheck
      data-placeholder={placeholder}
      data-empty={!value}
      onInput={emit}
      onKeyDown={(event) => {
        onKeyDown(event);
        if (event.defaultPrevented || composing.current || event.nativeEvent.isComposing) return;
        if ((event.key === "Backspace" || event.key === "Delete") &&
          deleteAdjacentToken(event.currentTarget, event.key === "Backspace")) {
          event.preventDefault();
          return;
        }
        if (event.key === "Enter" && event.shiftKey) {
          event.preventDefault();
          const [start, end] = selectedOffsets(event.currentTarget);
          replace(start, end, "\n");
        }
      }}
      onBeforeInput={(event) => {
        const type = (event.nativeEvent as InputEvent).inputType;
        if (composing.current || (type !== "insertParagraph" && type !== "insertLineBreak")) return;
        event.preventDefault();
        const [start, end] = selectedOffsets(event.currentTarget);
        replace(start, end, "\n");
      }}
      onCompositionStart={() => { composing.current = true; onCompositionChange(true, cursorOffset(root.current!)); }}
      onCompositionEnd={() => { composing.current = false; emit(); onCompositionChange(false, cursorOffset(root.current!)); }}
      onKeyUp={() => { if (root.current) onCursor(cursorOffset(root.current)); }}
      onMouseUp={() => { if (root.current) onCursor(cursorOffset(root.current)); }}
      onPaste={handlePaste}
      onCopy={(event) => {
        const [start, end] = selectedOffsets(event.currentTarget);
        if (start === end) return;
        event.preventDefault();
        event.clipboardData.setData("text/plain", editorText(event.currentTarget).slice(start, end));
      }}
      onCut={(event) => {
        const [start, end] = selectedOffsets(event.currentTarget);
        if (start === end) return;
        event.preventDefault();
        event.clipboardData.setData("text/plain", editorText(event.currentTarget).slice(start, end));
        replace(start, end, "");
      }}
      onDrop={(event) => event.preventDefault()}
    />
  );
});
