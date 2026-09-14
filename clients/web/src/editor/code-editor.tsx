import { useEffect, useRef, useState } from "react";
import Editor, { loader, type Monaco, type OnMount } from "@monaco-editor/react";
import * as monaco from "monaco-editor";
import EditorWorker from "monaco-editor/editor/editor.worker.js?worker";
import type { EditorFile } from "./files";
import type { FileLocation } from "./links";

self.MonacoEnvironment = {
  getWorker: () => new EditorWorker(),
};
loader.config({ monaco });

function language(path: string): string | undefined {
  const extension = path.split(".").at(-1)?.toLowerCase();
  return {
    c: "c",
    cpp: "cpp",
    css: "css",
    go: "go",
    html: "html",
    java: "java",
    js: "javascript",
    json: "json",
    jsx: "javascript",
    md: "markdown",
    py: "python",
    rs: "rust",
    sh: "shell",
    ts: "typescript",
    tsx: "typescript",
    xml: "xml",
    yaml: "yaml",
    yml: "yaml",
  }[extension ?? ""];
}

function defineThemes(api: Monaco) {
  api.editor.defineTheme("harness-light", {
    base: "vs",
    inherit: true,
    rules: [],
    colors: {
      "editor.background": "#FFFFFF",
      "editor.foreground": "#292524",
      "editorLineNumber.foreground": "#A8A29E",
      "editorLineNumber.activeForeground": "#57534E",
      "editor.selectionBackground": "#E7E5E4",
      "editor.inactiveSelectionBackground": "#F0EEEB",
      "editor.lineHighlightBackground": "#F5F5F4",
      "editorCursor.foreground": "#292524",
      "editorIndentGuide.background1": "#E7E5E4",
    },
  });
  api.editor.defineTheme("harness-dark", {
    base: "vs-dark",
    inherit: true,
    rules: [],
    colors: {
      "editor.background": "#24211F",
      "editor.foreground": "#E7E5E4",
      "editorLineNumber.foreground": "#78716C",
      "editorLineNumber.activeForeground": "#D6D3D1",
      "editor.selectionBackground": "#57534E",
      "editor.inactiveSelectionBackground": "#393431",
      "editor.lineHighlightBackground": "#292524",
      "editorCursor.foreground": "#E7E5E4",
      "editorIndentGuide.background1": "#393431",
    },
  });
}

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
  const [dark, setDark] = useState(() =>
    document.documentElement.classList.contains("dark"),
  );
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
    const observer = new MutationObserver(() =>
      setDark(document.documentElement.classList.contains("dark")),
    );
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["class"],
    });
    return () => observer.disconnect();
  }, []);

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
      language={language(file.path)}
      theme={dark ? "harness-dark" : "harness-light"}
      beforeMount={defineThemes}
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
