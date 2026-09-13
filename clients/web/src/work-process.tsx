import { useEffect, useState } from "react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Button } from "@/components/ui/button";
import {
  ChevronRight,
  FileText,
  LoaderCircle,
  Copy,
  Brain,
  GitBranch,
} from "./icons";
import { MessageMarkdown } from "./message-markdown";
import {
  processGroups,
  type ChatTurn,
  type ProcessItem,
} from "./state/chat-process";
import { runLabel } from "./state/chat";
import type { Block } from "../../contracts/run.ts";

function MessageImages({ blocks }: { blocks: Block[] }) {
  const images = blocks.filter(
    (block) =>
      block.kind === "image" &&
      block.media &&
      ["image/png", "image/jpeg", "image/webp"].includes(block.media.mime),
  );
  if (images.length === 0) return null;
  return (
    <div className="message-images">
      {images.map((block, index) => (
        <img
          key={`${block.media!.mime}:${index}`}
          src={`data:${block.media!.mime};base64,${block.media!.data}`}
          alt={`发送的图片 ${index + 1}`}
        />
      ))}
    </div>
  );
}

function CopyMessage({ text, label }: { text: string; label: string }) {
  const [notice, setNotice] = useState("");
  return (
    <span className="answer-actions">
      <Button
        variant="ghost"
        size="icon-xs"
        aria-label={label}
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(text);
            setNotice("已复制");
          } catch {
            setNotice("复制失败，请手动选择文字复制");
          }
        }}
      >
        <Copy />
      </Button>
      <span className="metadata" role="status">
        {notice}
      </span>
    </span>
  );
}

function Detail({
  item,
  onInspect,
}: {
  item: ProcessItem;
  onInspect: () => void;
}) {
  return (
    <Collapsible
      onOpenChange={(open) => {
        if (open) onInspect();
      }}
    >
      <CollapsibleTrigger className="tool-summary tool-row">
        {item.kind === "reasoning" ? <Brain /> : <FileText />}
        <span>
          {item.title}
          {item.status && ` · ${item.status}`}
        </span>
        <ChevronRight />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <pre
          className="tool-output"
          tabIndex={0}
          aria-label={`${item.title}详情`}
        >
          {item.text}
        </pre>
      </CollapsibleContent>
    </Collapsible>
  );
}

export function WorkProcess({
  turn,
  onInspect,
  onFork,
  forkDisabled = false,
  forking = false,
  stopping = false,
}: {
  turn: ChatTurn;
  onInspect: () => void;
  onFork?: (runID: string, boundaryEntryID: string) => void;
  forkDisabled?: boolean;
  forking?: boolean;
  stopping?: boolean;
}) {
  const status = turn.run?.status;
  const [open, setOpen] = useState(status !== "success");
  // 只在运行状态变化时收展；普通增量不覆盖用户的手动选择。
  useEffect(() => {
    setOpen(status !== "success");
  }, [status]);
  const promptText =
    turn.prompt?.message.blocks
      .filter((b) => b.kind === "text")
      .map((b) => b.text ?? "")
      .join("\n\n") ?? "";
  const title =
    stopping && status === "running" ? "停止中，正在收尾" : runLabel(status);
  return (
    <article className="turn" data-run-id={turn.id}>
      {turn.prompt && (
        <div data-entry-id={turn.prompt.id}>
          <div className="user-message">
            <MessageImages blocks={turn.prompt.message.blocks} />
            <MessageMarkdown text={promptText} />
          </div>
          <div className="user-actions">
            <CopyMessage text={promptText} label="复制用户消息" />
          </div>
        </div>
      )}
      {turn.standalone &&
        turn.items.map((item) =>
          item.kind === "detail" ||
          item.kind === "tool" ||
          item.kind === "reasoning" ? (
            <Detail key={item.id} item={item} onInspect={onInspect} />
          ) : (
            <div key={item.id} className="progress-text">
              {item.text}
            </div>
          ),
        )}
      {!turn.standalone &&
        (turn.items.length > 0 ||
          (turn.run !== undefined && status !== "success") ||
          !turn.answer) && (
          <Collapsible open={open} onOpenChange={setOpen} className="process">
            <CollapsibleTrigger className="process-heading">
              {status === "running" && (
                <LoaderCircle className="animate-spin" />
              )}
              工作过程 · {title}
              <ChevronRight className={open ? "rotate-90" : ""} />
            </CollapsibleTrigger>
            <CollapsibleContent className="process-body">
              {processGroups(turn.items).map((group) =>
                Array.isArray(group) ? (
                  group.every((item) => item.kind === "reasoning") ? (
                    <div key={group[0].id}>
                      {group.map((item) => (
                        <Detail
                          key={item.id}
                          item={item}
                          onInspect={onInspect}
                        />
                      ))}
                    </div>
                  ) : (
                    <Collapsible key={group[0].id} className="tool-group">
                      <CollapsibleTrigger className="tool-summary">
                        <FileText />
                        <span>
                          工具：
                          {[
                            ...new Set(
                              group
                                .filter((item) => item.kind === "tool")
                                .map((item) => item.title),
                            ),
                          ].join("、")}{" "}
                          ·{" "}
                          {group.filter((item) => item.kind === "tool").length}{" "}
                          项
                        </span>
                        <ChevronRight />
                      </CollapsibleTrigger>
                      <CollapsibleContent
                        className="tool-list"
                        tabIndex={0}
                        aria-label="工具列表"
                      >
                        {group.map((item) => (
                          <Detail
                            key={item.id}
                            item={item}
                            onInspect={onInspect}
                          />
                        ))}
                      </CollapsibleContent>
                    </Collapsible>
                  )
                ) : group.kind === "detail" ? (
                  <Detail key={group.id} item={group} onInspect={onInspect} />
                ) : group.kind === "image" && group.media ? (
                  <div
                    key={group.id}
                    className={group.status ? "progress-text" : "steer-message"}
                  >
                    {group.status && (
                      <span className="metadata">{group.status}</span>
                    )}
                    <MessageImages
                      blocks={[{ kind: "image", media: group.media }]}
                    />
                  </div>
                ) : (
                  <div
                    key={group.id}
                    className={
                      group.kind === "steer" ? "steer-message" : "progress-text"
                    }
                  >
                    {(group.kind === "steer" || group.status) && (
                      <span className="metadata">
                        {group.kind === "steer" ? "已调整方向" : group.status}
                      </span>
                    )}
                    {group.kind === "steer" ? (
                      <MessageMarkdown text={group.text} />
                    ) : group.kind === "text" ? (
                      <MessageMarkdown text={group.text} />
                    ) : (
                      group.text
                    )}
                    {group.kind === "steer" && (
                      <CopyMessage text={group.text} label="复制插话" />
                    )}
                  </div>
                ),
              )}
              {turn.run?.error && (
                <p className="inline-error">{turn.run.error}</p>
              )}
              <p className="metadata" role="status">
                {title}
              </p>
            </CollapsibleContent>
          </Collapsible>
        )}
      {turn.answer && (
        <div data-entry-id={turn.answer.id}>
          <div className="answer">
            <MessageMarkdown text={turn.answer.text} />
          </div>
          <div className="answer-controls">
            <CopyMessage text={turn.answer.text} label="复制回答" />
            {!turn.standalone && turn.prompt && onFork && (
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label={forking ? "正在分叉" : "从此回答分叉"}
                disabled={forkDisabled || forking}
                onClick={() => onFork(turn.id, turn.prompt!.id)}
              >
                <GitBranch />
              </Button>
            )}
          </div>
        </div>
      )}
      {status === "success" && turn.items.length === 0 && turn.answer && (
        <span className="metadata">已完成</span>
      )}
    </article>
  );
}
