import assert from "node:assert/strict";
import { after, before, test } from "node:test";
import { fileURLToPath } from "node:url";
import { createElement, type ComponentType } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { createServer, type ViteDevServer } from "vite";
import type { Snapshot } from "../../contracts/run.ts";

let server: ViteDevServer;
let ChatMessages: ComponentType<{
  snapshot: Snapshot | null;
  sessionID: string | null;
}>;

before(async () => {
  // 复用项目的 TSX / 别名配置，只渲染 HTML，不启动网络监听。
  server = await createServer({
    root: fileURLToPath(new URL("..", import.meta.url)),
    server: { middlewareMode: true, hmr: false, watch: null },
  });
  ({ ChatMessages } = await server.ssrLoadModule("/src/chat-messages.tsx"));
});
after(async () => {
  await server?.close();
});

test("long history finds run statuses with linear work", () => {
  const count = 400;
  let runReads = 0;
  const snapshot: Snapshot = {
    seqEpoch: "epoch",
    updateSeq: 0,
    entries: Array.from({ length: count }, (_, index) => ({
      id: `entry-${index}`,
      seq: index + 1,
      message: {
        get runID() {
          runReads++;
          return `run-${Math.floor(index / 2)}`;
        },
        role: index % 2 ? "assistant" : "user",
        blocks: [{ kind: "text", text: "普通消息" }],
      },
    })),
    runs: Array.from({ length: count / 2 }, (_, index) => ({
      runID: `run-${index}`,
      afterEntrySeq: index * 2 + 1,
      status: "success",
    })),
  };
  const html = renderToStaticMarkup(
    createElement(ChatMessages, { sessionID: "session", snapshot }),
  );
  assert.equal((html.match(/data-entry-id=/g) ?? []).length, count);
  assert.equal((html.match(/已完成/g) ?? []).length, count / 2);
  // 计访问次数而非耗时，避免机器快慢影响回归；每条消息留足常数次访问。
  assert.ok(runReads <= count * 20, `repeated history scans: ${runReads}`);
});

test("status stays after the last visible message, including drafts and legacy runs", () => {
  const snapshot: Snapshot = {
    seqEpoch: "epoch",
    updateSeq: 3,
    entries: [
      {
        id: "old",
        seq: 1,
        message: {
          runID: "legacy",
          role: "assistant",
          blocks: [{ kind: "text", text: "旧回答" }],
        },
      },
      {
        id: "user",
        seq: 2,
        message: {
          runID: "live",
          role: "user",
          blocks: [{ kind: "text", text: "问题" }],
        },
      },
    ],
    runs: [
      {
        runID: "live",
        afterEntrySeq: 2,
        status: "running",
        drafts: [
          {
            entryID: "draft",
            afterEntrySeq: 2,
            blocks: [{ kind: "text", text: "半句" }],
          },
        ],
      },
      { runID: "orphan", afterEntrySeq: 3, status: "interrupted" },
    ],
  };
  const html = renderToStaticMarkup(
    createElement(ChatMessages, { sessionID: "session", snapshot }),
  );
  assert.match(html, /旧回答[\s\S]*状态未记录/);
  assert.equal((html.match(/运行中/g) ?? []).length, 1);
  assert.match(html, /data-entry-id="draft"[\s\S]*半句[\s\S]*运行中/);
  assert.equal((html.match(/运行已中断/g) ?? []).length, 1);
  assert.ok(
    html.indexOf('data-entry-id="user"') <
      html.indexOf('data-entry-id="draft"'),
  );
});
