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
  Brain,
  LoaderCircle,
  CircleAlert,
  Copy,
  GitBranch,
} from "./icons";

export type RunStatus =
  | "running"
  | "stopping"
  | "completed"
  | "failed"
  | "stopped";
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
  activity: Activity[];
  answer: string;
};

// 按可见进展说明分组，不按模型调用次数切断工具链。
function groupActivity(activity: Activity[]) {
  const groups: (Activity | Activity[])[] = [];
  for (const item of activity) {
    if (item.kind === "progress" || item.kind === "steer") groups.push(item);
    else {
      const last = groups.at(-1);
      if (Array.isArray(last)) last.push(item);
      else groups.push([item]);
    }
  }
  return groups;
}

export function WorkProcess({
  turn,
  onInspect,
  onCopy,
  onFork,
}: {
  turn: Turn;
  onInspect: () => void;
  onCopy: (text: string) => void;
  onFork: () => void;
}) {
  const [open, setOpen] = useState(turn.status !== "completed");
  useEffect(() => {
    setOpen(turn.status !== "completed");
  }, [turn.status]);
  const running = turn.status === "running" || turn.status === "stopping";
  const title = running
    ? turn.status === "stopping"
      ? "停止中，正在收尾"
      : "正在工作"
    : turn.status === "completed"
      ? "工作过程"
      : turn.status === "failed"
        ? "运行失败"
        : "已停止";
  return (
    <article className="turn">
      <div className="user-message">
        {turn.images.length > 0 && (
          <div className="attachments">
            {turn.images.map((image) => (
              <img key={image.id} src={image.url} alt={image.name} />
            ))}
          </div>
        )}
        {turn.prompt}
      </div>
      <div className="user-actions">
        <Button
          variant="ghost"
          size="icon"
          aria-label="复制用户消息"
          onClick={() => onCopy(turn.prompt)}
        >
          <Copy />
        </Button>
      </div>
      <Collapsible open={open} onOpenChange={setOpen} className="process">
        <CollapsibleTrigger className="process-heading">
          {running ? (
            <LoaderCircle className="animate-spin" />
          ) : turn.status !== "completed" ? (
            <CircleAlert />
          ) : null}
          {title}
          <ChevronRight className={open ? "rotate-90" : ""} />
        </CollapsibleTrigger>
        <CollapsibleContent className="process-body">
          {groupActivity(turn.activity).map((group) =>
            Array.isArray(group) ? (
              <Collapsible key={group[0].id} className="tool-group">
                <CollapsibleTrigger className="tool-summary">
                  <FileText />
                  <span>
                    已读取文件、执行操作 ·{" "}
                    {group.filter((item) => item.kind === "tool").length} 项
                  </span>
                  <ChevronRight className="disclosure-chevron" />
                </CollapsibleTrigger>
                <CollapsibleContent
                  className="tool-list"
                  tabIndex={0}
                  aria-label="工具列表"
                >
                  {group.map((item) => (
                    <Collapsible
                      key={item.id}
                      onOpenChange={(value) => {
                        if (value) onInspect();
                      }}
                    >
                      <CollapsibleTrigger className="tool-summary tool-row">
                        {item.kind === "reasoning" ? <Brain /> : <FileText />}
                        <span>{item.text}</span>
                        <ChevronRight className="disclosure-chevron" />
                      </CollapsibleTrigger>
                      <CollapsibleContent>
                        <pre
                          className="tool-output"
                          tabIndex={0}
                          aria-label="工具详情"
                        >
                          {item.detail}
                        </pre>
                      </CollapsibleContent>
                    </Collapsible>
                  ))}
                </CollapsibleContent>
              </Collapsible>
            ) : (
              <p
                key={group.id}
                className={
                  group.kind === "steer" ? "steer-message" : "progress-text"
                }
              >
                {group.kind === "steer" && (
                  <span className="metadata">已调整方向</span>
                )}
                {group.text}
              </p>
            ),
          )}
          {turn.status === "failed" && (
            <p className="inline-error">
              模型请求失败。过程已保留，你可以重新发送；不会自动重试。
            </p>
          )}
          {turn.status === "stopped" && (
            <p className="muted">任务已停止，未开始的操作不会继续执行。</p>
          )}
        </CollapsibleContent>
      </Collapsible>
      {turn.status === "completed" && (
        <>
          <div className="answer">{turn.answer}</div>
          <div className="answer-actions">
            <Button
              variant="ghost"
              size="icon"
              aria-label="复制回答"
              onClick={() => onCopy(turn.answer)}
            >
              <Copy />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              aria-label="从此回答分叉"
              onClick={onFork}
            >
              <GitBranch />
            </Button>
          </div>
        </>
      )}
    </article>
  );
}
