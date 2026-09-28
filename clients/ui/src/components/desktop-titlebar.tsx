import { useEffect, useState } from "react";
import { Minus, Square, X } from "../icons";
import type { DesktopWindowControls } from "../types";

function RestoreWindowIcon() {
  return <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
    <path d="M8 6h10v10" />
    <rect x="5" y="9" width="11" height="10" rx="1" />
  </svg>;
}

export function DesktopTitlebar({ controls }: { controls?: DesktopWindowControls }) {
  const [maximized, setMaximized] = useState(false);
  useEffect(() => controls?.onMaximizedChange(setMaximized), [controls]);

  function invoke(action: () => Promise<void>) {
    void action().catch((error) => console.warn("窗口操作失败", error));
  }

  return <header className="desktop-titlebar" data-native-controls={!controls} aria-label="EDITH 窗口">
    <div className="desktop-titlebar-brand">
      <img src="/edith-icon.svg" alt="" draggable={false} />
      <span>EDITH</span>
    </div>
    {controls && <div className="desktop-window-controls">
      <button type="button" className="desktop-window-button" aria-label="最小化窗口" title="最小化" onClick={() => invoke(controls.minimize)}><Minus /></button>
      <button type="button" className="desktop-window-button" aria-label={maximized ? "还原窗口" : "最大化窗口"} title={maximized ? "还原" : "最大化"} onClick={() => invoke(controls.toggleMaximize)}>
        {maximized ? <RestoreWindowIcon /> : <Square />}
      </button>
      <button type="button" className="desktop-window-button desktop-window-close" aria-label="关闭窗口" title="关闭" onClick={() => invoke(controls.close)}><X /></button>
    </div>}
  </header>;
}
