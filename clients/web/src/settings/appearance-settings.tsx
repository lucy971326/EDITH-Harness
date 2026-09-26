import { Check } from "../icons";

export function AppearanceSettingsPanel({ theme, setTheme }: {
  theme: string;
  setTheme: (theme: string) => void;
}) {
  return (
    <>
      <header className="settings-heading">
        <h2>外观</h2>
        <p>调整工作台的显示方式。</p>
      </header>
      <section className="settings-preference-row">
        <div className="settings-preference-label">
          <h3>界面主题</h3>
          <p>选择明暗风格，或随系统自动切换。</p>
        </div>
        <div className="theme-grid" role="group" aria-label="界面主题">
          {[
            { id: "light", name: "浅色" },
            { id: "dark", name: "深色" },
            { id: "system", name: "跟随系统" },
          ].map((item) => (
            <button
              key={item.id}
              className={`theme-option ${theme === item.id ? "active" : ""}`}
              onClick={() => setTheme(item.id)}
              aria-pressed={theme === item.id}
            >
              <span className={`theme-preview theme-preview-${item.id}`} aria-hidden="true">
                <span className="theme-preview-sidebar"><i /><i /><i /></span>
                <span className="theme-preview-content"><i /><i /><span /></span>
              </span>
              <span className="theme-option-label"><span>{item.name}</span>
                <span className="theme-check">{theme === item.id && <Check />}</span>
              </span>
            </button>
          ))}
        </div>
      </section>
    </>
  );
}
