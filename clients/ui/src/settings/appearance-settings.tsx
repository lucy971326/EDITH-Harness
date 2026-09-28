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
