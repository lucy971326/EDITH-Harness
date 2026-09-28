import React from "react";
import ReactDOM from "react-dom/client";
import "@fontsource-variable/jetbrains-mono";
import { initializeFonts } from "./ui/src/fonts";
import App from "./ui/src/App";
import "./ui/src/styles.css";
import { platform as webPlatform } from "./web/platform";

async function main() {
  // 入口只选择一次接入方式，共用 UI 不感知 Wails 或 WebSocket 地址。
  const desktop = location.protocol === "wails:" || location.hostname === "wails.localhost";
  const platform = desktop ? (await import("./desktop/platform")).platform : webPlatform;
  await initializeFonts();
  ReactDOM.createRoot(document.getElementById("root")!).render(
    <React.StrictMode>
      <App platform={platform} />
    </React.StrictMode>,
  );
}

void main().catch(() => {
  const root = document.getElementById("root");
  if (!root) return;
  root.replaceChildren();
  const message = document.createElement("p");
  message.textContent = "应用资源加载失败，请检查连接后重试。";
  const retry = document.createElement("button");
  retry.textContent = "重新加载";
  retry.onclick = () => location.reload();
  root.append(message, retry);
});
