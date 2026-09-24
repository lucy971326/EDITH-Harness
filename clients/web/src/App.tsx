import { Approvals } from "./chat/approvals";
import { ApprovalSettingsPanel } from "./settings/approval-settings";
import { HookSettingsPanel } from "./settings/hook-settings";
import type { PermissionModeChoice } from "../../contracts/approvals.ts";
import type { PermissionMode } from "../../contracts/harness.ts";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type CSSProperties,
} from "react";
import {
  addReference,
  clearSubmittedReferences,
  encodeReferences,
  referencePath,
  type ContextReference,
  type ReferenceAttachment,
} from "./chat/context-references";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  PanelLeft,
  PanelRight,
  Folder,
  Command,
  WifiOff,
  RefreshCw,
} from "./icons";
import { SettingsPage } from "./settings/settings-page";
import { WorkspaceTabs } from "./workspace/workspace-tabs";
import { Sidebar } from "./workspace/sidebar";
import { ResizeHandle } from "./components/resize-handle";
import { AppContextMenu } from "./components/app-context-menu";
import {
  Composer,
  type Attachment,
  type CommandSelection,
  type ComposerHandle,
} from "./chat/composer";
import {
  formatRPCError,
  isWorkspaceUnavailable,
  RPCClient,
  RPCError,
  rpcURL,
  shouldClearSessionOnGetError,
} from "./client/rpc";
import { ChatConnection, initialChatState } from "./client/chat";
import { activeRun, latestUsage } from "./state/chat";
import { ChatMessages } from "./chat/chat-messages";
import { type ModelSelection } from "./components/model-menu";
import { compressImage } from "./chat/image-compression";
import { workspaceName } from "./state/projects";
import type { FileLocation } from "./editor/links";
import type { ReviewOpenRequest, SubagentOpenRequest } from "./workspace/workspace-tabs";
import type { SendParams, SessionView } from "../../contracts/harness.ts";
import type {
  AgentListResult,
  AgentSaveParams,
  AgentView,
  CommandView,
  ModelChoice,
  SkillView,
} from "../../contracts/appserver.ts";

type Draft = {
  version: number;
  text: string;
  images: Attachment[];
  references: ReferenceAttachment[];
};

function emptyDraft(): Draft {
  return { text: "", images: [], references: [], version: 0 };
}

function preference(key: string, fallback: string) {
  try {
    return localStorage.getItem(`harness-web:${key}`) ?? fallback;
  } catch {
    return fallback;
  }
}
function savePreference(key: string, value: string) {
  try {
    localStorage.setItem(`harness-web:${key}`, value);
  } catch {
    /* 私密模式不可写时，仍可在本次页面使用。 */
  }
}

function draftKey(sessionID: string | null): string {
  return sessionID ?? "";
}

function restoredSession(): string | null {
  try {
    return sessionStorage.getItem("harness-web:session");
  } catch {
    return null;
  }
}

export function chatSendParams(
  sessionID: string,
  text: string,
  images: Attachment[],
  expectedRunID?: string,
): SendParams {
  return {
    sessionID,
    ...(text ? { text } : {}),
    ...(images.length
      ? { images: images.map(({ mime, data }) => ({ mime, data })) }
      : {}),
    ...(expectedRunID ? { expectedRunID } : {}),
  };
}

export function shouldClearSubmittedDraft(
  drafts: Map<string, { version: number }>,
  sessionID: string,
  submittedVersion: number,
): boolean {
  return (drafts.get(sessionID)?.version ?? 0) === submittedVersion;
}

export default function App() {
  const appElement = useRef<HTMLDivElement>(null);
  const [settings, setSettings] = useState(false);
  const [sidebar, setSidebar] = useState(true);
  const [sidebarWidth, setSidebarWidth] = useState(() =>
    Math.max(
      200,
      Math.min(420, Number(preference("sidebar-width", "210")) || 210),
    ),
  );
  const [panel, setPanel] = useState(
    () => preference("panel", "false") === "true",
  );
  const [panelWidth, setPanelWidth] = useState(() =>
    Math.max(
      320,
      // 新阅读布局不继承旧版宽面板；后续拖动仍保存在本机。
      Math.min(
        1200,
        Number(preference("reading-panel-width", "0")) ||
          Math.min(
            480,
            (typeof window === "undefined" ? 1280 : window.innerWidth) * 0.25,
          ),
      ),
    ),
  );
  const [viewportWidth, setViewportWidth] = useState(() =>
    typeof window === "undefined" ? 1280 : window.innerWidth,
  );
  const [theme, setTheme] = useState(() => preference("theme", "system"));
  const [composerDraft, setComposerDraft] = useState<Draft>(emptyDraft);
  const { text: draft, images, references } = composerDraft;
  const [notice, setNotice] = useState("");
  const [approvalClient, setApprovalClient] = useState<RPCClient | null>(null);
  const [permissionModes, setPermissionModes] = useState<
    PermissionModeChoice[]
  >([]);
  const [chatState, setChatState] = useState(initialChatState);
  const { connection, detail: connectionDetail } = chatState;
  const [models, setModels] = useState<ModelChoice[] | null>(null);
  const [modelError, setModelError] = useState("");
  const [agentCatalog, setAgentCatalog] = useState<AgentListResult | null>(
    null,
  );
  const [agentError, setAgentError] = useState("");
  const [agentLoading, setAgentLoading] = useState(false);
  const [agentSaving, setAgentSaving] = useState(false);
  const [skills, setSkills] = useState<SkillView[]>([]);
  const [commands, setCommands] = useState<CommandView[]>([]);
  const [settingsSaving, setSettingsSaving] = useState(false);
  const [compressingImages, setCompressingImages] = useState(false);
  const [sending, setSending] = useState<string[]>([]);
  const [commandBusy, setCommandBusy] = useState(false);
  const [forkingEntryID, setForkingEntryID] = useState("");
  const [stopping, setStopping] = useState<{
    sessionID: string;
    runID: string;
  } | null>(null);
  const [sessions, setSessions] = useState<SessionView[] | null>(null);
  const [listError, setListError] = useState("");
  const [selectedID, setSelectedID] = useState<string | null>(restoredSession);
  const [selected, setSelected] = useState<SessionView | null>(null);
  const [opening, setOpening] = useState(false);
  const [creatingWorkspace, setCreatingWorkspace] = useState<string | null>(
    null,
  );
  const [fileOpenRequest, setFileOpenRequest] = useState<
    (FileLocation & { requestID: number; workspace: string }) | undefined
  >();
  const fileOpenRequestID = useRef(0);
  const [reviewOpenRequest, setReviewOpenRequest] =
    useState<ReviewOpenRequest>();
  const reviewOpenRequestID = useRef(0);
  const [subagentOpenRequest, setSubagentOpenRequest] =
    useState<SubagentOpenRequest>();
  const subagentOpenRequestID = useRef(0);
  const composer = useRef<ComposerHandle>(null);
  const objectUrls = useRef<string[]>([]);
  const drafts = useRef(new Map<string, Draft>());
  const clientRef = useRef<RPCClient | null>(null);
  const chatRef = useRef<ChatConnection | null>(null);
  const selectedIDRef = useRef<string | null>(selectedID);
  const sendingRef = useRef(new Set<string>());
  const stopPending = useRef(false);
  const settingsPending = useRef(false);
  const compressionPending = useRef(false);
  const commandPending = useRef(false);
  const forkPending = useRef(false);
  const draftRef = useRef(composerDraft);
  const selectGeneration = useRef(0);
  const listGeneration = useRef(0);

  const connected = connection === "connected";
  const sidebarSpace = sidebar ? sidebarWidth : 0;
  const panelOverlay =
    viewportWidth <= 950 || viewportWidth - sidebarSpace < 620;
  const panelMaxWidth = panelOverlay
    ? 1200
    : Math.min(1200, viewportWidth - sidebarSpace - 300);
  const visiblePanelWidth = Math.min(panelWidth, panelMaxWidth);
  const backendBusy = opening || creatingWorkspace !== null;
  const snapshot =
    chatState.sessionID === selectedID ? chatState.snapshot : null;
  const currentRun = activeRun(snapshot);
  const synchronized =
    connected &&
    chatState.sessionID === selectedID &&
    !chatState.syncing &&
    !chatState.error &&
    snapshot !== null;
  const busySending = selectedID !== null && sending.includes(selectedID);
  const stoppingCurrent =
    stopping?.sessionID === selectedID && stopping.runID === currentRun?.runID;
  const modelSelection: ModelSelection = {
    model: selected?.settings.model ?? "",
    reasoningEffort: selected?.settings.reasoningEffort ?? "",
  };
  const validModel = models?.some(
    (item) =>
      item.id === modelSelection.model &&
      item.reasoningEfforts.includes(modelSelection.reasoningEffort),
  );
  const selectedModel = models?.find(
    (item) => item.id === modelSelection.model,
  );
  const validAgent = agentCatalog?.agents.some(
    (agent) => agent.id === selected?.settings.agentID,
  );
  const sessionUsage = latestUsage(snapshot);
  const settingsDisabled =
    !connected ||
    !selected ||
    !synchronized ||
    !!currentRun ||
    busySending ||
    settingsSaving;
  const canSend =
    synchronized &&
    !!selected &&
    !busySending &&
    !stoppingCurrent &&
    !commandBusy &&
    !compressingImages &&
    (!!draft.trim() || images.length > 0 || references.length > 0) &&
    (!!currentRun || (!!validAgent && !!validModel && !modelError)) &&
    (!images.length || !!selectedModel?.vision);

  // 同一份草稿同时服务异步回调与当前画面；每次编辑立即保存到对应会话。
  function updateDraft(patch: Partial<Draft>) {
    const next = { ...draftRef.current, ...patch };
    drafts.current.set(draftKey(selectedIDRef.current), next);
    draftRef.current = next;
    setComposerDraft(next);
  }

  function editDraft(text: string) {
    updateDraft({ text, version: draftRef.current.version + 1 });
  }

  const referenceWorkspace = selected?.settings.workspace;
  const searchContextPaths = useCallback(
    async (workspace: string, query: string) => {
      const client = clientRef.current;
      if (!client?.connected) throw new Error("后台未连接");
      return client.searchPaths(workspace, query);
    },
    [],
  );
  const addContextReference = useCallback(
    (reference: ContextReference) => {
      if (!selectedID || selectedIDRef.current !== selectedID) return;
      if (reference.kind !== "assistant-selection" && !referenceWorkspace)
        return;
      const normalized =
        reference.kind === "assistant-selection"
          ? reference
          : {
              ...reference,
              path: referencePath(referenceWorkspace!, reference.path),
            };
      const next = addReference(draftRef.current.references, normalized);
      updateDraft({ references: next });
      setSettings(false);
      requestAnimationFrame(() => composer.current?.focus());
    },
    [selectedID, referenceWorkspace],
  );

  function removeReference(id: string) {
    const next = draftRef.current.references.filter((item) => item.id !== id);
    updateDraft({ references: next });
  }

  function applyDraft(sessionID: string | null) {
    const stored = drafts.current.get(draftKey(sessionID)) ?? emptyDraft();
    draftRef.current = stored;
    setComposerDraft(stored);
  }
  function setCurrentSession(id: string | null, session: SessionView | null) {
    if (id !== selectedIDRef.current) setSkills([]);
    selectedIDRef.current = id;
    setSelectedID(id);
    setSelected(session);
    chatRef.current?.select(id);
    try {
      if (id) sessionStorage.setItem("harness-web:session", id);
      else sessionStorage.removeItem("harness-web:session");
    } catch {
      /* 存储被禁用时只保留本页选择。 */
    }
  }

  function openSettings() {
    setSettings(true);
    if (window.innerWidth < 760) setSidebar(false);
  }
  async function addImages(files: FileList | File[] | null) {
    if (!files) return;
    if (!selectedModel?.vision) {
      setNotice("当前模型不能识别图片，请先选择视觉模型。");
      return;
    }
    const targetID = selectedIDRef.current;
    const initialImages = [...draftRef.current.images];
    const available = 4 - initialImages.length;
    if (available <= 0) {
      setNotice("每次最多发送 4 张图片。");
      return;
    }
    const chosen = Array.from(files).slice(0, available);
    if (chosen.length < Array.from(files).length)
      setNotice("每次最多发送 4 张图片。");
    compressionPending.current = true;
    setCompressingImages(true);
    try {
      const attachments: Attachment[] = [];
      for (const file of chosen) attachments.push(await compressImage(file));
      for (const attachment of attachments)
        objectUrls.current.push(attachment.url);
      const next = [...initialImages, ...attachments].slice(0, 4);
      const key = draftKey(targetID);
      if (selectedIDRef.current === targetID) {
        updateDraft({ images: next });
      } else {
        const stored = drafts.current.get(key) ?? {
          ...emptyDraft(),
          images: initialImages,
        };
        drafts.current.set(key, { ...stored, images: next });
      }
      setNotice("");
    } catch (error) {
      setNotice(
        error instanceof Error ? error.message : "图片压缩失败，原草稿已保留。",
      );
    } finally {
      compressionPending.current = false;
      setCompressingImages(false);
    }
  }
  function removeImage(id: string) {
    const removed = draftRef.current.images.find((item) => item.id === id);
    if (removed) URL.revokeObjectURL(removed.url);
    const next = draftRef.current.images.filter((item) => item.id !== id);
    updateDraft({ images: next });
  }

  async function loadSessions(client: RPCClient) {
    const generation = ++listGeneration.current;
    setListError("");
    try {
      const result = await client.list();
      if (
        generation !== listGeneration.current ||
        client !== clientRef.current ||
        !client.connected
      )
        return;
      setSessions(result.sessions);
      const currentID = selectedIDRef.current;
      if (!currentID) {
        setSelected(null);
        return;
      }
      await loadSelected(client, currentID);
    } catch (error) {
      if (generation !== listGeneration.current || client !== clientRef.current)
        return;
      setListError(formatRPCError(error, "无法加载项目列表"));
    }
  }

  async function loadSelected(client: RPCClient, sessionID: string) {
    const generation = ++selectGeneration.current;
    try {
      const result = await client.get(sessionID);
      if (
        generation !== selectGeneration.current ||
        client !== clientRef.current ||
        !client.connected
      )
        return;
      if (selectedIDRef.current !== sessionID) return;
      setSelected(result.session);
      void loadSkills(client, sessionID);
    } catch (error) {
      if (
        generation !== selectGeneration.current ||
        client !== clientRef.current
      )
        return;
      if (selectedIDRef.current !== sessionID) return;
      if (!shouldClearSessionOnGetError(error)) {
        setNotice(formatRPCError(error, "无法读取所选会话，请重连后重试"));
        return;
      }

      setCurrentSession(null, null);
      applyDraft(null);
      setNotice("所选会话已不存在。");
    }
  }

  async function selectSession(sessionID: string) {
    if (sessionID === selectedIDRef.current && !settings) return;

    setCurrentSession(
      sessionID,
      sessions?.find((item) => item.sessionID === sessionID) ?? null,
    );
    applyDraft(sessionID);
    setNotice("");
    setSettings(false);
    if (window.innerWidth < 760) setSidebar(false);
    const client = clientRef.current;
    if (!client?.connected) return;
    await loadSelected(client, sessionID);
  }

  async function createSession(workspace: string) {
    const client = clientRef.current;
    if (!client?.connected) return;
    setCreatingWorkspace(workspace);
    try {
      const result = await client.create(workspace);
      if (client !== clientRef.current || !client.connected) return;
      await loadSessions(client);
      if (client !== clientRef.current || !client.connected) return;

      setCurrentSession(result.session.sessionID, result.session);
      applyDraft(result.session.sessionID);
      void loadSkills(client, result.session.sessionID);
      setNotice("");
      setSettings(false);
    } finally {
      setCreatingWorkspace(null);
    }
  }

  async function createInWorkspace(workspace: string) {
    if (!clientRef.current?.connected || backendBusy) return;
    try {
      await createSession(workspace);
      return;
    } catch (error) {
      if (!isWorkspaceUnavailable(error)) {
        setNotice(formatRPCError(error, "创建会话失败，当前界面已保留"));
        return;
      }
    }

    // 历史会话可以保留已移动的目录；新会话必须重新定位到真实目录。
    setNotice("项目目录已移动或删除，请重新选择目录。历史会话仍会保留。");
    await openProject();
  }

  async function openProject() {
    const client = clientRef.current;
    if (!client?.connected || backendBusy) return;
    setOpening(true);
    try {
      const picked = await client.selectWorkspace();
      if (picked.canceled) return;
      await createSession(picked.workspace);
    } catch (error) {
      setNotice(
        formatRPCError(error, "选择目录失败，结果不明时请不要重复提交"),
      );
    } finally {
      setOpening(false);
    }
  }

  function reconnect() {
    chatRef.current?.connect();
  }

  async function loadPermissionModes(client: RPCClient) {
    setPermissionModes([]);
    try {
      const result = await client.call("permissions/modes", {});
      if (client !== clientRef.current || !client.connected) return;
      setPermissionModes(result.modes);
    } catch (cause) {
      if (client !== clientRef.current || !client.connected) return;
      setNotice(formatRPCError(cause, "权限模式加载失败，请重新连接"));
    }
  }

  async function loadModels(client: RPCClient) {
    setModelError("");
    try {
      const result = await client.models();
      if (client !== clientRef.current || !client.connected) return;
      setModels(result.models);
    } catch (error) {
      if (client !== clientRef.current) return;
      setModelError(formatRPCError(error, "模型目录加载失败"));
    }
  }

  async function loadAgents(client: RPCClient) {
    setAgentLoading(true);
    setAgentError("");
    try {
      const result = await client.agents();
      if (client !== clientRef.current || !client.connected) return;
      setAgentCatalog(result);
    } catch (error) {
      if (client !== clientRef.current) return;
      const message = formatRPCError(error, "Agent 加载失败");
      setAgentError(message);
      setNotice(message);
    } finally {
      if (client === clientRef.current) setAgentLoading(false);
    }
  }

  async function loadSkills(client: RPCClient, sessionID: string) {
    try {
      const result = await client.skills(sessionID);
      if (
        client !== clientRef.current ||
        !client.connected ||
        selectedIDRef.current !== sessionID
      )
        return;
      setSkills(result.skills);
    } catch (error) {
      if (client !== clientRef.current || selectedIDRef.current !== sessionID)
        return;
      setSkills([]);
      setNotice(formatRPCError(error, "Skill 候选加载失败"));
    }
  }

  async function loadCommands(client: RPCClient) {
    try {
      const result = await client.commands();
      if (client !== clientRef.current || !client.connected) return;
      setCommands(result.commands);
    } catch (error) {
      if (client !== clientRef.current) return;
      setCommands([]);
      setNotice(formatRPCError(error, "命令目录加载失败"));
    }
  }

  function replaceSession(session: SessionView) {
    setSessions(
      (current) =>
        current?.map((item) =>
          item.sessionID === session.sessionID ? session : item,
        ) ?? current,
    );
    if (selectedIDRef.current === session.sessionID) setSelected(session);
  }

  async function updateSessionSettings(value: {
    permissionMode?: PermissionMode;
    agentID: string;
    model: string;
    reasoningEffort: string;
  }) {
    const client = clientRef.current;
    const session = selected;
    if (
      !client?.connected ||
      !session ||
      !synchronized ||
      currentRun ||
      settingsPending.current
    )
      return;
    settingsPending.current = true;
    setSettingsSaving(true);
    setNotice("");
    try {
      const result = await client.updateSettings({
        sessionID: session.sessionID,
        ...value,
      });
      if (client !== clientRef.current || !client.connected) return;
      replaceSession(result.session);
      if (result.session.settings.agentID !== session.settings.agentID) {
        await loadAgents(client);
      }
    } catch (error) {
      setNotice(`${formatRPCError(error, "设置保存失败")}。仍使用后台原设置。`);
    } finally {
      settingsPending.current = false;
      setSettingsSaving(false);
    }
  }

  async function saveAgent(input: AgentSaveParams): Promise<AgentView | null> {
    const client = clientRef.current;
    if (!client?.connected || agentSaving) return null;
    setAgentSaving(true);
    setAgentError("");
    try {
      const result = await client.saveAgent(input);
      await loadAgents(client);
      return result.agent;
    } catch (error) {
      setAgentError(formatRPCError(error, "Agent 保存失败"));
      return null;
    } finally {
      setAgentSaving(false);
    }
  }

  async function deleteAgent(agentID: string): Promise<boolean> {
    const client = clientRef.current;
    if (!client?.connected || agentSaving) return false;
    setAgentSaving(true);
    setAgentError("");
    try {
      await client.deleteAgent(agentID);
      await loadAgents(client);
      return true;
    } catch (error) {
      setAgentError(formatRPCError(error, "Agent 删除失败"));
      return false;
    } finally {
      setAgentSaving(false);
    }
  }

  async function sendMessage() {
    const client = clientRef.current;
    const id = selectedIDRef.current;
    if (
      !id ||
      !client?.connected ||
      !canSend ||
      chatRef.current?.state.syncing ||
      sendingRef.current.has(id) ||
      settingsPending.current ||
      compressionPending.current
    )
      return;
    const text = draftRef.current.text;
    const submittedImages = [...draftRef.current.images];
    const submittedReferences = [...draftRef.current.references];
    const version = drafts.current.get(id)?.version ?? 0;
    sendingRef.current.add(id);
    setSending([...sendingRef.current]);
    setNotice("");
    try {
      await client.send(
        chatSendParams(
          id,
          encodeReferences(
            text,
            submittedReferences.map((item) => item.reference),
          ),
          submittedImages,
          currentRun?.runID,
        ),
      );
      // 文字按编辑版本清理；图片和引用按 ID 清理，保留等待期间的新输入。
      const visible = selectedIDRef.current === id;
      const currentDraft = visible
        ? draftRef.current
        : (drafts.current.get(id) ?? {
            text,
            images: submittedImages,
            references: submittedReferences,
            version,
          });
      const submittedImageIDs = new Set(
        submittedImages.map((image) => image.id),
      );
      const nextDraft = {
        version: currentDraft.version,
        references: clearSubmittedReferences(
          currentDraft.references,
          submittedReferences,
        ),
        text: shouldClearSubmittedDraft(drafts.current, id, version)
          ? ""
          : currentDraft.text,
        images: currentDraft.images.filter(
          (image) => !submittedImageIDs.has(image.id),
        ),
      };
      drafts.current.set(id, nextDraft);
      for (const image of submittedImages) URL.revokeObjectURL(image.url);
      if (visible) {
        draftRef.current = nextDraft;
        setComposerDraft(nextDraft);
      }
      if (client === clientRef.current && client.connected)
        void loadSessions(client);
    } catch (error) {
      if (selectedIDRef.current === id) {
        setNotice(
          error instanceof RPCError && error.code === -32009
            ? "本轮已结束或变化，草稿已保留。请确认后再次发送。"
            : `${formatRPCError(error, "发送失败")}。草稿已保留；结果不明时请先核对历史，不会自动重发。`,
        );
      }
    } finally {
      sendingRef.current.delete(id);
      setSending([...sendingRef.current]);
    }
  }

  async function stopRun() {
    const client = clientRef.current;
    const id = selectedIDRef.current;
    if (
      !id ||
      !client?.connected ||
      !synchronized ||
      !currentRun ||
      stoppingCurrent ||
      stopPending.current
    )
      return;
    const target = { sessionID: id, runID: currentRun.runID };
    stopPending.current = true;
    setStopping(target);
    try {
      await client.stop(id);
      // RPC 响应只确认停止请求；运行状态仍以 run-ended / Snapshot 为准。
    } catch (error) {
      setStopping((current) => (current === target ? null : current));
      if (selectedIDRef.current === id)
        setNotice(formatRPCError(error, "停止请求失败，请同步后确认状态"));
    } finally {
      stopPending.current = false;
    }
  }

  async function executeCommand(
    name: string,
    selection: CommandSelection,
  ): Promise<void> {
    const client = clientRef.current;
    const sessionID = selectedIDRef.current;
    if (
      !client?.connected ||
      !sessionID ||
      !synchronized ||
      currentRun ||
      commandPending.current
    )
      return;
    const key = draftKey(sessionID);
    const version = drafts.current.get(key)?.version ?? 0;
    commandPending.current = true;
    setCommandBusy(true);
    setNotice("");
    try {
      await client.callCommand(sessionID, name);

      const visible = selectedIDRef.current === sessionID;
      const currentDraft = visible ? draftRef.current : drafts.current.get(key);
      if (
        !currentDraft ||
        currentDraft.text !== selection.draft ||
        (drafts.current.get(key)?.version ?? 0) !== version
      )
        return;

      const nextDraft = {
        version: version + 1,
        text: `${selection.draft.slice(0, selection.start)}${selection.draft.slice(selection.end)}`,
        images: currentDraft.images,
        references: currentDraft.references,
      };
      drafts.current.set(key, nextDraft);
      if (visible) {
        draftRef.current = nextDraft;
        setComposerDraft(nextDraft);
      }
    } catch (error) {
      if (selectedIDRef.current === sessionID)
        setNotice(
          `${formatRPCError(error, "命令执行失败")}。输入已保留，不会自动重试。`,
        );
    } finally {
      commandPending.current = false;
      setCommandBusy(false);
    }
  }

  async function forkAnswer(runID: string, boundaryEntryID: string) {
    const client = clientRef.current;
    const sourceID = selectedIDRef.current;
    if (!client?.connected || !sourceID || !synchronized || forkPending.current)
      return;
    forkPending.current = true;
    setForkingEntryID(boundaryEntryID);
    setNotice("");
    try {
      const result = await client.fork(sourceID, runID, boundaryEntryID);
      if (client !== clientRef.current || !client.connected) return;
      setSessions((current) => {
        if (!current) return [result.session];
        return [
          result.session,
          ...current.filter(
            (item) => item.sessionID !== result.session.sessionID,
          ),
        ];
      });
      if (selectedIDRef.current === sourceID) {
        setCurrentSession(result.session.sessionID, result.session);
        applyDraft(result.session.sessionID);
        setSettings(false);
        setNotice("已从该回答创建分叉会话。");
        void loadSkills(client, result.session.sessionID);
      }
      void loadSessions(client);
    } catch (error) {
      if (selectedIDRef.current === sourceID)
        setNotice(
          `${formatRPCError(error, "分叉失败")}。当前会话和草稿已保留，不会自动重试。`,
        );
    } finally {
      forkPending.current = false;
      setForkingEntryID("");
    }
  }

  useEffect(() => {
    function measureViewport() {
      setViewportWidth(window.innerWidth);
    }
    window.addEventListener("resize", measureViewport);
    return () => window.removeEventListener("resize", measureViewport);
  }, []);
  useEffect(() => {
    const query = matchMedia("(prefers-color-scheme: dark)");
    function applyTheme() {
      document.documentElement.classList.toggle(
        "dark",
        theme === "dark" || (theme === "system" && query.matches),
      );
    }
    applyTheme();
    query.addEventListener("change", applyTheme);
    savePreference("theme", theme);
    return () => query.removeEventListener("change", applyTheme);
  }, [theme]);
  useEffect(() => {
    savePreference("panel", String(panel));
    savePreference("reading-panel-width", String(panelWidth));
    savePreference("sidebar-width", String(sidebarWidth));
  }, [panel, panelWidth, sidebarWidth]);
  useEffect(
    () => () => {
      objectUrls.current.forEach((url) => URL.revokeObjectURL(url));
    },
    [],
  );
  useEffect(() => {
    const chat = new ChatConnection(
      rpcURL(),
      setChatState,
      (client) => {
        clientRef.current = client;
        setApprovalClient(client);
        void loadSessions(client);
        void loadModels(client);
        void loadPermissionModes(client);
        void loadAgents(client);
        void loadCommands(client);
      },
      (client) => {
        void loadSessions(client);
      },
    );
    chatRef.current = chat;
    chat.select(selectedIDRef.current);
    chat.connect();
    return () => {
      clientRef.current = null;
      chatRef.current = null;
      listGeneration.current++;
      selectGeneration.current++;
      chat.close();
    };
  }, []);
  useEffect(() => {
    if (chatState.missing && chatState.sessionID === selectedIDRef.current) {
      setCurrentSession(null, null);
      applyDraft(null);
      setNotice("所选会话已不存在。");
    }
    if (
      stopping?.sessionID === chatState.sessionID &&
      chatState.snapshot &&
      !chatState.syncing &&
      !chatState.snapshot.runs.some(
        (run) => run.runID === stopping.runID && run.status === "running",
      )
    )
      setStopping(null);
  }, [chatState, stopping]);

  return (
    <TooltipProvider>
      <div
        ref={appElement}
        className="app"
        data-panel-overlay={panelOverlay}
        style={
          {
            "--panel-width": `${visiblePanelWidth}px`,
            "--sidebar-width": `${sidebarWidth}px`,
          } as CSSProperties
        }
      >
        {sidebar && (
          <Sidebar
            connection={connection}
            backendBusy={backendBusy}
            sessions={sessions}
            listError={listError}
            selectedID={selectedID}
            settingsOpen={settings}
            onClose={() => setSidebar(false)}
            onOpenProject={() => void openProject()}
            onReload={() => {
              const client = clientRef.current;
              if (client?.connected) void loadSessions(client);
            }}
            onSelect={(sessionID) => void selectSession(sessionID)}
            onCreate={(workspace) => void createInWorkspace(workspace)}
            onOpenSettings={openSettings}
            onReconnect={reconnect}
          />
        )}
        {sidebar && (
          <ResizeHandle
            label="调整项目侧栏宽度"
            value={sidebarWidth}
            min={200}
            max={420}
            growToward="right"
            className="sidebar-resize-handle"
            onResize={(width) =>
              appElement.current?.style.setProperty(
                "--sidebar-width",
                `${width}px`,
              )
            }
            onChange={setSidebarWidth}
          />
        )}
        <main className="main">
          <header className="topbar">
            <div className="topbar-title">
              {!sidebar && (
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="展开项目侧栏"
                  onClick={() => setSidebar(true)}
                >
                  <PanelLeft />
                </Button>
              )}
              <Folder />
              {settings ? (
                <span>设置</span>
              ) : selected ? (
                <>
                  <span>{workspaceName(selected.settings.workspace)}</span>
                  <span className="breadcrumb-divider">/</span>
                  <span className="title-truncate">{selected.title}</span>
                </>
              ) : (
                <span>聊天</span>
              )}
            </div>
            <div className="topbar-actions">
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={panel ? "收起辅助工作区" : "打开辅助工作区"}
                    onClick={() => setPanel(!panel)}
                  >
                    <PanelRight />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>辅助工作区</TooltipContent>
              </Tooltip>
            </div>
          </header>
          <div className="workspace">
            {settings ? (
              <SettingsPage
                hookSettings={
                  <HookSettingsPanel
                    client={approvalClient}
                    currentWorkspace={selected?.settings.workspace ?? ""}
                  />
                }
                approvalSettings={
                  <ApprovalSettingsPanel
                    client={approvalClient}
                    models={models}
                    modelError={modelError}
                    onReloadModels={() => {
                      const client = clientRef.current;
                      if (client?.connected) void loadModels(client);
                    }}
                    onSaved={() => {
                      const client = clientRef.current;
                      if (client?.connected) void loadPermissionModes(client);
                    }}
                  />
                }
                theme={theme}
                setTheme={setTheme}
                onBack={() => setSettings(false)}
                agents={agentCatalog?.agents ?? null}
                kinds={agentCatalog?.kinds ?? []}
                tools={agentCatalog?.tools ?? []}
                loading={agentLoading}
                error={agentError}
                saving={agentSaving}
                onReload={() => {
                  const client = clientRef.current;
                  if (client?.connected) void loadAgents(client);
                }}
                onSave={saveAgent}
                onDelete={deleteAgent}
              />
            ) : (
              <section className="chat" aria-label="聊天">
                {connection !== "connected" && (
                  <div className="connection-banner" role="status">
                    <WifiOff />
                    <span>
                      {connection === "connecting"
                        ? "正在连接后台…"
                        : connectionDetail ||
                          "连接已断开。后台任务不会因此停止，草稿仍保留在本页。"}
                    </span>
                    {connection === "disconnected" && (
                      <Button variant="ghost" size="sm" onClick={reconnect}>
                        <RefreshCw />
                        重新连接
                      </Button>
                    )}
                  </div>
                )}
                {connected &&
                  selectedID &&
                  (chatState.syncing || chatState.error) && (
                    <div className="connection-banner" role="status">
                      <span>
                        {chatState.syncing
                          ? "正在同步历史与运行状态…"
                          : chatState.error}
                      </span>
                      {!chatState.syncing && (
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => void chatRef.current?.synchronize()}
                        >
                          重新同步
                        </Button>
                      )}
                    </div>
                  )}
                {chatState.notice && (
                  <div className="connection-banner" role="status">
                    {chatState.notice}
                  </div>
                )}
                <ChatMessages
                  snapshot={snapshot}
                  sessionID={selectedID}
                  stoppingRunID={
                    stopping?.sessionID === selectedID
                      ? stopping.runID
                      : undefined
                  }
                  forkingEntryID={forkingEntryID || undefined}
                  forkDisabled={!synchronized || !!currentRun}
                  workspace={selected?.settings.workspace}
                  onOpenFile={(location) => {
                    const workspace = selected?.settings.workspace;
                    if (!workspace) return;
                    setPanel(true);
                    setSettings(false);
                    setFileOpenRequest({
                      ...location,
                      workspace,
                      requestID: ++fileOpenRequestID.current,
                    });
                  }}
                  onOpenDiff={(runID) => {
                    if (!selectedID) return;
                    setPanel(true);
                    setSettings(false);
                    setReviewOpenRequest({
                      sessionID: selectedID,
                      runID,
                      requestID: ++reviewOpenRequestID.current,
                    });
                  }}
                  onOpenSubagent={(taskID) => {
                    if (!selectedID) return;
                    setPanel(true);
                    setSettings(false);
                    setSubagentOpenRequest({
                      parentSessionID: selectedID,
                      taskID,
                      requestID: ++subagentOpenRequestID.current,
                    });
                  }}
                  onAddReference={addContextReference}
                  pendingReferences={references}
                  onFork={(runID, boundaryEntryID) =>
                    void forkAnswer(runID, boundaryEntryID)
                  }
                >
                  <div className="empty-chat">
                    <div className="empty-symbol">
                      <Command />
                    </div>
                    {selectedID ? (
                      <>
                        <h1>{selected?.title ?? "正在恢复会话"}</h1>
                        <p>
                          {!synchronized
                            ? "正在等待后台同步，已有内容不会被当作空会话。"
                            : "选择模型和思考档位，开始这场对话。"}
                        </p>
                      </>
                    ) : (
                      <>
                        <h1>从一个想法开始。</h1>
                        <p>
                          {connected
                            ? "选择左侧会话，或打开一个项目。"
                            : "后台尚未连接。输入会留在本页，不会发送。"}
                        </p>
                        <div className="starter-actions">
                          {[
                            "梳理项目结构",
                            "帮我检查代码",
                            "一起设计新功能",
                          ].map((text) => (
                            <Button
                              key={text}
                              variant="outline"
                              onClick={() => {
                                editDraft(text);
                                composer.current?.focus();
                              }}
                            >
                              {text}
                            </Button>
                          ))}
                        </div>
                      </>
                    )}
                  </div>
                </ChatMessages>
                <Approvals client={connected ? approvalClient : null}>
                  <Composer
                    key={selectedID ?? ""}
                    references={references}
                    referenceWorkspace={referenceWorkspace}
                    onSearchPaths={searchContextPaths}
                    onAddReference={addContextReference}
                    onRemoveReference={removeReference}
                    draft={draft}
                    images={images}
                    notice={notice}
                    agents={agentCatalog?.agents ?? null}
                    agentID={selected?.settings.agentID ?? ""}
                    settingsDisabled={settingsDisabled}
                    permissionMode={selected?.settings.permissionMode}
                    permissionModes={permissionModes}
                    onPermissionChange={(permissionMode) =>
                      void updateSessionSettings({
                        agentID: selected?.settings.agentID ?? "",
                        model: selected?.settings.model ?? "",
                        reasoningEffort:
                          selected?.settings.reasoningEffort ?? "",
                        permissionMode,
                      })
                    }
                    usage={sessionUsage}
                    running={!!currentRun}
                    stopping={stoppingCurrent}
                    canSend={!!canSend}
                    stopDisabled={!synchronized || stoppingCurrent}
                    modelDisabled={settingsDisabled}
                    validModel={!!validModel}
                    models={models}
                    modelSelection={modelSelection}
                    modelError={modelError}
                    imageDisabled={compressingImages || !selectedModel?.vision}
                    skills={skills}
                    commands={commands}
                    suggestionsDisabled={!selected || !synchronized}
                    commandBusy={commandBusy}
                    onDraftChange={editDraft}
                    onSend={() => void sendMessage()}
                    onStop={() => void stopRun()}
                    onAddImages={(files) => void addImages(files)}
                    onRemoveImage={removeImage}
                    onModelChange={(value) =>
                      void updateSessionSettings({
                        agentID: selected?.settings.agentID ?? "",
                        model: value.model,
                        reasoningEffort: value.reasoningEffort,
                      })
                    }
                    onAgentChange={(agentID) =>
                      void updateSessionSettings({
                        agentID,
                        model: selected?.settings.model ?? "",
                        reasoningEffort:
                          selected?.settings.reasoningEffort ?? "",
                      })
                    }
                    onRetryModels={() => {
                      if (clientRef.current?.connected)
                        void loadModels(clientRef.current);
                    }}
                    onCommand={executeCommand}
                    onDismissNotice={() => setNotice("")}
                    composerRef={composer}
                  />
                </Approvals>
              </section>
            )}
          </div>
        </main>
        <>
          {panel && (
            <button
              className="panel-scrim"
              aria-label="关闭辅助区覆盖层"
              onClick={() => setPanel(false)}
            />
          )}
          <aside className="aux-panel" aria-label="辅助工作区" hidden={!panel}>
            <ResizeHandle
              label="调整辅助工作区宽度"
              value={visiblePanelWidth}
              min={320}
              max={panelMaxWidth}
              growToward="left"
              onResize={(width) =>
                appElement.current?.style.setProperty(
                  "--panel-width",
                  `${width}px`,
                )
              }
              onChange={setPanelWidth}
            />
            <WorkspaceTabs
              onAddReference={addContextReference}
              workspace={selected?.settings.workspace ?? null}
              sessionID={selectedID}
              sessionTitle={selected?.title ?? null}
              runs={snapshot?.runs ?? []}
              runActive={!!currentRun}
              client={connected ? clientRef.current : null}
              openRequest={fileOpenRequest}
              reviewRequest={reviewOpenRequest}
              subagentRequest={subagentOpenRequest}
              models={models}
              agents={agentCatalog?.agents ?? null}
              onHide={() => setPanel(false)}
            />
          </aside>
        </>
        <AppContextMenu
          onAddReference={
            selected && selectedID === selected.sessionID
              ? addContextReference
              : undefined
          }
        />
      </div>
    </TooltipProvider>
  );
}
