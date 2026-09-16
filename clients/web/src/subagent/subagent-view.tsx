import { useEffect, useRef, useState } from "react";
import { ChatMessages } from "../chat-messages";
import { Composer, type Attachment } from "../composer";
import { compressImage } from "../image-compression";
import { activeRun, applyRunEvent, latestUsage } from "../state/chat";
import { formatRPCError, type RPCClient } from "../client/rpc";
import type { ModelSelection } from "../model-menu";
import type { AgentView, ModelChoice } from "../../../contracts/appserver";
import type { SubagentInfo } from "../../../contracts/harness";
import type { RunDiffSummary, Snapshot } from "../../../contracts/run";
import type { FileLocation } from "../editor/links";

export function SubagentView({
  parentSessionID,
  taskID,
  client,
  models,
  agents,
  active,
  onTask,
  onOpenSubagent,
  onOpenFile,
  onOpenDiff,
  onDiffUpdate,
}: {
  parentSessionID: string;
  taskID: string;
  client: RPCClient | null;
  models: ModelChoice[] | null;
  agents: AgentView[] | null;
  active: boolean;
  onTask: (task: SubagentInfo) => void;
  onOpenSubagent: (parentSessionID: string, taskID: string) => void;
  onOpenFile: (location: FileLocation) => void;
  onOpenDiff: (
    runID: string,
    summary: RunDiffSummary,
    runActive: boolean,
  ) => void;
  onDiffUpdate: (
    runID: string,
    summary: RunDiffSummary,
    runActive: boolean,
  ) => void;
}) {
  const [task, setTask] = useState<SubagentInfo | null>(null);
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [draft, setDraft] = useState("");
  const [images, setImages] = useState<Attachment[]>([]);
  const [notice, setNotice] = useState("");
  const [syncing, setSyncing] = useState(true);
  const [sending, setSending] = useState(false);
  const [compressingImages, setCompressingImages] = useState(false);
  const [stopping, setStopping] = useState(false);
  const [settingsSaving, setSettingsSaving] = useState(false);
  const [generation, setGeneration] = useState(0);
  const subscription = useRef("");
  const childSessionID = useRef("");
  const compressionPending = useRef(false);
  const snapshotRef = useRef<Snapshot | null>(null);
  const taskRef = useRef<SubagentInfo | null>(null);
  const urls = useRef<string[]>([]);
  const renderFrame = useRef<number | null>(null);
  const activeRef = useRef(active);
  const onTaskRef = useRef(onTask);
  const onDiffUpdateRef = useRef(onDiffUpdate);
  taskRef.current = task;
  activeRef.current = active;
  onTaskRef.current = onTask;
  onDiffUpdateRef.current = onDiffUpdate;

  function publishSnapshot() {
    if (!activeRef.current || renderFrame.current !== null) return;
    renderFrame.current = requestAnimationFrame(() => {
      renderFrame.current = null;
      if (activeRef.current) setSnapshot(snapshotRef.current);
    });
  }

  useEffect(() => {
    childSessionID.current = "";
    if (!client?.connected) {
      setSyncing(true);
      return;
    }
    let cancelled = false;
    let subscriptionID = "";
    setSyncing(true);
    setNotice("");
    const removeListener = client.onRunEvent(
      ({ subscriptionID: incoming, event }) => {
        if (cancelled || incoming !== subscription.current) return;
        const current = snapshotRef.current;
        if (!current) return;
        const next = applyRunEvent(current, event);
        if (!next) {
          setGeneration((value) => value + 1);
          return;
        }
        if (next !== current) {
          snapshotRef.current = next;
          publishSnapshot();
        }
        if (event.kind === "run-started")
          updateTaskStatus("running", event.runID);
        if (event.kind === "run-ended") setStopping(false);
        if (event.kind === "run-ended" && event.status) {
          updateTaskStatus(
            event.status === "success" ? "completed" : event.status,
            event.runID,
          );
        }
        if (event.kind === "run-diff-updated" && event.diff)
          onDiffUpdateRef.current(
            event.runID,
            event.diff,
            next.runs.some(
              (item) =>
                item.runID === event.runID && item.status === "running",
            ),
          );
        if (event.kind === "run-ended") {
          const summary = next.runs.find(
            (item) => item.runID === event.runID,
          )?.diff;
          if (summary)
            onDiffUpdateRef.current(event.runID, summary, false);
        }
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
        childSessionID.current = result.childSessionID;
        taskRef.current = result.task;
        snapshotRef.current = result.snapshot;
        setTask(result.task);
        onTaskRef.current(result.task);
        setSnapshot(result.snapshot);
        for (const item of result.snapshot.runs) {
          if (item.diff)
            onDiffUpdateRef.current(
              item.runID,
              item.diff,
              item.status === "running",
            );
        }
        setSyncing(false);
      })
      .catch((error) => {
        if (!cancelled) {
          setSyncing(false);
          setNotice(formatRPCError(error, "子任务同步失败"));
        }
      });
    return () => {
      cancelled = true;
      removeListener();
      if (renderFrame.current !== null)
        cancelAnimationFrame(renderFrame.current);
      renderFrame.current = null;
      if (subscription.current === subscriptionID) subscription.current = "";
      if (subscriptionID && client.connected)
        void client.unsubscribe(subscriptionID).catch(() => {});
    };
  }, [client, parentSessionID, taskID, generation]);

  useEffect(() => {
    if (active) setSnapshot(snapshotRef.current);
  }, [active]);

  function updateTaskStatus(status: SubagentInfo["status"], runID: string) {
    const current = taskRef.current;
    if (!current) return;
    const next = { ...current, status, currentRunID: runID };
    taskRef.current = next;
    setTask(next);
    onTaskRef.current(next);
  }

  useEffect(
    () => () => {
      for (const url of urls.current) URL.revokeObjectURL(url);
    },
    [],
  );

  const run = activeRun(snapshot);
  const selection: ModelSelection = {
    model: task?.model ?? "",
    reasoningEffort: task?.reasoningEffort ?? "",
  };
  const selectedModel = models?.find((model) => model.id === selection.model);
  const validModel = !!selectedModel?.reasoningEfforts.includes(
    selection.reasoningEffort,
  );
  const disabled =
    !client?.connected || syncing || sending || compressingImages;
  const canSend =
    !disabled && validModel && (draft.trim() !== "" || images.length > 0);

  async function send() {
    if (
      !client?.connected ||
      !canSend ||
      sending ||
      compressionPending.current
    )
      return;
    const text = draft;
    const submitted = images;
    setSending(true);
    setNotice("");
    try {
      await client.sendSubagent({
        parentSessionID,
        taskID,
        ...(text.trim() ? { text } : {}),
        ...(submitted.length
          ? { images: submitted.map(({ mime, data }) => ({ mime, data })) }
          : {}),
      });
      setDraft((current) => (current === text ? "" : current));
      setImages((current) => {
        if (current !== submitted) return current;
        for (const image of submitted) URL.revokeObjectURL(image.url);
        return [];
      });
    } catch (error) {
      setNotice(formatRPCError(error, "发送失败"));
    } finally {
      setSending(false);
    }
  }

  async function addImages(files: FileList | File[] | null) {
    if (!files || !selectedModel?.vision || compressionPending.current) return;
    compressionPending.current = true;
    setCompressingImages(true);
    try {
      const remaining = Math.max(0, 4 - images.length);
      const next = await Promise.all(
        Array.from(files).slice(0, remaining).map(compressImage),
      );
      for (const image of next) urls.current.push(image.url);
      setImages((current) => [...current, ...next].slice(0, 4));
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "图片处理失败");
    } finally {
      compressionPending.current = false;
      setCompressingImages(false);
    }
  }

  async function updateSettings(value: ModelSelection) {
    if (!client?.connected || !task || run || settingsSaving) return;
    setSettingsSaving(true);
    try {
      const result = await client.updateSubagentSettings({
        parentSessionID,
        taskID,
        model: value.model,
        reasoningEffort: value.reasoningEffort,
      });
      setTask(result.task);
      onTaskRef.current(result.task);
    } catch (error) {
      setNotice(formatRPCError(error, "设置保存失败"));
    } finally {
      setSettingsSaving(false);
    }
  }

  return (
    <section
      className="chat subagent-chat"
      aria-label={`子任务 ${task?.taskName ?? ""}`}
      hidden={!active}
    >
      <ChatMessages
        snapshot={snapshot}
        sessionID={`subagent:${taskID}`}
        stoppingRunID={stopping ? run?.runID : undefined}
        workspace={task?.workspace}
        onOpenFile={onOpenFile}
        onOpenDiff={(runID, summary) =>
          onOpenDiff(
            runID,
            summary,
            snapshot?.runs.some(
              (item) => item.runID === runID && item.status === "running",
            ) ?? false,
          )
        }
        onOpenSubagent={(nestedTaskID) => {
          if (childSessionID.current)
            onOpenSubagent(childSessionID.current, nestedTaskID);
        }}
      >
        <div className="empty-chat">
          <h1>{syncing ? "正在同步子任务…" : (task?.taskName ?? "子任务")}</h1>
          {task?.description && <p>{task.description}</p>}
        </div>
      </ChatMessages>
      <Composer
        draft={draft}
        images={images}
        notice={notice}
        agents={agents?.filter((agent) => agent.id === task?.agentID) ?? null}
        agentID={task?.agentID ?? ""}
        settingsDisabled
        usage={latestUsage(snapshot)}
        running={!!run}
        stopping={stopping}
        canSend={canSend}
        stopDisabled={!run || stopping || !client?.connected}
        modelDisabled={!!run || settingsSaving || syncing}
        validModel={validModel}
        models={models}
        modelSelection={selection}
        modelError=""
        imageDisabled={!selectedModel?.vision || sending || compressingImages}
        skills={[]}
        commands={[]}
        suggestionsDisabled
        commandBusy={false}
        onDraftChange={setDraft}
        onSend={() => void send()}
        onStop={() => {
          if (!client?.connected || !run || stopping) return;
          setStopping(true);
          void client.stopSubagent(parentSessionID, taskID).catch((error) => {
            setStopping(false);
            setNotice(formatRPCError(error, "停止失败"));
          });
        }}
        onAddImages={(files) => void addImages(files)}
        onRemoveImage={(id) =>
          setImages((current) => {
            const image = current.find((item) => item.id === id);
            if (image) URL.revokeObjectURL(image.url);
            return current.filter((item) => item.id !== id);
          })
        }
        onModelChange={(value) => void updateSettings(value)}
        onAgentChange={() => {}}
        onRetryModels={() => {}}
        onCommand={async () => {}}
        onDismissNotice={() => setNotice("")}
      />
    </section>
  );
}
