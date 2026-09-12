import {
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Button } from "@/components/ui/button";
import { ArrowDown } from "./icons";
import { blockText, chatMessages, runLabel } from "./state/chat";
import type { Snapshot } from "../../contracts/run.ts";

export function ChatMessages({
  snapshot,
  sessionID,
  children,
}: {
  snapshot: Snapshot | null;
  sessionID: string | null;
  children?: ReactNode;
}) {
  const scroll = useRef<HTMLDivElement>(null);
  const follow = useRef(true);
  const [showLatest, setShowLatest] = useState(false);
  // 每份快照只整理一次；输入框编辑不重算历史，渲染时直接查运行和末条消息。
  const { messages, runsByID, lastMessageIDs } = useMemo(() => {
    const messages = snapshot ? chatMessages(snapshot) : [];
    const runsByID = new Map(snapshot?.runs.map((run) => [run.runID, run]));
    const lastMessageIDs = new Map<string, string>();
    for (const item of messages) {
      lastMessageIDs.set(item.message.runID ?? "", item.id);
    }
    return { messages, runsByID, lastMessageIDs };
  }, [snapshot]);
  useLayoutEffect(() => {
    follow.current = true;
    setShowLatest(false);
  }, [sessionID]);
  useLayoutEffect(() => {
    if (follow.current && scroll.current)
      scroll.current.scrollTop = scroll.current.scrollHeight;
  }, [snapshot, sessionID]);

  return (
    <div className="chat-history">
      <div
        className="messages"
        ref={scroll}
        onScroll={(event) => {
          const element = event.currentTarget;
          follow.current =
            element.scrollHeight - element.clientHeight - element.scrollTop <
            48;
          setShowLatest(!follow.current);
        }}
      >
        <div className="message-column">
          {messages.length === 0 && children}
          {messages.map((item) => {
            const runID = item.message.runID ?? "";
            const run = runsByID.get(runID);
            const lastInRun = lastMessageIDs.get(runID) === item.id;
            return (
              <article
                key={item.id}
                data-entry-id={item.id}
                className="chat-entry"
              >
                <div
                  className={
                    item.message.role === "user"
                      ? "user-message"
                      : "message-body"
                  }
                >
                  {item.message.role !== "user" && (
                    <span className="metadata">
                      {item.message.role === "assistant"
                        ? "助手"
                        : item.message.role === "collaboration"
                          ? "协作消息"
                          : "工具 / 系统"}
                      {item.draft
                        ? " · 生成中"
                        : item.message.incomplete
                          ? " · 未完成"
                          : ""}
                    </span>
                  )}
                  {item.message.blocks.map((block, position) => (
                    <div
                      key={position}
                      className={
                        block.kind === "reasoning"
                          ? "message-reasoning"
                          : "message-text"
                      }
                    >
                      {block.kind === "reasoning" && (
                        <span className="metadata">思考</span>
                      )}
                      {blockText(block)}
                    </div>
                  ))}
                </div>
                {lastInRun && item.message.runID && (
                  <div className="metadata run-status" role="status">
                    {runLabel(run?.status)}
                    {run?.status === "failed" && run.error && (
                      <p className="inline-error">{run.error}</p>
                    )}
                  </div>
                )}
              </article>
            );
          })}
          {snapshot?.runs
            .filter((run) => !lastMessageIDs.has(run.runID))
            .map((run) => (
              <p className="metadata run-status" key={run.runID}>
                {runLabel(run.status)} {run.status === "failed" && run.error}
              </p>
            ))}
        </div>
      </div>
      {showLatest && (
        <Button
          variant="outline"
          size="sm"
          className="back-latest"
          onClick={() => {
            follow.current = true;
            setShowLatest(false);
            if (scroll.current)
              scroll.current.scrollTop = scroll.current.scrollHeight;
          }}
        >
          <ArrowDown />
          回到最新
        </Button>
      )}
    </div>
  );
}
