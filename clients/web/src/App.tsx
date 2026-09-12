import { useEffect, useRef, useState, type CSSProperties } from "react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
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
  FolderOpen,
  Plus,
  ChevronRight,
  ArrowUp,
  Square,
  X,
  Settings,
  Bot,
  Circle,
  Command,
  WifiOff,
  RefreshCw,
} from "./icons";
import { SettingsPage } from "./settings-page";
import { WorkspaceTabs } from "./workspace-tabs";
import {
  formatRPCError,
  RPCClient,
  RPCError,
  rpcURL,
  shouldClearSessionOnGetError,
} from "./client/rpc";
import { ChatConnection, initialChatState } from "./client/chat";
import { activeRun } from "./state/chat";
import { ChatMessages } from "./chat-messages";
import { ModelMenu, type ModelSelection } from "./model-menu";
import { groupSessions, workspaceName } from "./state/projects";
import type { SessionView } from "../../contracts/harness.ts";
import type { ModelChoice } from "../../contracts/appserver.ts";

type Attachment = { id: string; name: string; url: string };
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
  const [modelSelections, setModelSelections] = useState<
    Record<string, ModelSelection>
  >({});
  const [sending, setSending] = useState<string[]>([]);
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
  const imageInput = useRef<HTMLInputElement>(null);
  const input = useRef<HTMLTextAreaElement>(null);
  const objectUrls = useRef<string[]>([]);
  const drafts = useRef(new Map<string, Draft>());
  const clientRef = useRef<RPCClient | null>(null);
  const chatRef = useRef<ChatConnection | null>(null);
  const selectedIDRef = useRef<string | null>(selectedID);
  const draftVersions = useRef(new Map<string, number>());
  const sendingRef = useRef(new Set<string>());
  const stopPending = useRef(false);
  const draftRef = useRef(draft);
  const imagesRef = useRef(images);
  const selectGeneration = useRef(0);
  const listGeneration = useRef(0);
  draftRef.current = draft;
  imagesRef.current = images;

  const connected = connection === "connected";
  const backendBusy = opening || creatingWorkspace !== null;
  const projects = sessions ? groupSessions(sessions) : [];
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
  const modelSelection = (selectedID && modelSelections[selectedID]) || {
    model: selected?.settings.model ?? "",
    reasoningEffort: selected?.settings.reasoningEffort ?? "",
  };
  const validModel = models?.some(
    (item) =>
      item.id === modelSelection.model &&
      item.reasoningEfforts.includes(modelSelection.reasoningEffort),
  );
  const canSend =
    synchronized &&
    !!selected &&
    !busySending &&
    !stoppingCurrent &&
    !!draft.trim() &&
    (!!currentRun || (validModel && !modelError));

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
  function addImages(files: FileList | File[] | null) {
    if (!files) return;
    const attachments: Attachment[] = [];
    for (const file of Array.from(files)) {
      if (
        !["image/png", "image/jpeg", "image/webp", "image/gif"].includes(
          file.type,
        )
      ) {
        setNotice("请选择 PNG、JPEG、WebP 或 GIF 图片。");
        continue;
      }
      if (file.size > 10 * 1024 * 1024) {
        setNotice("单张图片请不超过 10 MB。");
        continue;
      }
      const url = URL.createObjectURL(file);
      objectUrls.current.push(url);
      attachments.push({ id: crypto.randomUUID(), name: file.name, url });
    }
    setImages((current) => [...current, ...attachments].slice(0, 4));
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
      setSettings(false);
    } catch (error) {
      setNotice(formatRPCError(error, "创建会话失败，当前界面已保留"));
    } finally {
      setCreatingWorkspace(null);
    }
  }

  async function createInWorkspace(workspace: string) {
    if (!clientRef.current?.connected || backendBusy) return;
    await createSession(workspace);
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

  async function sendMessage() {
    const client = clientRef.current;
    const id = selectedIDRef.current;
    if (
      !id ||
      !client?.connected ||
      !canSend ||
      chatRef.current?.state.syncing ||
      sendingRef.current.has(id)
    )
      return;
    if (imagesRef.current.length) {
      setNotice("图片发送尚未接入，请先移除附件。不会只发送文字。");
      return;
    }
    const text = draftRef.current;
    const version = draftVersions.current.get(id) ?? 0;
    sendingRef.current.add(id);
    setSending([...sendingRef.current]);
    setNotice("");
    try {
      await client.send({
        sessionID: id,
        text,
        model: modelSelection.model,
        reasoningEffort: modelSelection.reasoningEffort,
        ...(currentRun ? { expectedRunID: currentRun.runID } : {}),
      });
      // 确认只清这次输入；用户编辑过或已切到别的会话都不能被覆盖。
      if ((draftVersions.current.get(id) ?? 0) === version) {
        const saved = drafts.current.get(id);
        if (saved) drafts.current.set(id, { ...saved, text: "" });
        if (selectedIDRef.current === id) {
          draftRef.current = "";
          setDraft("");
        }
      }
      if (client === clientRef.current && client.connected)
        void loadSessions(client);
    } catch (error) {
      if (selectedIDRef.current === id) {
        setNotice(
          error instanceof RPCError && error.code === -32009
            ? "本轮已结束或变化，文字已保留。请确认后再次发送。"
            : `${formatRPCError(error, "发送失败")}。文字已保留；结果不明时请先核对历史，不会自动重发。`,
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

  const connectionLabel =
    connection === "connecting"
      ? "正在连接"
      : connection === "connected"
        ? "已连接"
        : "已断开";

  return (
    <TooltipProvider>
      <div
        className="app"
        style={{ "--panel-width": `${panelWidth}px` } as CSSProperties}
      >
        {sidebar && (
          <>
            <button
              className="sidebar-scrim"
              aria-label="关闭项目侧栏"
              onClick={() => setSidebar(false)}
            />
            <aside className="sidebar">
              <div className="brand-row">
                <span className="brand">
                  Harness<span className="brand-period">.</span>
                </span>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="收起项目侧栏"
                  onClick={() => setSidebar(false)}
                >
                  <PanelLeft />
                </Button>
              </div>
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="open-project-wrap">
                    <Button
                      variant="ghost"
                      className="open-project"
                      disabled={!connected || backendBusy}
                      onClick={() => void openProject()}
                    >
                      <FolderOpen />
                      打开项目
                      <Plus className="ml-auto" />
                    </Button>
                  </span>
                </TooltipTrigger>
                {!connected && <TooltipContent>尚未连接后台</TooltipContent>}
              </Tooltip>
              <div className="sidebar-heading">项目</div>
              <nav className="project-list" aria-label="项目与会话">
                {connection !== "connected" &&
                  sessions === null &&
                  !listError && (
                    <p className="metadata sidebar-empty">
                      {connection === "connecting"
                        ? "正在连接后台…"
                        : "尚未连接后台。"}
                    </p>
                  )}
                {connected && sessions === null && !listError && (
                  <p className="metadata sidebar-empty">正在加载项目…</p>
                )}
                {listError && (
                  <div className="sidebar-empty">
                    <p className="metadata">{listError}</p>
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={!connected}
                      onClick={() => {
                        const client = clientRef.current;
                        if (client?.connected) void loadSessions(client);
                      }}
                    >
                      重新加载列表
                    </Button>
                  </div>
                )}
                {sessions && sessions.length === 0 && !listError && (
                  <p className="metadata sidebar-empty">
                    还没有项目。打开一个目录开始。
                  </p>
                )}
                {projects.map((project) => (
                  <Collapsible
                    defaultOpen
                    key={project.workspace}
                    className="project-group"
                  >
                    <div className="project-heading">
                      <CollapsibleTrigger className="project-trigger">
                        <ChevronRight className="disclosure-chevron" />
                        <Folder />
                        <span title={project.workspace}>{project.name}</span>
                      </CollapsibleTrigger>
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={`在 ${project.name} 新建会话`}
                        disabled={!connected || backendBusy}
                        onClick={() =>
                          void createInWorkspace(project.workspace)
                        }
                      >
                        <Plus />
                      </Button>
                    </div>
                    <CollapsibleContent>
                      {project.sessions.map((item) => (
                        <button
                          key={item.sessionID}
                          className={`session-row ${item.sessionID === selectedID && !settings ? "selected" : ""}`}
                          onClick={() => void selectSession(item.sessionID)}
                        >
                          <span>{item.title}</span>
                        </button>
                      ))}
                    </CollapsibleContent>
                  </Collapsible>
                ))}
              </nav>
              <div className="sidebar-bottom">
                <Button
                  variant="ghost"
                  className={settings ? "selected" : ""}
                  onClick={openSettings}
                >
                  <Settings />
                  设置
                </Button>
                <div className="local-caption">
                  <Circle />
                  本机工作空间<span>{connectionLabel}</span>
                </div>
                {connection === "disconnected" && (
                  <Button variant="ghost" size="sm" onClick={reconnect}>
                    <RefreshCw />
                    重新连接
                  </Button>
                )}
              </div>
            </aside>
          </>
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
                <ChatMessages snapshot={snapshot} sessionID={selectedID}>
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
                                input.current?.focus();
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
                <div className="composer-area">
                  <div className="composer-column">
                    {notice && (
                      <div role="status" className="inline-notice">
                        {notice}
                        <button
                          aria-label="关闭提示"
                          onClick={() => setNotice("")}
                        >
                          <X />
                        </button>
                      </div>
                    )}
                    <div className="composer">
                      {images.length > 0 && (
                        <div className="attachments">
                          {images.map((image) => (
                            <div key={image.id}>
                              <img src={image.url} alt={image.name} />
                              <button
                                aria-label={`移除图片 ${image.name}`}
                                onClick={() =>
                                  setImages(
                                    images.filter(
                                      (item) => item.id !== image.id,
                                    ),
                                  )
                                }
                              >
                                <X />
                              </button>
                            </div>
                          ))}
                        </div>
                      )}
                      <Textarea
                        ref={input}
                        aria-label="消息输入"
                        placeholder={
                          currentRun ? "发送以调整当前任务" : "说说你的想法"
                        }
                        value={draft}
                        onChange={(event) => editDraft(event.target.value)}
                        onKeyDown={(event) => {
                          if (
                            event.key === "Enter" &&
                            !event.shiftKey &&
                            !event.nativeEvent.isComposing &&
                            event.keyCode !== 229
                          ) {
                            event.preventDefault();
                            void sendMessage();
                          }
                        }}
                        onPaste={(event) => {
                          const files = Array.from(event.clipboardData.files);
                          if (files.length) {
                            event.preventDefault();
                            addImages(files);
                          }
                        }}
                      />
                      <div className="composer-toolbar">
                        <div className="composer-left">
                          <input
                            ref={imageInput}
                            type="file"
                            hidden
                            accept="image/png,image/jpeg,image/webp,image/gif"
                            multiple
                            onChange={(event) => {
                              addImages(event.target.files);
                              event.target.value = "";
                            }}
                          />
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Button
                                variant="ghost"
                                size="icon"
                                aria-label="添加图片"
                                onClick={() => imageInput.current?.click()}
                              >
                                <Plus />
                              </Button>
                            </TooltipTrigger>
                            <TooltipContent>
                              添加图片，也可以粘贴。不会发送。
                            </TooltipContent>
                          </Tooltip>
                          <Button
                            variant="ghost"
                            className="agent-select"
                            disabled
                            aria-label="选择 Agent"
                          >
                            <Bot />
                            {selected
                              ? selected.settings.agentID || "未设置"
                              : "未加载"}
                          </Button>
                        </div>
                        <div className="composer-right">
                          <span className="usage" aria-label="上下文用量未接入">
                            —
                          </span>
                          <ModelMenu
                            models={models}
                            value={modelSelection}
                            disabled={
                              !connected ||
                              !selected ||
                              !synchronized ||
                              !!currentRun ||
                              busySending
                            }
                            error={modelError}
                            onRetry={() => {
                              if (clientRef.current?.connected)
                                void loadModels(clientRef.current);
                            }}
                            onChange={(value) => {
                              if (selectedID)
                                setModelSelections((current) => ({
                                  ...current,
                                  [selectedID]: value,
                                }));
                            }}
                          />
                          {currentRun && (
                            <Button
                              variant="outline"
                              size="icon"
                              aria-label={
                                stoppingCurrent ? "停止中" : "停止任务"
                              }
                              disabled={!synchronized || stoppingCurrent}
                              onClick={() => void stopRun()}
                            >
                              <Square className="stop-icon" />
                            </Button>
                          )}
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>
                                <Button
                                  size="icon"
                                  className="send-button"
                                  aria-label={
                                    currentRun ? "调整当前任务" : "发送消息"
                                  }
                                  disabled={!canSend}
                                  onClick={() => void sendMessage()}
                                >
                                  <ArrowUp />
                                </Button>
                              </span>
                            </TooltipTrigger>
                            <TooltipContent>
                              {currentRun
                                ? "直接插话，不排队"
                                : validModel
                                  ? "发送消息"
                                  : "请先选择模型和思考档位"}
                            </TooltipContent>
                          </Tooltip>
                        </div>
                      </div>
                    </div>
                    <div className="composer-caption">
                      <span>
                        {stoppingCurrent
                          ? "停止中，等待后台收尾…"
                          : busySending
                            ? "等待后台确认…"
                            : currentRun
                              ? "Enter 调整当前任务 · 不排队"
                              : "Enter 发送 · Shift + Enter 换行"}
                      </span>
                      <span>
                        {modelError ||
                          (models?.length === 0
                            ? "尚未配置模型"
                            : "图片、用量与完整工具展示尚未接入")}
                      </span>
                    </div>
                  </div>
                </div>
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
