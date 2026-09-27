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

export const editorFontSize = Number.parseFloat(
  getComputedStyle(document.documentElement).getPropertyValue("--font-size-code"),
);

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

// Monaco 不解析 CSS 变量；在挂载和主题切换时读取当前 Token，不重建编辑器或 Model。
export function defineEditorThemes(api: Monaco) {
  const dark = document.documentElement.classList.contains("dark");
  const style = getComputedStyle(document.documentElement);
  const color = (name: string) => style.getPropertyValue(name).trim();
  api.editor.defineTheme(dark ? "harness-dark" : "harness-light", {
    base: dark ? "vs-dark" : "vs",
    inherit: true,
    rules: [
      { token: "comment", foreground: color("--code-comment").slice(1) },
      { token: "keyword", foreground: color("--code-keyword").slice(1) },
      { token: "string", foreground: color("--code-string").slice(1) },
      { token: "number", foreground: color("--code-number").slice(1) },
      { token: "type", foreground: color("--code-type").slice(1) },
      { token: "type.identifier", foreground: color("--code-type").slice(1) },
    ],
    colors: {
      "editor.background": color("--card"),
      "editor.foreground": color("--foreground"),
      "editorLineNumber.foreground": color("--code-line-number"),
      "editorLineNumber.activeForeground": color("--foreground"),
      "editor.selectionBackground": color("--selection"),
      "editor.inactiveSelectionBackground": color("--selection-inactive"),
      "editor.lineHighlightBackground": color("--muted"),
      "editorCursor.foreground": color("--ring"),
      "editorIndentGuide.background1": color("--code-indent"),
      "editorIndentGuide.activeBackground1": color("--border-strong"),
      "editorWhitespace.foreground": color("--input"),
      "editorGutter.background": color("--card"),
      "editorWidget.background": color("--popover"),
      "editorWidget.border": color("--border"),
      "editorHoverWidget.background": color("--popover"),
      "editorHoverWidget.border": color("--border"),
      "editorSuggestWidget.background": color("--popover"),
      "editorSuggestWidget.border": color("--border"),
      "editorSuggestWidget.selectedBackground": color("--secondary"),
      "menu.background": color("--popover"),
      "menu.foreground": color("--foreground"),
      "menu.selectionBackground": color("--accent"),
      "menu.selectionForeground": color("--foreground"),
      "menu.separatorBackground": color("--border"),
      "focusBorder": color("--ring"),
      "diffEditor.insertedTextBackground": color("--diff-added-text"),
      "diffEditor.removedTextBackground": color("--diff-removed-text"),
      "diffEditor.insertedLineBackground": color("--diff-added-background"),
      "diffEditor.removedLineBackground": color("--diff-removed-background"),
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
    defineEditorThemes(monaco);
    const observer = new MutationObserver(() => {
      defineEditorThemes(monaco);
      setDark(document.documentElement.classList.contains("dark"));
    });
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
