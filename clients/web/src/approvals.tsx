import { useEffect, useState, type ReactNode } from "react";
import type { PendingApproval } from "../../contracts/approvals";
import { Button } from "@/components/ui/button";
import { RPCClient, RPCError, formatRPCError } from "./client/rpc";
import { FileText, Terminal } from "./icons";

// 本机用户的审批收件区；所有会话与子 Agent 的申请都标明来源。
export function Approvals({ client, children }: {
  client: RPCClient | null;
  children: ReactNode;
}) {
  const [pending, setPending] = useState<PendingApproval[]>([]);
  const [answering, setAnswering] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    setPending([]);
    setError("");
    setAnswering("");
    if (!client) return;
    let active = true;
    let subscriptionID = "";
    client.onApprovals = (notification) => {
      if (active && notification.subscriptionID === subscriptionID) {
        setPending(notification.event);
      }
    };
    void client.call("approval/subscribe", {}, {
      accept(result) {
        if (!active) {
          void client.unsubscribe(result.subscriptionID).catch(() => {});
          return;
        }
        subscriptionID = result.subscriptionID;
        setPending(result.pending);
      },
    }).catch((cause: unknown) => {
      if (!active) return;
      setError(formatRPCError(cause, "审批同步失败"));
      if (!(cause instanceof RPCError)) client.close();
    });
    return () => {
      active = false;
      client.onApprovals = null;
      if (subscriptionID && client.connected) {
        void client.unsubscribe(subscriptionID).catch(() => {});
      }
    };
  }, [client]);

  async function respond(id: string, approved: boolean) {
    if (!client?.connected || answering) return;
    setAnswering(id);
    setError("");
    try {
      await client.call("approval/respond", {
        requestID: id,
        decision: { approved, reason: approved ? "用户批准本次操作" : "用户拒绝本次操作" },
      });
    } catch (cause) {
      if (!client.connected) return;
      setError(formatRPCError(cause, "回答失败，请检查待审批状态"));
      // 回答可能已到达；断线重建投影，不自动重发有副作用的操作。
      if (!(cause instanceof RPCError)) client.close();
    } finally {
      setAnswering("");
    }
  }

  const current = pending[0];
  const mcp = current?.mcp;
  const title = mcp?.kind === "config" ? "启用项目 MCP"
    : mcp?.kind === "call" ? `MCP · ${mcp.server}/${mcp.tool}`
    : current?.request.toolName === "exec_command" ? "终端" : "文件修改";
  return (
    <>
      <div hidden={!!current}>{children}</div>
      {(current || error) && <section className="composer-area" aria-label="待审批操作">
        <div className="composer-column">
          {error && <p className="inline-notice" role="alert">{error}</p>}
          {current && <article className="composer approval-card" key={current.id}>
          <header className="approval-heading">
            {mcp?.kind === "call" || current.request.toolName === "exec_command" ? <Terminal /> : <FileText />}
            <span>{title}</span>
            <span>
              {!mcp && current.request.requested.network && " · 联网"}
              {!mcp && !!current.request.requested.writeRoots?.length &&
                ` · 写入 ${current.request.requested.writeRoots.length} 个目录`}
            </span>
            {pending.length > 1 && <span className="approval-count">还有 {pending.length - 1} 项待审批</span>}
          </header>
          <div className="approval-body">
          {current.reviewReason && <p className="metadata">{current.reviewReason}</p>}
          {mcp?.kind === "config" ? <>
            <p>这些 MCP Server 将在宿主环境运行。信任后，同一配置版本以后会自动启用。</p>
            <pre>{mcp.servers?.map((server) => `${server.name} → ${server.target}`).join("\n")}</pre>
          </> : mcp?.kind === "call" ? <>
            <p>允许 Agent 调用此 MCP 工具一次吗？Server 在宿主环境执行。</p>
            <pre>{JSON.stringify(mcp.arguments ?? {}, null, 2)}</pre>
          </> : <>
          <p>{current.request.reason || "本次操作需要额外权限，是否允许？"}</p>
          <pre>{String(
            current.request.arguments.cmd ?? current.request.arguments.patch ??
            JSON.stringify(current.request.arguments, null, 2)
          )}</pre>
          </>}
          </div>
          <footer className="approval-footer">
          <details className="approval-details">
            <summary>{mcp ? "配置与来源详情" : "权限与来源详情"}</summary>
            <div className="approval-detail-content">
          {mcp ? <>
            {mcp.source && <p>配置文件：{mcp.source}</p>}
            {mcp.digest && <p>配置摘要：{mcp.digest.slice(0, 12)}</p>}
            <p>工作目录：{mcp.workspace}</p>
          </> : <>
          <div>
            本次额外开放：
            {current.request.requested.network && <span>联网；</span>}
            {(current.request.requested.writeRoots ?? []).map((root) => (
              <code key={root}>{root}（目录可写） </code>
            ))}
          </div>
            <p>工作目录：{current.request.workdir}</p>
          </>}
            <p>来源会话：{current.sessionID}</p>
            </div>
          </details>
          <div className="approval-actions">
            <Button
              variant="outline"
              size="sm"
              disabled={!client?.connected || !!answering}
              onClick={() => void respond(current.id, false)}
            >拒绝</Button>
            <Button
              size="sm"
              disabled={!client?.connected || !!answering}
              onClick={() => void respond(current.id, true)}
            >{answering === current.id ? "正在提交…" : mcp?.kind === "config" ? "信任此版本" : "允许一次"}</Button>
          </div>
          </footer>
          </article>}
        </div>
      </section>}
    </>
  );
}
