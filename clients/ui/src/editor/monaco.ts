import { useEffect, useState } from "react";
import { loader, type Monaco } from "@monaco-editor/react";
import * as monaco from "monaco-editor";
import EditorWorker from "monaco-editor/editor/editor.worker.js?worker";
import JSONWorker from "monaco-editor/language/json/json.worker.js?worker";
import CSSWorker from "monaco-editor/language/css/css.worker.js?worker";
import HTMLWorker from "monaco-editor/language/html/html.worker.js?worker";
import TypeScriptWorker from "monaco-editor/language/typescript/ts.worker.js?worker";
import { monacoColor } from "./theme";

self.MonacoEnvironment = {
  getWorker: (_moduleId, label) => {
    switch (label) {
      case "json": return new JSONWorker();
      case "css":
      case "scss":
      case "less": return new CSSWorker();
      case "html":
      case "handlebars":
      case "razor": return new HTMLWorker();
      case "javascript":
      case "typescript": return new TypeScriptWorker();
      default: return new EditorWorker();
    }
  },
};
loader.config({ monaco });

export let editorFontFamily =
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
  const color = (name: string) => monacoColor(style.getPropertyValue(name));
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
    // 内置字体与粗体资源加载后刷新字宽；切换字体不重建 Model。
    const remeasureFonts = () => monaco.editor.remeasureFonts();
    const updateFonts = () => {
      editorFontFamily = getComputedStyle(document.documentElement).getPropertyValue("--font-code").trim();
      for (const editor of monaco.editor.getEditors()) editor.updateOptions({ fontFamily: editorFontFamily });
      remeasureFonts();
    };
    window.addEventListener("edith-fonts-changed", updateFonts);
    updateFonts();
    document.fonts.addEventListener("loadingdone", remeasureFonts);
    remeasureFonts();
    defineEditorThemes(monaco);
    const observer = new MutationObserver(() => {
      defineEditorThemes(monaco);
      setDark(document.documentElement.classList.contains("dark"));
    });
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["class", "data-palette"],
    });
    return () => {
      observer.disconnect();
      window.removeEventListener("edith-fonts-changed", updateFonts);
      document.fonts.removeEventListener("loadingdone", remeasureFonts);
    };
  }, []);
  return dark ? "harness-dark" : "harness-light";
}
