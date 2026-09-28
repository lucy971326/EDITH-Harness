import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { ConnectionStatus } from "../client/rpc";

export function ConnectionIndicator({ status, detail, onReconnect }: {
  status: ConnectionStatus;
  detail?: string;
  onReconnect: () => void;
}) {
  const label = status === "connected" ? "在线" : status === "connecting" ? "连接中" : "离线";
  const description = status === "connected" ? "已连接后台"
    : status === "connecting" ? "正在连接后台…"
    : `${detail || "连接已断开，草稿仍保留"}；点击重新连接`;
  return <span className="connection-indicator" role="status" aria-live="polite">
    <Tooltip>
      <TooltipTrigger asChild>
        <Button variant="ghost" size="xs" className="connection-indicator-control"
          data-status={status} aria-label={description} aria-disabled={status !== "disconnected"}
          onClick={() => { if (status === "disconnected") onReconnect(); }}>
          <span className="connection-indicator-light" aria-hidden="true" />
          <span>{label}</span>
        </Button>
      </TooltipTrigger>
      <TooltipContent>{description}</TooltipContent>
    </Tooltip>
  </span>;
}
