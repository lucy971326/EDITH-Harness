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
  Copy,
  Brain,
  Bot,
  Terminal,
  GitBranch,
  GitCompareArrows,
} from "../icons";
import { MessageMarkdown } from "./message-markdown";
import { UserMessage } from "./user-message";
import {
  processGroups,
  type ChatTurn,
  type ProcessItem,
} from "../state/chat-process";
import { runLabel } from "../state/chat";
import type { Block } from "../../../contracts/run.ts";
import type { RunDiffSummary } from "../../../contracts/run.ts";
import type { FileLocation } from "../editor/links";

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
  const className = collaboration ? "process-collaboration" : undefined;
  const Icon = collaboration
    ? Bot
    : item.title === "exec_command" || item.title === "bash" || item.title === "write_stdin"
      ? Terminal
      : FileText;
  let label = item.title;
  if (item.kind === "tool") {
    if (item.title === "exec_command" || item.title === "bash") {
      label = item.status === "等待结果" ? "正在运行" : "已运行";
    } else {
      label = toolActions[item.title] ?? item.title;
    }
  }
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
          <span className="process-line">
            <span className="process-line-name">{label}</span>
            {item.preview && (
              <span className="process-line-preview">· {item.preview}</span>
            )}
            {item.status && item.status !== "已完成" && (
              <span className="process-line-status">· {item.status === "等待结果" ? "运行中" : item.status}</span>
            )}
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
  write_stdin: "终端",
  apply_patch: "修改",
  read: "读取文件",
  write: "写入文件",
  edit: "编辑文件",
};

function TerminalGroup({ items, onInspect }: { items: ProcessItem[]; onInspect: () => void }) {
  const failures = items.filter((item) => item.status === "异常").length;
  return (
    <Collapsible className="tool-group" onOpenChange={(open) => {
      if (open) onInspect();
    }}>
      <CollapsibleTrigger className="tool-summary process-summary">
        <Terminal />
        <span className="process-line">
          <span className="process-line-name">运行了命令</span>
          {failures > 0 && <span className="process-line-status">· {failures} 项异常</span>}
        </span>
        <ChevronRight />
      </CollapsibleTrigger>
      <CollapsibleContent className="tool-list" tabIndex={0} aria-label="终端命令">
        {items.map((item) => (
          <Detail key={item.id} item={item} onInspect={onInspect} />
        ))}
      </CollapsibleContent>
    </Collapsible>
  );
}

function ProcessOutput({
  item,
  workspace,
  onOpenFile,
  skillNames,
}: {
  item: ProcessItem;
  workspace?: string | null;
  onOpenFile?: (location: FileLocation) => void;
  skillNames?: string[];
}) {
  if (item.kind === "reasoning") {
    return (
      <div className="process-reasoning" role="status">
        <Brain aria-hidden="true" />
        <span className="process-line-status">{item.title}</span>
        <span className="process-line-preview">{item.preview}</span>
      </div>
    );
  }
  if (item.kind === "image" && item.media) {
    return (
      <div className={item.status ? "progress-text" : "steer-message"}>
        {item.status && <span className="metadata">{item.status}</span>}
        <MessageImages blocks={[{ kind: "image", media: item.media }]} />
      </div>
    );
  }
  return (
    <div className={item.kind === "steer" ? "steer-message" : "progress-text"}>
      {(item.kind === "steer" || item.status) && (
        <span className="metadata">{item.kind === "steer" ? "已调整方向" : item.status}</span>
      )}
      {item.kind === "steer" ? (
        <UserMessage text={item.text} workspace={workspace} onOpenFile={onOpenFile} skillNames={skillNames} />
      ) : item.kind === "text" ? (
        <MessageMarkdown text={item.text} workspace={workspace} onOpenFile={onOpenFile} />
      ) : item.text}
      {item.kind === "steer" && <CopyMessage text={item.text} label="复制插话" />}
    </div>
  );
}

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
  skillNames,
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
  skillNames?: string[];
}) {
  const status = turn.run?.status;
  const [open, setOpen] = useState(status !== "success");
  // 只随运行状态切换默认收展；普通增量保留用户的选择。
  useEffect(() => setOpen(status !== "success"), [status]);
  const promptText =
    turn.prompt?.message.blocks
      .filter((b) => b.kind === "text")
      .map((b) => b.text ?? "")
      .join("\n\n") ?? "";
  const title =
    stopping && status === "running" ? "停止中，正在收尾" : runLabel(status);
  const diff = turn.run?.diff;
  const additions = diff?.files.reduce((sum, file) => sum + file.additions, 0) ?? 0;
  const deletions = diff?.files.reduce((sum, file) => sum + file.deletions, 0) ?? 0;
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
                  skillNames={skillNames}
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
          <Collapsible open={open} onOpenChange={(next) => {
            setOpen(next);
            if (next) onInspect();
          }} className="process">
            <CollapsibleTrigger className="process-heading">
              <span className="process-heading-title">
                工作过程{status === "success" ? "" : ` · ${title}`}
              </span>
              <ChevronRight className={open ? "rotate-90" : ""} />
            </CollapsibleTrigger>
            <CollapsibleContent className="process-body">
              {processGroups(turn.items).map((group) =>
                Array.isArray(group) ? (
                  <TerminalGroup key={group[0].id} items={group} onInspect={onInspect} />
                ) : group.kind === "subagent" ||
                  (group.kind === "collaboration" && group.taskID) ? (
                  <SubagentCard key={group.id} item={group} onOpen={onOpenSubagent} />
                ) : group.kind === "detail" ||
                  group.kind === "tool" ||
                  group.kind === "collaboration" ? (
                  <Detail key={group.id} item={group} onInspect={onInspect} />
                ) : (
                  <ProcessOutput
                    key={group.id}
                    item={group}
                    workspace={workspace}
                    onOpenFile={onOpenFile}
                    skillNames={skillNames}
                  />
                ),
              )}
              {turn.run?.error && <p className="inline-error">{turn.run.error}</p>}
              {(status !== "success" || !turn.answer) && (
                <p className="metadata" role="status">{title}</p>
              )}
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
      {diff && diff.files.length > 0 && (
        <button
          className="run-diff-card ui-key ui-focus"
          aria-label={`审查本轮改动：${diff.files.length} 个文件，新增 ${additions} 行，删除 ${deletions} 行`}
          onClick={() => onOpenDiff?.(turn.run!.runID, diff)}
        >
          <span className="run-diff-mark" aria-hidden="true"><GitCompareArrows /></span>
          <span className="run-diff-copy">
            <span className="run-diff-caption">本轮文件改动</span>
            <span className="run-diff-readout">
              <strong>已修改 {diff.files.length} 个文件</strong>
              <span className="run-diff-counts">
                <span className="diff-additions">+{additions}</span>
                <span className="diff-deletions">−{deletions}</span>
              </span>
            </span>
          </span>
          <span className="run-diff-action">审查改动 <ChevronRight aria-hidden="true" /></span>
        </button>
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
      item.preview === candidate.preview &&
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
