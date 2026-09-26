import { useEffect, useRef, useState } from "react";
import Editor, { type OnMount } from "@monaco-editor/react";
import type { EditorFile } from "./files";
import type { FileLocation } from "./links";
import type { ContextReference } from "../chat/context-references";
import { ContextMenu } from "../components/context-menu";
import {
  defineEditorThemes,
  editorFontFamily,
  editorFontSize,
  editorLanguage,
  useEditorTheme,
} from "./monaco";

interface EditorMenu {
  x: number;
  y: number;
  hasSelection: boolean;
}

export function CodeEditor({
  file,
  location,
  onChange,
  onSave,
  onAddReference,
}: {
  file: EditorFile;
  location?: FileLocation;
  onChange: (content: string) => void;
  onSave: () => void;
  onAddReference?: (reference: ContextReference) => void;
}) {
  const editorRef = useRef<Parameters<OnMount>[0] | null>(null);
  const saveRef = useRef(onSave);
  const addReferenceRef = useRef(onAddReference);
  const pathRef = useRef(file.path);
  const [menu, setMenu] = useState<EditorMenu | null>(null);
  const theme = useEditorTheme();
  saveRef.current = onSave;
  addReferenceRef.current = onAddReference;
  pathRef.current = file.path;

  function revealLocation(editor: Parameters<OnMount>[0]) {
    if (!location?.line || location.path !== file.path) return;
    editor.setPosition({
      lineNumber: location.line,
      column: location.column ?? 1,
    });
    editor.revealLineInCenter(location.line);
  }

  useEffect(() => {
    const editor = editorRef.current;
    if (!editor) return;
    revealLocation(editor);
    editor.focus();
  }, [file.path, location]);

  function addSelectionToChat() {
    const current = editorRef.current;
    const model = current?.getModel();
    const selection = current?.getSelection();
    if (!current || !model || !selection || selection.isEmpty()) return;
    // 从当前 Model 取内容，不重读磁盘；路径保留文件系统格式，避免 URI 改写盘符。
    addReferenceRef.current?.({
      kind: "selection",
      path: pathRef.current,
      startLine: selection.startLineNumber,
      startColumn: selection.startColumn,
      endLine: selection.endLineNumber,
      endColumn: selection.endColumn,
      content: model.getValueInRange(selection),
    });
    current.focus();
  }

  function runEditorAction(action: string) {
    const current = editorRef.current;
    if (!current) return;
    current.focus();
    current.trigger("harness.context-menu", action, null);
  }

  const mount: OnMount = (editor, api) => {
    editorRef.current = editor;
    editor.addCommand(api.KeyMod.CtrlCmd | api.KeyCode.KeyS, () => saveRef.current());
    revealLocation(editor);
    editor.focus();
  };

  return (
    <>
      <div
        className="monaco-host"
        onContextMenu={(event) => {
          event.preventDefault();
          event.stopPropagation();
          const selection = editorRef.current?.getSelection();
          setMenu({
            x: event.clientX,
            y: event.clientY,
            hasSelection: Boolean(selection && !selection.isEmpty()),
          });
        }}
      >
        <Editor
          path={file.path}
          value={file.content}
          language={editorLanguage(file.path)}
          theme={theme}
          beforeMount={defineEditorThemes}
          onMount={mount}
          onChange={(value) => onChange(value ?? "")}
          loading={<div className="editor-loading">正在加载编辑器…</div>}
          saveViewState
          options={{
            automaticLayout: true,
            contextmenu: false,
            fontFamily: editorFontFamily,
            fontSize: editorFontSize,
            lineHeight: 21,
            minimap: { enabled: false },
            overviewRulerBorder: false,
            overviewRulerLanes: 0,
            padding: { top: 12 },
            renderLineHighlight: "line",
            scrollBeyondLastLine: false,
            smoothScrolling: true,
            wordWrap: "off",
          }}
        />
      </div>
      {menu && (
        <ContextMenu
          x={menu.x}
          y={menu.y}
          label="编辑器快捷操作"
          onClose={() => setMenu(null)}
          items={[
            {
              label: "添加到对话",
              disabled: !menu.hasSelection || !onAddReference,
              action: addSelectionToChat,
            },
            { type: "separator" },
            {
              label: "复制",
              disabled: !menu.hasSelection,
              action: () => runEditorAction("editor.action.clipboardCopyAction"),
            },
            {
              label: "剪切",
              disabled: !menu.hasSelection,
              action: () => runEditorAction("editor.action.clipboardCutAction"),
            },
            {
              label: "粘贴",
              action: () => runEditorAction("editor.action.clipboardPasteAction"),
            },
          ]}
        />
      )}
    </>
  );
}
