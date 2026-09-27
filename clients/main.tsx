import React from "react";
import ReactDOM from "react-dom/client";
import "@fontsource-variable/jetbrains-mono";
import "@fontsource-variable/noto-sans-sc";
import App from "./ui/src/App";
import "./ui/src/styles.css";
import { platform as webPlatform } from "./web/platform";

async function main() {
  // 入口只选择一次接入方式，共用 UI 不感知 Wails 或 WebSocket 地址。
  const desktop = location.protocol === "wails:" || location.hostname === "wails.localhost";
  const platform = desktop ? (await import("./desktop/platform")).platform : webPlatform;
  ReactDOM.createRoot(document.getElementById("root")!).render(
    <React.StrictMode>
      <App platform={platform} />
    </React.StrictMode>,
  );
}

void main();
