import { runSubscription } from "../client/run-subscription";
import { useCallback, useEffect, useState } from "react";
import type { RunDiffSummary } from "../../../contracts/run.ts";
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
  const runID = initialSummary.runID;

  useEffect(() => {
    if (!client?.connected) return;
    const subscription = runSubscription(
      client,
      (accept) => client.subscribeSubagent(parentSessionID, taskID, accept),
      {
        syncing: () => setSyncError(""),
        event: (event) => {
          if (event.runID !== runID) return;
          if (event.kind === "run-diff-updated" && event.diff)
            setSummary(event.diff);
          if (event.kind === "run-started") setRunActive(true);
          if (event.kind === "run-ended") setRunActive(false);
        },
        snapshot: (result) => {
          const run = result.snapshot.runs.find((item) => item.runID === runID);
          if (run?.diff) setSummary(run.diff);
          setRunActive(run?.status === "running");
        },
        error: (error) =>
          setSyncError(formatRPCError(error, "子任务 Diff 同步失败")),
      },
    );
    void subscription.synchronize();
    return subscription.close;
  }, [client, generation, parentSessionID, runID, taskID]);

  const readDiff = useCallback(
    (targetRunID: string, path: string) =>
      client!.readSubagentRunDiff(parentSessionID, taskID, targetRunID, path),
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
