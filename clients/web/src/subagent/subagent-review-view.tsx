import { useCallback, useEffect, useRef, useState } from "react";
import type { RunDiffSummary, Snapshot } from "../../../contracts/run.ts";
import type { RPCClient } from "../client/rpc";
import { formatRPCError } from "../client/rpc";
import { ReviewView } from "../review/review-view";

export function SubagentReviewView({
  parentSessionID,
  taskID,
  workspace,
  initialSummary,
  initialRunActive,
  client,
}: {
  parentSessionID: string;
  taskID: string;
  workspace: string;
  initialSummary: RunDiffSummary;
  initialRunActive: boolean;
  client: RPCClient | null;
}) {
  const [summary, setSummary] = useState(initialSummary);
  const [runActive, setRunActive] = useState(initialRunActive);
  const [syncError, setSyncError] = useState("");
  const [generation, setGeneration] = useState(0);
  const subscription = useRef("");
  const boundary = useRef<Pick<Snapshot, "seqEpoch" | "updateSeq"> | null>(
    null,
  );
  const resyncing = useRef(false);
  const runID = initialSummary.runID;

  useEffect(() => {
    if (!client?.connected) return;
    let cancelled = false;
    let subscriptionID = "";
    setSyncError("");
    const removeListener = client.onRunEvent(
      ({ subscriptionID: incoming, event }) => {
        if (cancelled || incoming !== subscription.current) return;
        const current = boundary.current;
        if (!current) return;
        if (event.seqEpoch !== current.seqEpoch) {
          if (resyncing.current) return;
          resyncing.current = true;
          setGeneration((value) => value + 1);
          return;
        }
        if (event.updateSeq <= current.updateSeq) return;
        if (event.updateSeq !== current.updateSeq + 1) {
          if (resyncing.current) return;
          resyncing.current = true;
          setGeneration((value) => value + 1);
          return;
        }
        boundary.current = {
          seqEpoch: event.seqEpoch,
          updateSeq: event.updateSeq,
        };
        if (event.runID !== runID) return;
        if (event.kind === "run-diff-updated" && event.diff)
          setSummary(event.diff);
        if (event.kind === "run-started") setRunActive(true);
        if (event.kind === "run-ended") setRunActive(false);
      },
    );
    void client
      .subscribeSubagent(parentSessionID, taskID, (result) => {
        if (cancelled) {
          void client.unsubscribe(result.subscriptionID).catch(() => {});
          return;
        }
        subscriptionID = result.subscriptionID;
        subscription.current = subscriptionID;
        resyncing.current = false;
        boundary.current = {
          seqEpoch: result.snapshot.seqEpoch,
          updateSeq: result.snapshot.updateSeq,
        };
        const run = result.snapshot.runs.find((item) => item.runID === runID);
        if (run?.diff) setSummary(run.diff);
        setRunActive(run?.status === "running");
      })
      .catch((error) => {
        if (!cancelled)
          setSyncError(formatRPCError(error, "子任务 Diff 同步失败"));
      });
    return () => {
      cancelled = true;
      removeListener();
      if (subscription.current === subscriptionID) subscription.current = "";
      if (subscriptionID && client.connected)
        void client.unsubscribe(subscriptionID).catch(() => {});
    };
  }, [client, generation, parentSessionID, runID, taskID]);

  const readDiff = useCallback(
    (targetRunID: string, path: string) =>
      client!.readSubagentRunDiff(
        parentSessionID,
        taskID,
        targetRunID,
        path,
      ),
    [client, parentSessionID, taskID],
  );
  const revertDiff = useCallback(
    (targetRunID: string, path: string, revision: number) =>
      client!.revertSubagentRunDiff(
        parentSessionID,
        taskID,
        targetRunID,
        path,
        revision,
      ),
    [client, parentSessionID, taskID],
  );

  if (syncError) {
    return (
      <div className="workspace-chooser" role="alert">
        <strong>无法同步子任务更改</strong>
        <span>{syncError}</span>
      </div>
    );
  }
  return (
    <ReviewView
      sessionID={parentSessionID}
      workspace={workspace}
      initialSummary={summary}
      client={client}
      runActive={runActive}
      readDiff={readDiff}
      revertDiff={revertDiff}
    />
  );
}
