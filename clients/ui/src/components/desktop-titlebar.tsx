import { useEffect, useState } from "react";
import { Minus, Square, X } from "../icons";
import { Hint } from "@/components/ui/tooltip";
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
      <Hint text="最小化"><button type="button" className="desktop-window-button" aria-label="最小化窗口" onClick={() => invoke(controls.minimize)}><Minus /></button></Hint>
      <Hint text={maximized ? "还原" : "最大化"}><button type="button" className="desktop-window-button" aria-label={maximized ? "还原窗口" : "最大化窗口"} onClick={() => invoke(controls.toggleMaximize)}>
        {maximized ? <RestoreWindowIcon /> : <Square />}
      </button></Hint>
      <Hint text="关闭"><button type="button" className="desktop-window-button desktop-window-close" aria-label="关闭窗口" onClick={() => invoke(controls.close)}><X /></button></Hint>
    </div>}
  </header>;
}
