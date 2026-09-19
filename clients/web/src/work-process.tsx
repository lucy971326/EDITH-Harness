import { memo, useEffect, useState } from "react";
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
  Bot,
  GitBranch,
  GitCompareArrows,
} from "./icons";
import { MessageMarkdown } from "./message-markdown";
import { UserMessage } from "./user-message";
import {
  processGroups,
  type ChatTurn,
  type ProcessItem,
} from "./state/chat-process";
import { runLabel } from "./state/chat";
import type { Block } from "../../contracts/run.ts";
import type { RunDiffSummary } from "../../contracts/run.ts";
import type { FileLocation } from "./editor/links";

function MessageImages({
  blocks,
  compact = false,
}: {
  blocks: Block[];
  compact?: boolean;
}) {
  const images = blocks.filter(
    (block) =>
      block.kind === "image" &&
      block.media &&
      ["image/png", "image/jpeg", "image/webp"].includes(block.media.mime),
  );
  if (images.length === 0) return null;
  return (
    <div className={`message-images${compact ? " user-message-images" : ""}`}>
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
  const collaboration = item.kind === "collaboration";
  const className = collaboration
    ? "process-collaboration"
    : item.kind === "reasoning"
      ? "process-reasoning"
      : undefined;
  const Icon = collaboration
    ? Bot
    : item.kind === "reasoning"
      ? Brain
      : FileText;
  return (
    <Collapsible
      className={className}
      onOpenChange={(open) => {
        if (open) onInspect();
      }}
    >
      <CollapsibleTrigger className="tool-summary process-summary">
        <Icon />
        <span className="process-detail-label">
          <span>
            {item.title}
            {item.status && ` · ${item.status}`}
          </span>
          {collaboration && (
            <span className="process-detail-preview">{item.text}</span>
          )}
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

function SubagentCard({
  item,
  onOpen,
}: {
  item: ProcessItem;
  onOpen?: (taskID: string) => void;
}) {
  if (!item.taskID) return <Detail item={item} onInspect={() => {}} />;
  return (
    <button
      className="subagent-card"
      onClick={() => onOpen?.(item.taskID!)}
      aria-label={`打开子任务 ${item.taskName ?? ""}`}
    >
      <Bot />
      <span>
        <strong>{item.taskName || "子任务"}</strong>
        <small>{item.kind === "collaboration" ? item.text : item.status}</small>
      </span>
      <ChevronRight />
    </button>
  );
}

const toolActions: Record<string, string> = {
  subagent_options: "查看子任务能力",
  subagent_spawn: "派出子任务",
  subagent_send: "追加子任务指令",
  subagent_list: "查看子任务",
  subagent_wait: "等待子任务",
  subagent_stop: "停止子任务",
  bash: "运行命令",
  read: "读取文件",
  write: "写入文件",
  edit: "编辑文件",
};

function WorkProcessComponent({
  turn,
  onInspect,
  onFork,
  forkDisabled = false,
  forking = false,
  stopping = false,
  workspace,
  onOpenFile,
  onOpenDiff,
  onOpenSubagent,
}: {
  turn: ChatTurn;
  onInspect: () => void;
  onFork?: (runID: string, boundaryEntryID: string) => void;
  forkDisabled?: boolean;
  forking?: boolean;
  stopping?: boolean;
  workspace?: string | null;
  onOpenFile?: (location: FileLocation) => void;
  onOpenDiff?: (runID: string, summary: RunDiffSummary) => void;
  onOpenSubagent?: (taskID: string) => void;
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
          <div className="user-prompt">
            <MessageImages blocks={turn.prompt.message.blocks} compact />
            {promptText.trim() && (
              <div className="user-message">
                <UserMessage
                  text={promptText}
                  workspace={workspace}
                  onOpenFile={onOpenFile}
                />
              </div>
            )}
          </div>
          <div className="user-actions">
            <CopyMessage text={promptText} label="复制用户消息" />
          </div>
        </div>
      )}
      {turn.standalone &&
        turn.items.map((item) =>
          item.kind === "subagent" ||
          (item.kind === "collaboration" && item.taskID) ? (
            <SubagentCard key={item.id} item={item} onOpen={onOpenSubagent} />
          ) : item.kind === "detail" ||
            item.kind === "tool" ||
            item.kind === "reasoning" ||
            item.kind === "collaboration" ? (
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
                  <Collapsible key={group[0].id} className="tool-group">
                    <CollapsibleTrigger className="tool-summary process-summary">
                      {group.every((item) =>
                        item.title.startsWith("subagent_"),
                      ) ? (
                        <Bot />
                      ) : (
                        <FileText />
                      )}
                      <span>
                        {[
                          ...new Set(
                            group.map(
                              (item) => toolActions[item.title] ?? item.title,
                            ),
                          ),
                        ].join("、")}{" "}
                        ·{" "}
                        {group.length === 1
                          ? group[0].status
                          : group.length + " 项"}
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
                ) : group.kind === "subagent" ||
                  (group.kind === "collaboration" && group.taskID) ? (
                  <SubagentCard
                    key={group.id}
                    item={group}
                    onOpen={onOpenSubagent}
                  />
                ) : group.kind === "detail" ||
                  group.kind === "reasoning" ||
                  group.kind === "collaboration" ? (
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
                      <UserMessage
                        text={group.text}
                        workspace={workspace}
                        onOpenFile={onOpenFile}
                      />
                    ) : group.kind === "text" ? (
                      <MessageMarkdown
                        text={group.text}
                        workspace={workspace}
                        onOpenFile={onOpenFile}
                      />
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
          <div className="answer" data-assistant-entry-id={turn.answer.id}>
            <MessageMarkdown
              text={turn.answer.text}
              workspace={workspace}
              onOpenFile={onOpenFile}
            />
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
      {turn.run?.diff && turn.run.diff.files.length > 0 && (
        <button
          className="run-diff-card"
          onClick={() => onOpenDiff?.(turn.run!.runID, turn.run!.diff!)}
        >
          <GitCompareArrows />
          <span>
            <strong>已修改 {turn.run.diff.files.length} 个文件</strong>
            <small>
              <i className="diff-additions">
                +
                {turn.run.diff.files.reduce(
                  (sum, file) => sum + file.additions,
                  0,
                )}
              </i>{" "}
              <i className="diff-deletions">
                -
                {turn.run.diff.files.reduce(
                  (sum, file) => sum + file.deletions,
                  0,
                )}
              </i>
            </small>
          </span>
          <span className="run-diff-action">审查</span>
        </button>
      )}
      {status === "success" && turn.items.length === 0 && turn.answer && (
        <span className="metadata">已完成</span>
      )}
    </article>
  );
}

function sameTurn(previous: ChatTurn, next: ChatTurn): boolean {
  if (previous === next) return true;
  if (
    previous.id !== next.id ||
    previous.standalone !== next.standalone ||
    previous.run?.status !== next.run?.status ||
    previous.run?.error !== next.run?.error ||
    previous.run?.diff?.revision !== next.run?.diff?.revision ||
    previous.prompt?.id !== next.prompt?.id ||
    previous.prompt?.message !== next.prompt?.message ||
    previous.answer?.id !== next.answer?.id ||
    previous.answer?.text !== next.answer?.text ||
    previous.items.length !== next.items.length
  )
    return false;
  return previous.items.every((item, index) => {
    const candidate = next.items[index];
    return (
      item.id === candidate.id &&
      item.kind === candidate.kind &&
      item.title === candidate.title &&
      item.text === candidate.text &&
      item.status === candidate.status &&
      item.media === candidate.media &&
      item.taskID === candidate.taskID &&
      item.taskName === candidate.taskName
    );
  });
}

// 流式输出只重绘正在变化的 Turn，旧消息中的 Markdown 和图片不重复解析。
export const WorkProcess = memo(
  WorkProcessComponent,
  (previous, next) =>
    sameTurn(previous.turn, next.turn) &&
    previous.stopping === next.stopping &&
    previous.forking === next.forking &&
    previous.forkDisabled === next.forkDisabled &&
    previous.workspace === next.workspace,
);
