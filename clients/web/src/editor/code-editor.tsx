import { useEffect, useRef } from "react";
import Editor, { type OnMount } from "@monaco-editor/react";
import type { EditorFile } from "./files";
import type { FileLocation } from "./links";
import { defineEditorThemes, editorLanguage, useEditorTheme } from "./monaco";

export function CodeEditor({
  file,
  location,
  onChange,
  onSave,
}: {
  file: EditorFile;
  location?: FileLocation;
  onChange: (content: string) => void;
  onSave: () => void;
}) {
  const editorRef = useRef<Parameters<OnMount>[0] | null>(null);
  const saveRef = useRef(onSave);
  const theme = useEditorTheme();
  saveRef.current = onSave;

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

  const mount: OnMount = (editor, api) => {
    editorRef.current = editor;
    editor.addCommand(api.KeyMod.CtrlCmd | api.KeyCode.KeyS, () => saveRef.current());
    revealLocation(editor);
    editor.focus();
  };

  return (
    <Editor
      className="monaco-host"
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
        contextmenu: true,
        fontFamily:
          '"Cascadia Code", "SFMono-Regular", Consolas, "Liberation Mono", monospace',
        fontSize: 13,
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
  );
}
