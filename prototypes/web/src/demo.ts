export type RunStatus =
  "running" | "stopping" | "completed" | "failed" | "stopped";
export type Activity = {
  id: string;
  kind: "progress" | "tool" | "reasoning" | "steer";
  text: string;
  detail?: string;
};
export type Attachment = { id: string; name: string; url: string };
export type Turn = {
  id: string;
  prompt: string;
  images: Attachment[];
  status: RunStatus;
  tick: number;
  activity: Activity[];
  answer: string;
};
export type Session = {
  id: string;
  project: string;
  title: string;
  draft: string;
  images: Attachment[];
  agent: string;
  model: string;
  effort: string;
  turns: Turn[];
};
export type Agent = {
  id: string;
  name: string;
  prompt: string;
  tools: string[];
};

export const models = [
  {
    id: "vision",
    name: "视觉模型",
    description: "演示配置 · 支持图片",
    vision: true,
    efforts: ["低", "中", "高"],
  },
  {
    id: "text",
    name: "文字模型",
    description: "演示配置 · 仅文字",
    vision: false,
    efforts: ["关闭", "标准"],
  },
];
export const initialAgents: Agent[] = [
  {
    id: "default",
    name: "默认 Agent",
    prompt: "用清晰、简洁的方式协助用户完成任务。修改前先理解需求。",
    tools: ["读取文件", "编辑文件", "运行命令"],
  },
  {
    id: "review",
    name: "代码审查",
    prompt: "先理解改动，再指出有证据的问题。不要无关重构。",
    tools: ["读取文件", "运行命令"],
  },
];
export const sampleActivity: Activity[] = [
  {
    id: "a1",
    kind: "progress",
    text: "我先看一下现有页面和组件，再把视觉规则统一起来。",
  },
  {
    id: "a2",
    kind: "tool",
    text: "已读取 ProductDefine.md",
    detail:
      "# 产品定义\n\n项目 / 会话侧栏 | 聊天 | 辅助工作区\n\n运行时展开工作过程，完成后自动收起。\n最终回答始终保留。\n\n模型与思考使用同一个菜单。\n上下文用量放在菜单左侧。",
  },
  {
    id: "a3",
    kind: "reasoning",
    text: "思考摘要",
    detail:
      "这是演示用摘要，不是模型的真实思考。\n先统一语义 Token，再用公共组件组织交互，可以减少各处样式漂移。",
  },
  {
    id: "a4",
    kind: "tool",
    text: "已搜索界面组件",
    detail:
      "$ rg --files src/components\nbutton.tsx\ndialog.tsx\npopover.tsx\nselect.tsx\ncollapsible.tsx\ntextarea.tsx\n\n6 个匹配结果",
  },
  {
    id: "a5",
    kind: "tool",
    text: "已运行组件检查",
    detail:
      "$ npm run build\n\n检查 TypeScript…\n检查组件引用…\n检查交互状态…\n\n所有检查通过。\n\n以上为原型示例输出，不是真实命令执行。",
  },
  {
    id: "a6",
    kind: "progress",
    text: "布局已经理顺。接下来把输入区和工作过程的交互接起来。",
  },
  {
    id: "a7",
    kind: "tool",
    text: "已编辑输入区与工作过程",
    detail:
      "修改摘要（示例）\n\n+ 模型和思考合并在一个菜单\n+ 运行中发送直接调整方向\n+ 任务完成后收起过程\n+ 工具详情使用限高滚动区域\n\n没有添加等待队列。",
  },
];
export const sampleAnswer =
  "已把界面整理成三个区域：项目会话、聊天和辅助工作区。\n\n输入区的模型与思考合并在同一个菜单，上下文用量放在它的左边。运行中发送会直接调整当前任务，不会排队。\n\n工作过程保留三级展开，完成后自动收起。你可以展开上方的过程，查看每一步的示例工具输出。";
export function newSession(project: string): Session {
  return {
    id: crypto.randomUUID(),
    project,
    title: "新对话",
    draft: "",
    images: [],
    agent: "default",
    model: "vision",
    effort: "中",
    turns: [],
  };
}
export function demoTurn(status: RunStatus = "completed"): Turn {
  return {
    id: crypto.randomUUID(),
    prompt: "我们一起把 Harness 的界面整理得更统一、好用一些。",
    images: [],
    status,
    tick: status === "running" ? 2 : 10,
    activity:
      status === "running" ? sampleActivity.slice(0, 2) : sampleActivity,
    answer: status === "completed" ? sampleAnswer : "",
  };
}
export const firstSession = {
  ...newSession("Harness"),
  title: "一起设计新的工作空间",
  turns: [demoTurn()],
};
