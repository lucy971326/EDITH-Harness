import { useEffect, useRef } from "react";
import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import type { RPCClient } from "../client/rpc";

const encoder = new TextEncoder();

export function TerminalView({
  processID,
  workspace,
  active,
  client,
}: {
  processID: string;
  workspace: string;
  active: boolean;
  client: RPCClient;
}) {
  const host = useRef<HTMLDivElement>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const activeRef = useRef(active);
  activeRef.current = active;

  useEffect(() => {
    if (!host.current) return;

    const terminal = new Terminal({
      cursorBlink: true,
      convertEol: false,
      fontFamily: terminalFont(),
      fontSize: 13,
      lineHeight: 1.18,
      scrollback: 5000,
      theme: terminalTheme(),
    });
    const fit = new FitAddon();
    terminal.loadAddon(fit);
    terminal.open(host.current);
    fit.fit();
    terminalRef.current = terminal;
    fitRef.current = fit;

    const themeObserver = new MutationObserver(() => {
      terminal.options.theme = terminalTheme();
    });
    themeObserver.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["class"],
    });

    let mounted = true;
    let running = true;
    let ready = false;
    let lastRows = terminal.rows;
    let lastCols = terminal.cols;
    let resizeTimer: ReturnType<typeof setTimeout> | undefined;
    let writes: Promise<unknown> = Promise.resolve();
    const pendingInput: Uint8Array[] = [];

    function sendInput(data: Uint8Array) {
      const encoded = encodeBase64(data);
      writes = writes
        .then(() => client.writeTerminal(processID, encoded))
        .catch(() => undefined);
    }

    const stopOutput = client.onCommandOutput(processID, (notification) => {
      terminal.write(decodeBase64(notification.deltaBase64));
      if (ready) return;
      ready = true;
      fit.fit();
      lastRows = terminal.rows;
      lastCols = terminal.cols;
      void client.resizeTerminal(processID, lastRows, lastCols).catch(() => {});
      for (const data of pendingInput) sendInput(data);
      pendingInput.length = 0;
    });
    const input = terminal.onData((data) => {
      if (!running) return;
      const bytes = encoder.encode(data);
      if (!ready) {
        pendingInput.push(bytes);
        return;
      }
      sendInput(bytes);
    });
    const observer = new ResizeObserver(() => {
      if (!activeRef.current || !running || !ready) return;
      clearTimeout(resizeTimer);
      resizeTimer = setTimeout(() => {
        if (!activeRef.current || !running || !ready) return;
        fit.fit();
        if (terminal.rows === lastRows && terminal.cols === lastCols) return;
        lastRows = terminal.rows;
        lastCols = terminal.cols;
        void client.resizeTerminal(processID, lastRows, lastCols).catch(() => {});
      }, 50);
    });
    observer.observe(host.current);

    // 延后一拍启动，避免 React StrictMode 的开发期 effect 重放创建两次进程。
    const startTimer = setTimeout(() => {
      void client
        .execTerminal(processID, workspace, terminal.rows, terminal.cols)
        .then(({ exitCode }) => {
          running = false;
          if (!mounted) return;
          terminal.write(
            `\r\n\x1b[90m[进程已退出，代码 ${exitCode}]\x1b[0m\r\n`,
          );
        })
        .catch(() => {
          running = false;
          if (!mounted) return;
          terminal.write("\r\n\x1b[90m[终端连接已结束]\x1b[0m\r\n");
        });
    }, 0);

    return () => {
      mounted = false;
      running = false;
      clearTimeout(startTimer);
      clearTimeout(resizeTimer);
      observer.disconnect();
      themeObserver.disconnect();
      input.dispose();
      stopOutput();
      terminalRef.current = null;
      fitRef.current = null;
      terminal.dispose();
    };
  }, [client, processID, workspace]);

  useEffect(() => {
    if (!active) return;
    const frame = requestAnimationFrame(() => {
      fitRef.current?.fit();
      terminalRef.current?.focus();
    });
    return () => cancelAnimationFrame(frame);
  }, [active]);

  return <div className="terminal-host" ref={host} />;
}

function terminalTheme() {
  const style = getComputedStyle(document.documentElement);
  return {
    background: style.getPropertyValue("--card").trim(),
    foreground: style.getPropertyValue("--foreground").trim(),
    cursor: style.getPropertyValue("--foreground").trim(),
    selectionBackground: style.getPropertyValue("--secondary").trim(),
  };
}

function terminalFont(): string {
  return getComputedStyle(document.documentElement)
    .getPropertyValue("--font-code")
    .trim();
}

function encodeBase64(data: Uint8Array): string {
  let binary = "";
  for (const byte of data) binary += String.fromCharCode(byte);
  return btoa(binary);
}

function decodeBase64(value: string): Uint8Array {
  const binary = atob(value);
  const data = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index++) {
    data[index] = binary.charCodeAt(index);
  }
  return data;
}
