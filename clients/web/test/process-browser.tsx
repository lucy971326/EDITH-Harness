// 仅供 Vite 开发模式下人工验收，不进入正式入口或生产构建。
import { useState } from "react";
import { createRoot } from "react-dom/client";
import { ChatMessages } from "../src/chat-messages";
import type { Snapshot } from "../../contracts/run";
import "../src/styles.css";

const initial: Snapshot = {
  updateSeq: 0,
  seqEpoch: "test",
  runs: [{ runID: "r", status: "running", afterEntrySeq: 1 }],
  entries: [
    {
      id: "u",
      seq: 1,
      message: {
        runID: "r",
        role: "user",
        blocks: [{ kind: "text", text: "请检查 **代码**" }],
      },
    },
    {
      id: "a",
      seq: 2,
      message: {
        runID: "r",
        role: "assistant",
        afterSeq: 1,
        blocks: [
          { kind: "text", text: "我先读取文件。" },
          {
            kind: "tool-call",
            tool: { id: "t", name: "read", args: '{"path":"server.go"}' },
          },
          { kind: "reasoning", text: "这是公开的思考摘要。" },
          {
            kind: "tool-call",
            tool: { id: "t2", name: "bash", args: '{"command":"test"}' },
          },
        ],
      },
    },
    {
      id: "t",
      seq: 3,
      message: {
        runID: "r",
        role: "tool",
        blocks: [
          {
            kind: "tool-result",
            result: { id: "t", name: "read", content: "长输出\n".repeat(150) },
          },
        ],
      },
    },
  ],
};

function Fixture() {
  const [snapshot, setSnapshot] = useState(initial);
  const [stopping, setStopping] = useState(false);
  return (
    <div style={{ height: "100dvh", display: "flex", flexDirection: "column" }}>
      <nav style={{ display: "flex", gap: 16, padding: 12 }}>
        <button
          onClick={() => {
            setSnapshot(structuredClone(initial));
            setStopping(false);
          }}
        >
          重置运行
        </button>
        <button
          onClick={() =>
            setSnapshot((previous) => ({
              ...previous,
              runs: [
                {
                  ...previous.runs[0],
                  drafts: [
                    {
                      entryID: "draft",
                      afterEntrySeq: 1,
                      blocks: [
                        { kind: "text", text: "正在生成\n".repeat(100) },
                      ],
                    },
                  ],
                },
              ],
            }))
          }
        >
          文字增量
        </button>
        <button
          onClick={() =>
            setSnapshot((previous) => ({
              ...previous,
              runs: [{ ...previous.runs[0], status: "success", drafts: [] }],
              entries: [
                ...initial.entries,
                {
                  id: "final",
                  seq: 4,
                  message: {
                    runID: "r",
                    role: "assistant",
                    afterSeq: 1,
                    blocks: [
                      {
                        kind: "text",
                        text: "## 已完成\n\n**修复成功**。\n\n|项目|结果|\n|---|---|\n|测试|通过|\n\n<script>alert(1)</script>",
                      },
                    ],
                  },
                },
              ],
            }))
          }
        >
          正常完成
        </button>
        <button onClick={() => setStopping(true)}>停止中</button>
        {(["failed", "cancelled", "interrupted"] as const).map((status) => (
          <button
            key={status}
            onClick={() =>
              setSnapshot((previous) => ({
                ...previous,
                runs: [
                  {
                    ...previous.runs[0],
                    status,
                    drafts: [],
                    error: status === "failed" ? "模拟失败" : undefined,
                  },
                ],
                entries: [
                  ...initial.entries,
                  {
                    id: "partial",
                    seq: 4,
                    message: {
                      runID: "r",
                      role: "assistant",
                      incomplete: true,
                      blocks: [{ kind: "text", text: "半截正文" }],
                    },
                  },
                ],
              }))
            }
          >
            {status}
          </button>
        ))}
        <button
          onClick={() => document.documentElement.classList.toggle("dark")}
        >
          亮暗
        </button>
      </nav>
      <section className="chat" style={{ flex: 1, minHeight: 0 }}>
        <ChatMessages
          snapshot={snapshot}
          sessionID="test"
          stoppingRunID={stopping ? "r" : undefined}
        />
      </section>
    </div>
  );
}
createRoot(document.getElementById("root")!).render(<Fixture />);
