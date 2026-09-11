import type { RunStatus } from "./demo";

// 浏览器支持 WebMCP 时，复用界面里的状态演示动作；不接触真实会话。
type DemoState = RunStatus | "empty";
type ModelContext = {
  registerTool: (
    tool: {
      name: string;
      description: string;
      inputSchema: object;
      execute: (input: unknown) => Promise<object>;
    },
    options: { signal: AbortSignal },
  ) => void | Promise<void>;
};

export function registerDemoTool(select: (state: DemoState) => void) {
  const context = (document as Document & { modelContext?: ModelContext })
    .modelContext;
  if (!context) return;
  const lifecycle = new AbortController();
  const states: DemoState[] = [
    "empty",
    "running",
    "completed",
    "failed",
    "stopped",
  ];
  Promise.resolve().then(
    () => context.registerTool(
      {
        name: "show_harness_demo_state",
        description:
          "切换当前 Harness 原型会话的演示状态，替换当前示例消息，不修改真实数据。",
        inputSchema: {
          type: "object",
          properties: { state: { type: "string", enum: states } },
          required: ["state"],
          additionalProperties: false,
        },
        async execute(input) {
          const state = (input as { state?: DemoState } | null)?.state;
          if (!state || !states.includes(state))
            throw new Error("请选择有效的原型状态");
          select(state);
          await new Promise<void>((resolve) =>
            requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
          );
          return { state, prototypeOnly: true };
        },
      },
      { signal: lifecycle.signal },
    ),
  ).catch((error) => console.warn("原型演示工具注册失败", error));
  return () => lifecycle.abort();
}
