import { useEffect, useState } from "react";
import { loader, type Monaco } from "@monaco-editor/react";
import * as monaco from "monaco-editor";
import EditorWorker from "monaco-editor/editor/editor.worker.js?worker";

self.MonacoEnvironment = {
  getWorker: () => new EditorWorker(),
};
loader.config({ monaco });

export const editorFontFamily =
  getComputedStyle(document.documentElement).getPropertyValue("--font-code").trim();

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
    rules: [
      { token: "comment", foreground: "74746D" },
      { token: "keyword", foreground: "154BD1" },
      { token: "string", foreground: "A23F36" },
      { token: "number", foreground: "7C5A1F" },
      { token: "type", foreground: "315EAF" },
      { token: "type.identifier", foreground: "315EAF" },
    ],
    colors: {
      "editor.background": "#FFFFFF",
      "editor.foreground": "#111111",
      "editorLineNumber.foreground": "#999991",
      "editorLineNumber.activeForeground": "#555550",
      "editor.selectionBackground": "#DCE6FF",
      "editor.inactiveSelectionBackground": "#EAF0FF",
      "editor.lineHighlightBackground": "#F8F8F7",
      "editorCursor.foreground": "#1E63FF",
      "editorIndentGuide.background1": "#E2E2DD",
      "editorIndentGuide.activeBackground1": "#B9B9B4",
      "editorWhitespace.foreground": "#D7D7D1",
      "editorGutter.background": "#FFFFFF",
      "editorWidget.background": "#FFFFFF",
      "editorWidget.border": "#DDDDDA",
      "editorHoverWidget.background": "#FFFFFF",
      "editorHoverWidget.border": "#DDDDDA",
      "diffEditor.insertedTextBackground": "#CBEFD680",
      "diffEditor.removedTextBackground": "#FFD6D180",
      "diffEditor.insertedLineBackground": "#ECF7EF",
      "diffEditor.removedLineBackground": "#FFF0EE",
    },
  });
  api.editor.defineTheme("harness-dark", {
    base: "vs-dark",
    inherit: true,
    rules: [
      { token: "comment", foreground: "AAA9A2" },
      { token: "keyword", foreground: "99B8FF" },
      { token: "string", foreground: "E5A092" },
      { token: "number", foreground: "D8B784" },
      { token: "type", foreground: "B5C9FF" },
      { token: "type.identifier", foreground: "B5C9FF" },
    ],
    colors: {
      "editor.background": "#1C1C1A",
      "editor.foreground": "#F5F5F1",
      "editorLineNumber.foreground": "#85857D",
      "editorLineNumber.activeForeground": "#C6C6BF",
      "editor.selectionBackground": "#36558D",
      "editor.inactiveSelectionBackground": "#2A3C5B",
      "editor.lineHighlightBackground": "#252523",
      "editorCursor.foreground": "#80A4FF",
      "editorIndentGuide.background1": "#393934",
      "editorIndentGuide.activeBackground1": "#62625A",
      "editorWhitespace.foreground": "#41413C",
      "editorGutter.background": "#1C1C1A",
      "editorWidget.background": "#242422",
      "editorWidget.border": "#41413C",
      "editorHoverWidget.background": "#242422",
      "editorHoverWidget.border": "#41413C",
      "diffEditor.insertedTextBackground": "#2EA04355",
      "diffEditor.removedTextBackground": "#F8514955",
      "diffEditor.insertedLineBackground": "#1D3325",
      "diffEditor.removedLineBackground": "#3A2423",
    },
  });
}

export function useEditorTheme(): string {
  const [dark, setDark] = useState(() =>
    document.documentElement.classList.contains("dark"),
  );
  useEffect(() => {
    // 中文字体按字符分片加载；到达后刷新 Monaco 缓存的字宽，避免光标错位。
    const remeasureFonts = () => monaco.editor.remeasureFonts();
    document.fonts.addEventListener("loadingdone", remeasureFonts);
    remeasureFonts();
    const observer = new MutationObserver(() =>
      setDark(document.documentElement.classList.contains("dark")),
    );
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["class"],
    });
    return () => {
      observer.disconnect();
      document.fonts.removeEventListener("loadingdone", remeasureFonts);
    };
  }, []);
  return dark ? "harness-dark" : "harness-light";
}
