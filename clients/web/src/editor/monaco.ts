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
      { token: "comment", foreground: "687386" },
      { token: "keyword", foreground: "315EAF" },
      { token: "string", foreground: "A31515" },
      { token: "number", foreground: "875F00" },
      { token: "type", foreground: "0B7285" },
      { token: "type.identifier", foreground: "0B7285" },
    ],
    colors: {
      "editor.background": "#FFFFFF",
      "editor.foreground": "#1F2328",
      "editorLineNumber.foreground": "#9AA1AA",
      "editorLineNumber.activeForeground": "#4B5563",
      "editor.selectionBackground": "#DCE8FF",
      "editor.inactiveSelectionBackground": "#EAF0FA",
      "editor.lineHighlightBackground": "#F5F7FA",
      "editorCursor.foreground": "#2563EB",
      "editorIndentGuide.background1": "#E1E5EA",
      "editorIndentGuide.activeBackground1": "#B8C0CB",
      "editorWhitespace.foreground": "#D8DDE5",
      "editorGutter.background": "#FFFFFF",
      "editorWidget.background": "#FFFFFF",
      "editorWidget.border": "#DDE1E6",
      "editorHoverWidget.background": "#FFFFFF",
      "editorHoverWidget.border": "#DDE1E6",
      "diffEditor.insertedTextBackground": "#CBEFD680",
      "diffEditor.removedTextBackground": "#FFD6D180",
      "diffEditor.insertedLineBackground": "#EAF7EE",
      "diffEditor.removedLineBackground": "#FFF0EE",
    },
  });
  api.editor.defineTheme("harness-dark", {
    base: "vs-dark",
    inherit: true,
    rules: [
      { token: "comment", foreground: "8B949E" },
      { token: "keyword", foreground: "79B8FF" },
      { token: "string", foreground: "FFAB70" },
      { token: "number", foreground: "D2A8FF" },
      { token: "type", foreground: "76E3EA" },
      { token: "type.identifier", foreground: "76E3EA" },
    ],
    colors: {
      "editor.background": "#1D2026",
      "editor.foreground": "#DDE1E7",
      "editorLineNumber.foreground": "#737B87",
      "editorLineNumber.activeForeground": "#BAC1CB",
      "editor.selectionBackground": "#294D7A",
      "editor.inactiveSelectionBackground": "#263B57",
      "editor.lineHighlightBackground": "#242831",
      "editorCursor.foreground": "#6EA8FE",
      "editorIndentGuide.background1": "#343A45",
      "editorIndentGuide.activeBackground1": "#596273",
      "editorWhitespace.foreground": "#343A45",
      "editorGutter.background": "#1D2026",
      "editorWidget.background": "#242830",
      "editorWidget.border": "#363C47",
      "editorHoverWidget.background": "#242830",
      "editorHoverWidget.border": "#363C47",
      "diffEditor.insertedTextBackground": "#2EA04355",
      "diffEditor.removedTextBackground": "#F8514955",
      "diffEditor.insertedLineBackground": "#1F3D2A",
      "diffEditor.removedLineBackground": "#482629",
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
