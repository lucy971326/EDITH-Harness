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

test("legacy answers render normally while live and interrupted statuses stay visible", () => {
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
  assert.match(html, /class="answer"[\s\S]*旧回答/);
  assert.doesNotMatch(html, /状态未记录/);
  assert.match(html, /半句[\s\S]*运行中/);
  assert.match(html, /运行已中断/);
  assert.ok(html.indexOf('data-entry-id="user"') < html.indexOf("半句"));
});

test("Markdown supports structure but removes HTML and unsafe links", () => {
  const snapshot: Snapshot = {
    seqEpoch: "e",
    updateSeq: 0,
    runs: [{ runID: "r", status: "success", afterEntrySeq: 1 }],
    entries: [
      {
        id: "a",
        seq: 2,
        message: {
          runID: "r",
          role: "assistant",
          blocks: [
            {
              kind: "text",
              text: "**粗体**\n\n- 列表\n\n[x](javascript:alert%281%29)\n\n<script>alert(1)</script>\n\n![远程](https://example.com/track.png)",
            },
          ],
        },
      },
    ],
  };
  const html = renderToStaticMarkup(
    createElement(ChatMessages, { sessionID: "s", snapshot }),
  );
  assert.match(html, /<strong>粗体<\/strong>/);
  assert.match(html, /<li>列表<\/li>/);
  assert.doesNotMatch(html, /<script|javascript:|<img/);
});
