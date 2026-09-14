import { useEffect, useState } from "react";
import { loader, type Monaco } from "@monaco-editor/react";
import * as monaco from "monaco-editor";
import EditorWorker from "monaco-editor/editor/editor.worker.js?worker";

self.MonacoEnvironment = {
  getWorker: () => new EditorWorker(),
};
loader.config({ monaco });

export function editorLanguage(path: string): string | undefined {
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

export function defineEditorThemes(api: Monaco) {
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

export function useEditorTheme(): string {
  const [dark, setDark] = useState(() =>
    document.documentElement.classList.contains("dark"),
  );
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
  return dark ? "harness-dark" : "harness-light";
}
