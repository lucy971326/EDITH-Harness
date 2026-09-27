import { Check, Monitor, Moon, Palette, Sun } from "../icons";

export function AppearanceSettingsPanel({ theme, setTheme }: {
  theme: string;
  setTheme: (theme: string) => void;
}) {
  return (
    <>
      <header className="settings-heading"><h2>外观</h2></header>
      <div className="settings-two-pane">
        <aside className="settings-subnav" aria-label="外观项目">
          <h3>外观</h3>
          <div className="settings-subnav-list">
            <div className="settings-subnav-item" aria-current="page">
              <Palette />
              <span className="settings-subnav-copy"><span className="settings-subnav-name">界面主题</span></span>
            </div>
          </div>
        </aside>
        <section className="settings-detail-pane">
          <div className="settings-detail-title-row"><div className="settings-identity"><span className="settings-identity-icon"><Palette /></span><h3>界面主题</h3></div></div>
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
        </section>
      </div>
    </>
  );
}
