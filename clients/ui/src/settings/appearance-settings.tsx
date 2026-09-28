import { useState } from "react";
import { contentFonts, codeFonts, selectFonts, useFonts } from "../fonts";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@/components/ui/select";
import { Check, Monitor, Moon, Sun } from "../icons";

export function ThemeOptions({ theme, setTheme }: {
  theme: string;
  setTheme: (theme: string) => void;
}) {
  return (
    <div className="theme-grid" role="group" aria-label="界面主题">
          {[
            { id: "light", name: "浅色", Icon: Sun },
            { id: "dark", name: "深色", Icon: Moon },
            { id: "system", name: "跟随系统", Icon: Monitor },
          ].map((item) => (
            <button
              key={item.id}
              className={`theme-option ui-focus ${theme === item.id ? "active" : ""}`}
              onClick={() => setTheme(item.id)}
              aria-pressed={theme === item.id}
            >
              <span className={`theme-preview theme-preview-${item.id}`} aria-hidden="true">
                <span className="theme-preview-sidebar"><i /><i /><i /></span>
                <span className="theme-preview-content"><i /><i /><span /></span>
              </span>
              <span className="theme-option-label"><item.Icon /><span>{item.name}</span>
                <span className="theme-check">{theme === item.id && <Check />}</span>
              </span>
            </button>
          ))}
    </div>
  );
}

export function FontOptions() {
  const fonts = useFonts();
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  async function change(kind: "content" | "code", value: string) {
    setLoading(true);
    setError("");
    try { await selectFonts({ ...fonts, [kind]: value }); }
    catch { setError("字体加载失败，已保留原字体。请重试。"); }
    finally { setLoading(false); }
  }
  return <section className="settings-section font-settings" aria-label="字体">
    <h3>字体</h3>
    {([{ kind: "content", label: "内容字体", options: contentFonts }, { kind: "code", label: "代码字体", options: codeFonts }] as const).map(({ kind, label, options }) =>
      <div className="font-setting" key={kind}>
        <div className="font-setting-heading"><label id={`font-${kind}`}>{label}</label>
          <Select value={fonts[kind]} disabled={loading} onValueChange={(value) => void change(kind, value)}>
            <SelectTrigger aria-labelledby={`font-${kind}`}><SelectValue /></SelectTrigger>
            <SelectContent>{options.map((font) => <SelectItem key={font.id} value={font.id}>{font.label}</SelectItem>)}</SelectContent>
          </Select>
        </div>
        <div className={`font-preview font-preview-${kind}`}>{kind === "content" ? "把想法变成作品。Build something meaningful. 0123456789" : "const message = '你好，EDITH'; // 0O 1lI => {}"}</div>
      </div>)}
    {loading && <p className="settings-description" role="status">正在加载内置字体…</p>}
    {error && <p className="settings-notice" role="alert">{error}</p>}
  </section>;
}
