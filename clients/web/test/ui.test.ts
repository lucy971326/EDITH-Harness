import assert from "node:assert/strict";
import { after, before, test } from "node:test";
import { fileURLToPath } from "node:url";
import { createElement, type ComponentType, type ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { createServer, type ViteDevServer } from "vite";
import type { SessionView } from "../../contracts/harness.ts";
import type { ModelChoice } from "../../contracts/appserver.ts";
import type { ModelSelection } from "../src/model-menu.tsx";
import type { Attachment } from "../src/composer.tsx";

let server: ViteDevServer;
let TooltipProvider: ComponentType<{ children?: ReactNode }>;
let Sidebar: ComponentType<Record<string, unknown>>;
let Composer: ComponentType<Record<string, unknown>>;
let isComposerSubmitKey: (event: {
  key: string;
  shiftKey: boolean;
  keyCode: number;
  nativeEvent: { isComposing: boolean };
}) => boolean;
let chatSendParams: (
  sessionID: string,
  text: string,
  selection: ModelSelection,
  expectedRunID?: string,
) => {
  sessionID: string;
  text: string;
  model?: string;
  reasoningEffort?: string;
  expectedRunID?: string;
};
let shouldClearSubmittedDraft: (
  versions: Map<string, number>,
  sessionID: string,
  submittedVersion: number,
) => boolean;

before(async () => {
  server = await createServer({
    root: fileURLToPath(new URL("..", import.meta.url)),
    server: { middlewareMode: true, hmr: { port: 24679 }, watch: null },
  });
  ({ TooltipProvider } = await server.ssrLoadModule(
    "/src/components/ui/tooltip.tsx",
  ));
  ({ Sidebar } = await server.ssrLoadModule("/src/sidebar.tsx"));
  ({ Composer, isComposerSubmitKey } = await server.ssrLoadModule(
    "/src/composer.tsx",
  ));
  ({ chatSendParams, shouldClearSubmittedDraft } = await server.ssrLoadModule(
    "/src/App.tsx",
  ));
});
after(async () => {
  await server?.close();
});

function session(
  id: string,
  workspace: string,
  title = id,
): SessionView {
  return {
    sessionID: id,
    title,
    createdAt: "2026-01-01T00:00:00Z",
    settings: {
      agentID: "default",
      model: "",
      reasoningEffort: "",
      workspace,
    },
  };
}

function renderSidebar(props: Record<string, unknown>) {
  return renderToStaticMarkup(
    createElement(
      TooltipProvider,
      null,
      createElement(Sidebar, {
        connection: "connected",
        backendBusy: false,
        sessions: [session("keep", "/tmp/alpha", "保留草稿")],
        listError: "",
        selectedID: "keep",
        settingsOpen: false,
        onClose() {},
        onOpenProject() {},
        onReload() {},
        onSelect() {},
        onCreate() {},
        onOpenSettings() {},
        onReconnect() {},
        ...props,
      }),
    ),
  );
}

const idleComposer = {
  draft: "你好",
  images: [] as Attachment[],
  notice: "",
  agentLabel: "default",
  running: false,
  stopping: false,
  busySending: false,
  canSend: true,
  stopDisabled: true,
  modelDisabled: false,
  validModel: true,
  models: [
    { id: "demo", reasoningEfforts: ["high"] },
  ] as ModelChoice[],
  modelSelection: { model: "demo", reasoningEffort: "high" },
  modelError: "",
  onDraftChange() {},
  onSend() {},
  onStop() {},
  onAddImages() {},
  onRemoveImage() {},
  onModelChange() {},
  onRetryModels() {},
  onDismissNotice() {},
};

function renderComposer(props: Record<string, unknown>) {
  return renderToStaticMarkup(
    createElement(
      TooltipProvider,
      null,
      createElement(Composer, { ...idleComposer, ...props }),
    ),
  );
}

test("busy send includes expectedRunID; idle send omits it", () => {
  const selection = { model: "demo", reasoningEffort: "high" };
  assert.deepEqual(chatSendParams("s", "插话", selection, "run-1"), {
    sessionID: "s",
    text: "插话",
    model: "demo",
    reasoningEffort: "high",
    expectedRunID: "run-1",
  });
  assert.deepEqual(chatSendParams("s", "新一轮", selection), {
    sessionID: "s",
    text: "新一轮",
    model: "demo",
    reasoningEffort: "high",
  });
  assert.equal(
    "expectedRunID" in chatSendParams("s", "新一轮", selection),
    false,
  );
});

test("send confirmation only clears the submitted draft version", () => {
  const versions = new Map<string, number>([
    ["one", 2],
    ["two", 4],
  ]);
  assert.equal(shouldClearSubmittedDraft(versions, "one", 2), true);
  assert.equal(shouldClearSubmittedDraft(versions, "one", 1), false);
  versions.set("one", 3);
  assert.equal(shouldClearSubmittedDraft(versions, "one", 2), false);
  assert.equal(shouldClearSubmittedDraft(versions, "two", 4), true);
});

test("Enter submits; Shift+Enter, IME composing and keyCode 229 do not", () => {
  const enter = {
    key: "Enter",
    shiftKey: false,
    keyCode: 13,
    nativeEvent: { isComposing: false },
  };
  assert.equal(isComposerSubmitKey(enter), true);
  assert.equal(isComposerSubmitKey({ ...enter, shiftKey: true }), false);
  assert.equal(
    isComposerSubmitKey({ ...enter, nativeEvent: { isComposing: true } }),
    false,
  );
  assert.equal(isComposerSubmitKey({ ...enter, keyCode: 229 }), false);
  assert.equal(isComposerSubmitKey({ ...enter, key: "Tab" }), false);
});

test("sidebar highlights the selected session and disables project actions offline", () => {
  const html = renderSidebar({
    connection: "disconnected",
    settingsOpen: false,
  });
  assert.match(html, /session-row selected/);
  assert.match(html, /保留草稿/);
  assert.match(html, /disabled[^>]*>[\s\S]*打开项目/);
  assert.match(html, /重新连接/);
  const settings = renderSidebar({ settingsOpen: true });
  assert.match(settings, /设置/);
  assert.equal(settings.includes("session-row selected"), false);
});

test("composer keeps image preview, model menu and stop button states", () => {
  const withImage = renderComposer({
    images: [{ id: "img-1", name: "shot.png", url: "blob:preview" }],
    canSend: false,
  });
  assert.match(withImage, /alt="shot.png"/);
  assert.match(withImage, /aria-label="移除图片 shot.png"/);
  assert.match(withImage, /aria-label="模型与思考"/);
  assert.match(withImage, /disabled[^>]*>[\s\S]*发送消息/);
  assert.equal(withImage.includes("停止任务"), false);

  const running = renderComposer({
    running: true,
    stopping: false,
    canSend: true,
    stopDisabled: false,
  });
  assert.match(running, /aria-label="停止任务"/);
  assert.match(running, /aria-label="调整当前任务"/);
  assert.match(running, /Enter 调整当前任务 · 不排队/);

  const stopping = renderComposer({
    running: true,
    stopping: true,
    stopDisabled: true,
    busySending: false,
  });
  assert.match(stopping, /aria-label="停止中"/);
  assert.match(stopping, /停止中，等待后台收尾…/);
});
