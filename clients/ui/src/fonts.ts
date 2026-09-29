import { useSyncExternalStore } from "react";

export const contentFonts = [
  { id: "misans", label: "MiSans", family: '"EDITH MiSans"' },
  { id: "harmony", label: "HarmonyOS Sans", family: '"EDITH HarmonyOS Sans"' },
] as const;
export const codeFonts = [
  { id: "jetbrains", label: "JetBrains Mono", family: '"JetBrains Mono Variable"' },
  { id: "cascadia", label: "Cascadia Mono", family: '"EDITH Cascadia Mono"' },
  { id: "maple", label: "Maple Mono", family: '"EDITH Maple Mono"' },
] as const;
type FontPreferences = { content: string; code: string };
const storageKey = "harness-web:fonts";
const defaults: FontPreferences = { content: "misans", code: "jetbrains" };
let current = defaults;
let revision = 0;
const listeners = new Set<() => void>();

export function useFonts() {
  return useSyncExternalStore((listener) => {
    listeners.add(listener);
    return () => { listeners.delete(listener); };
  }, () => current);
}

// 加载成功才切换；并发选择只应用最后一次，失败保留正在使用的内置字体。
export async function selectFonts(next: FontPreferences) {
  const ownRevision = ++revision;
  const content = contentFonts.find((font) => font.id === next.content) ?? contentFonts[0];
  const code = codeFonts.find((font) => font.id === next.code) ?? codeFonts[0];
  const chinese = code.id === "cascadia" ? "EDITH Code SC Cascadia" : "EDITH Code SC";
  const loaded = await Promise.all([
    document.fonts.load(`400 15px ${content.family}`, "EDITH 你好，世界"),
    document.fonts.load(`700 15px ${content.family}`, "EDITH 标题"),
    document.fonts.load(`400 13px ${code.family}`, "0O1lI{}=>"),
    document.fonts.load(`400 13px "${chinese}"`, "中文，注释"),
  ]);
  if (loaded.some((faces) => faces.length === 0)) throw new Error("字体未能加载，请重试");
  if (ownRevision !== revision) return;
  const root = document.documentElement;
  root.style.setProperty("--font-content", content.family);
  root.style.setProperty("--font-code", `${code.family}, "${chinese}"`);
  current = { content: content.id, code: code.id };
  try { localStorage.setItem(storageKey, JSON.stringify(current)); } catch { /* 本次选择仍生效。 */ }
  for (const listener of listeners) listener();
  window.dispatchEvent(new Event("edith-fonts-changed"));
}

export async function initializeFonts() {
  let saved = defaults;
  try { saved = { ...defaults, ...JSON.parse(localStorage.getItem(storageKey) ?? "{}") }; } catch { /* 损坏偏好恢复默认。 */ }
  await Promise.all([
    document.fonts.load('400 14px "EDITH MiSans"', "设置会话"),
    selectFonts(saved).catch(() => selectFonts(defaults)),
  ]);
}
