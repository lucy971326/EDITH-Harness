import assert from "node:assert/strict";
import { after, before, test } from "node:test";
import { fileURLToPath } from "node:url";
import { createElement, type ComponentType, type ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { createServer, type ViteDevServer } from "vite";
import type { SessionView } from "../../contracts/harness.ts";
import type { ModelChoice } from "../../contracts/appserver.ts";
import type { Attachment } from "../src/composer.tsx";

let server: ViteDevServer;
let TooltipProvider: ComponentType<{ children?: ReactNode }>;
let Sidebar: ComponentType<Record<string, unknown>>;
let Composer: ComponentType<Record<string, unknown>>;
let composerTrigger: (value: string, cursor: number) => {
  prefix: "/" | "$";
  query: string;
  start: number;
  end: number;
} | null;
let isComposerSubmitKey: (event: {
  key: string;
  shiftKey: boolean;
  keyCode: number;
  nativeEvent: { isComposing: boolean };
}) => boolean;
let chatSendParams: (
  sessionID: string,
  text: string,
  images: Attachment[],
  expectedRunID?: string,
) => {
  sessionID: string;
  text?: string;
  images?: { mime: string; data: string }[];
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
  ({ Composer, composerTrigger, isComposerSubmitKey } =
    await server.ssrLoadModule("/src/composer.tsx"));
  ({ chatSendParams, shouldClearSubmittedDraft } =
    await server.ssrLoadModule("/src/App.tsx"));
});
after(async () => {
  await server?.close();
});

function session(id: string, workspace: string, title = id): SessionView {
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
  agents: [
    {
      id: "default",
      name: "Harness",
      kind: "react",
      systemPrompt: "",
      tools: [],
      inUse: true,
    },
  ],
  agentID: "default",
  settingsDisabled: false,
  usage: undefined,
  running: false,
  stopping: false,
  canSend: true,
  stopDisabled: true,
  modelDisabled: false,
  validModel: true,
  models: [{ id: "demo", reasoningEfforts: ["high"] }] as ModelChoice[],
  modelSelection: { model: "demo", reasoningEffort: "high" },
  modelError: "",
  imageDisabled: false,
  skills: [],
  commands: [],
  suggestionsDisabled: false,
  commandBusy: false,
  onDraftChange() {},
  onSend() {},
  onStop() {},
  onAddImages() {},
  onRemoveImage() {},
  onModelChange() {},
  onAgentChange() {},
  onRetryModels() {},
  async onCommand() {
    return true;
  },
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

test("send carries only content and expected run identity", () => {
  const image: Attachment = {
    id: "i",
    name: "a.webp",
    url: "blob:a",
    mime: "image/webp",
    data: "YWJj",
  };
  assert.deepEqual(chatSendParams("s", "插话", [image], "run-1"), {
    sessionID: "s",
    text: "插话",
    images: [{ mime: "image/webp", data: "YWJj" }],
    expectedRunID: "run-1",
  });
  assert.deepEqual(chatSendParams("s", "新一轮", []), {
    sessionID: "s",
    text: "新一轮",
  });
  assert.equal("expectedRunID" in chatSendParams("s", "新一轮", []), false);
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

test("slash offers commands and skills; dollar offers only skills", () => {
  assert.deepEqual(composerTrigger("先做 /com 后做", 7), {
    prefix: "/",
    query: "com",
    start: 3,
    end: 7,
  });
  assert.deepEqual(composerTrigger("$skill-creator", 6), {
    prefix: "$",
    query: "skill",
    start: 0,
    end: 14,
  });
  const catalog = {
    skills: [{ name: "review", description: "审查代码", scope: "workspace" }],
    commands: [{ name: "compact", description: "压缩对话" }],
  };
  const slash = renderComposer({ ...catalog, draft: "/" });
  assert.match(slash, /\/compact/);
  assert.match(slash, /\$review/);
  const dollar = renderComposer({ ...catalog, draft: "$" });
  assert.doesNotMatch(dollar, /\/compact/);
  assert.match(dollar, /\$review/);
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
    images: [
      {
        id: "img-1",
        name: "shot.png",
        url: "blob:preview",
        mime: "image/webp",
        data: "YWJj",
      },
    ],
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
  assert.doesNotMatch(running, /composer-caption/);

  const withUsage = renderComposer({
    usage: { inputTokens: 10, cacheReadTokens: 10, contextWindow: 100 },
  });
  assert.match(withUsage, /usage-ring/);
  assert.match(withUsage, /--usage-percent:20%/);
  assert.match(withUsage, /上下文已使用 20%/);
  assert.doesNotMatch(withUsage, /tokens/);

  const stopping = renderComposer({
    running: true,
    stopping: true,
    stopDisabled: true,
  });
  assert.match(stopping, /aria-label="停止中"/);
});
