import { useEffect, useRef, useState, type CSSProperties } from "react";
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
import { SettingsPage } from "./settings-page";
import { WorkspaceTabs } from "./workspace-tabs";
import { Sidebar } from "./sidebar";
import {
  Composer,
  type Attachment,
  type CommandSelection,
  type ComposerHandle,
} from "./composer";
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
import { ChatMessages } from "./chat-messages";
import { type ModelSelection } from "./model-menu";
import { compressImage } from "./image-compression";
import { workspaceName } from "./state/projects";
import type { SendParams, SessionView } from "../../contracts/harness.ts";
import type {
  AgentListResult,
  AgentSaveParams,
  AgentView,
  CommandView,
  ModelChoice,
  SkillView,
} from "../../contracts/appserver.ts";

type Draft = { text: string; images: Attachment[] };

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
  versions: Map<string, number>,
  sessionID: string,
  submittedVersion: number,
): boolean {
  return (versions.get(sessionID) ?? 0) === submittedVersion;
}

export default function App() {
  const [settings, setSettings] = useState(false);
  const [sidebar, setSidebar] = useState(true);
  const [panel, setPanel] = useState(
    () => preference("panel", "false") === "true",
  );
  const [panelWidth, setPanelWidth] = useState(() =>
    Math.max(
      300,
      Math.min(640, Number(preference("panel-width", "380")) || 380),
    ),
  );
  const [theme, setTheme] = useState(() => preference("theme", "system"));
  const [draft, setDraft] = useState("");
  const [images, setImages] = useState<Attachment[]>([]);
  const [notice, setNotice] = useState("");
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
  const composer = useRef<ComposerHandle>(null);
  const objectUrls = useRef<string[]>([]);
  const drafts = useRef(new Map<string, Draft>());
  const clientRef = useRef<RPCClient | null>(null);
  const chatRef = useRef<ChatConnection | null>(null);
  const selectedIDRef = useRef<string | null>(selectedID);
  const draftVersions = useRef(new Map<string, number>());
  const sendingRef = useRef(new Set<string>());
  const stopPending = useRef(false);
  const settingsPending = useRef(false);
  const compressionPending = useRef(false);
  const commandPending = useRef(false);
  const forkPending = useRef(false);
  const draftRef = useRef(draft);
  const imagesRef = useRef(images);
  const selectGeneration = useRef(0);
  const listGeneration = useRef(0);
  draftRef.current = draft;
  imagesRef.current = images;

  const connected = connection === "connected";
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
  const agentLabel =
    agentCatalog?.agents.find(
      (agent) => agent.id === selected?.settings.agentID,
    )?.name ??
    selected?.settings.agentID ??
    "未加载";
  const canSend =
    synchronized &&
    !!selected &&
    !busySending &&
    !stoppingCurrent &&
    !commandBusy &&
    !compressingImages &&
    (!!draft.trim() || images.length > 0) &&
    (!!currentRun || (!!validAgent && !!validModel && !modelError)) &&
    (!images.length || !!selectedModel?.vision);

  function editDraft(text: string) {
    const key = draftKey(selectedIDRef.current);
    draftVersions.current.set(key, (draftVersions.current.get(key) ?? 0) + 1);
    draftRef.current = text;
    setDraft(text);
  }

  function rememberDraft(
    sessionID: string | null,
    text: string,
    attachments: Attachment[],
  ) {
    drafts.current.set(draftKey(sessionID), { text, images: attachments });
  }
  function applyDraft(sessionID: string | null) {
    const stored = drafts.current.get(draftKey(sessionID)) ?? {
      text: "",
      images: [],
    };
    setDraft(stored.text);
    setImages(stored.images);
    draftRef.current = stored.text;
    imagesRef.current = stored.images;
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
    const initialImages = [...imagesRef.current];
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
        imagesRef.current = next;
        setImages(next);
      } else {
        const stored = drafts.current.get(key) ?? {
          text: "",
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
    const removed = imagesRef.current.find((item) => item.id === id);
    if (removed) URL.revokeObjectURL(removed.url);
    const next = imagesRef.current.filter((item) => item.id !== id);
    imagesRef.current = next;
    setImages(next);
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
      rememberDraft(sessionID, draftRef.current, imagesRef.current);
      setCurrentSession(null, null);
      applyDraft(null);
      setNotice("所选会话已不存在。");
    }
  }

  async function selectSession(sessionID: string) {
    if (sessionID === selectedIDRef.current && !settings) return;
    rememberDraft(selectedIDRef.current, draftRef.current, imagesRef.current);
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
      rememberDraft(selectedIDRef.current, draftRef.current, imagesRef.current);
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
    const text = draftRef.current;
    const submittedImages = [...imagesRef.current];
    const version = draftVersions.current.get(id) ?? 0;
    sendingRef.current.add(id);
    setSending([...sendingRef.current]);
    setNotice("");
    try {
      await client.send(
        chatSendParams(id, text, submittedImages, currentRun?.runID),
      );
      // 文字按编辑版本清理；图片按 ID 清理，保留等待期间的新输入。
      const visible = selectedIDRef.current === id;
      const currentDraft = visible
        ? { text: draftRef.current, images: imagesRef.current }
        : (drafts.current.get(id) ?? { text, images: submittedImages });
      const submittedImageIDs = new Set(
        submittedImages.map((image) => image.id),
      );
      const nextDraft = {
        text: shouldClearSubmittedDraft(draftVersions.current, id, version)
          ? ""
          : currentDraft.text,
        images: currentDraft.images.filter(
          (image) => !submittedImageIDs.has(image.id),
        ),
      };
      drafts.current.set(id, nextDraft);
      for (const image of submittedImages) URL.revokeObjectURL(image.url);
      if (visible) {
        draftRef.current = nextDraft.text;
        imagesRef.current = nextDraft.images;
        setDraft(nextDraft.text);
        setImages(nextDraft.images);
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
    const version = draftVersions.current.get(key) ?? 0;
    commandPending.current = true;
    setCommandBusy(true);
    setNotice("");
    try {
      await client.callCommand(sessionID, name);

      const visible = selectedIDRef.current === sessionID;
      const currentDraft = visible
        ? { text: draftRef.current, images: imagesRef.current }
        : drafts.current.get(key);
      if (
        !currentDraft ||
        currentDraft.text !== selection.draft ||
        (draftVersions.current.get(key) ?? 0) !== version
      )
        return;

      const nextDraft = {
        text: `${selection.draft.slice(0, selection.start)}${selection.draft.slice(selection.end)}`,
        images: currentDraft.images,
      };
      drafts.current.set(key, nextDraft);
      draftVersions.current.set(key, version + 1);
      if (visible) {
        draftRef.current = nextDraft.text;
        setDraft(nextDraft.text);
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
    if (
      !client?.connected ||
      !sourceID ||
      !synchronized ||
      forkPending.current
    )
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
        rememberDraft(sourceID, draftRef.current, imagesRef.current);
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
    savePreference("panel-width", String(panelWidth));
  }, [panel, panelWidth]);
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
        void loadSessions(client);
        void loadModels(client);
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
      rememberDraft(selectedIDRef.current, draftRef.current, imagesRef.current);
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
        className="app"
        style={{ "--panel-width": `${panelWidth}px` } as CSSProperties}
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
                <Composer
                  draft={draft}
                  images={images}
                  notice={notice}
                  agentLabel={agentLabel}
                  agents={agentCatalog?.agents ?? null}
                  agentID={selected?.settings.agentID ?? ""}
                  settingsDisabled={settingsDisabled}
                  usage={sessionUsage}
                  compressingImages={compressingImages}
                  running={!!currentRun}
                  stopping={stoppingCurrent}
                  busySending={busySending}
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
                      reasoningEffort: selected?.settings.reasoningEffort ?? "",
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
            <div
              role="separator"
              aria-label="调整辅助工作区宽度"
              aria-orientation="vertical"
              aria-valuemin={300}
              aria-valuemax={640}
              aria-valuenow={panelWidth}
              tabIndex={0}
              className="resize-handle"
              onKeyDown={(event) => {
                if (event.key === "ArrowLeft")
                  setPanelWidth((width) => Math.min(640, width + 20));
                if (event.key === "ArrowRight")
                  setPanelWidth((width) => Math.max(300, width - 20));
              }}
              onPointerDown={(event) =>
                event.currentTarget.setPointerCapture(event.pointerId)
              }
              onPointerMove={(event) => {
                if (event.currentTarget.hasPointerCapture(event.pointerId))
                  setPanelWidth(
                    Math.max(
                      300,
                      Math.min(640, window.innerWidth - event.clientX),
                    ),
                  );
              }}
              onPointerUp={(event) =>
                event.currentTarget.releasePointerCapture(event.pointerId)
              }
            />
            <WorkspaceTabs onHide={() => setPanel(false)} />
          </aside>
        </>
      </div>
    </TooltipProvider>
  );
}
