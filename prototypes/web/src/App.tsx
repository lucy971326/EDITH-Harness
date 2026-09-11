import { useEffect, useRef, useState, type CSSProperties } from "react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
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
  ChevronDown,
  ChevronRight,
  ArrowUp,
  ArrowDown,
  X,
  Settings,
  Square,
  Check,
  Bot,
  WifiOff,
  RefreshCw,
  FlaskConical,
  Circle,
  BookOpen,
  Command,
} from "./icons";
import {
  demoTurn,
  firstSession,
  initialAgents,
  models,
  newSession,
  sampleActivity,
  sampleAnswer,
  type Session,
  type RunStatus,
  type Attachment,
} from "./demo";
import { WorkProcess } from "./work-process";
import { SettingsPage } from "./settings-page";
import { WorkspaceTabs } from "./workspace-tabs";
import { registerDemoTool } from "./demo-tool";

function preference(key: string, fallback: string) {
  try {
    return localStorage.getItem(`harness-prototype:${key}`) ?? fallback;
  } catch {
    return fallback;
  }
}
function savePreference(key: string, value: string) {
  try {
    localStorage.setItem(`harness-prototype:${key}`, value);
  } catch {
    /* 私密模式不可写时，仍可在本次页面使用。 */
  }
}

export default function App() {
  const [sessions, setSessions] = useState<Session[]>([
    firstSession,
    { ...newSession("Harness"), title: "梳理 App-server 的边界" },
    { ...newSession("Playground"), title: "一个新的想法" },
  ]);
  const [activeId, setActiveId] = useState(firstSession.id);
  const [agents, setAgents] = useState(initialAgents);
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
  const [connected, setConnected] = useState(true);
  const [disconnectedSnapshot, setDisconnectedSnapshot] = useState<Session[]>(
    [],
  );
  const [notice, setNotice] = useState("");
  const [projectDialog, setProjectDialog] = useState(false);
  const [projectName, setProjectName] = useState("");
  const [modelMenu, setModelMenu] = useState(false);
  const [following, setFollowing] = useState(true);
  const scrollArea = useRef<HTMLDivElement>(null);
  const imageInput = useRef<HTMLInputElement>(null);
  const input = useRef<HTMLTextAreaElement>(null);
  const objectUrls = useRef<string[]>([]);
  const session = sessions.find((item) => item.id === activeId)!;
  const visibleTurns = connected
    ? session.turns
    : (disconnectedSnapshot.find((item) => item.id === activeId)?.turns ?? []);
  const currentRun = session.turns.find(
    (turn) => turn.status === "running" || turn.status === "stopping",
  );
  const model = models.find((item) => item.id === session.model)!;
  const projects = [...new Set(sessions.map((item) => item.project))];

  function updateSession(patch: Partial<Session>) {
    setSessions((items) =>
      items.map((item) =>
        item.id === activeId ? { ...item, ...patch } : item,
      ),
    );
  }
  function selectSession(id: string) {
    setActiveId(id);
    setSettings(false);
    setFollowing(true);
    if (window.innerWidth < 760) setSidebar(false);
  }
  function createSession(project: string) {
    const empty = sessions.find(
      (item) =>
        item.project === project &&
        !item.turns.length &&
        !item.draft &&
        !item.images.length,
    );
    if (empty) {
      selectSession(empty.id);
      return;
    }
    const created = newSession(project);
    setSessions((items) => [created, ...items]);
    selectSession(created.id);
  }
  function send() {
    if (
      !connected ||
      currentRun?.status === "stopping" ||
      (!session.draft.trim() && !session.images.length)
    )
      return;
    if (session.images.length && !model.vision) {
      setNotice("当前模型不支持图片，请切换模型或移除附件。");
      return;
    }
    if (currentRun && session.images.length) {
      setNotice("原型中的插话只演示文字，请先移除图片。");
      return;
    }
    const prompt = session.draft.trim();
    setSessions((items) =>
      items.map((item) => {
        if (item.id !== activeId) return item;
        const turns = currentRun
          ? item.turns.map((turn) =>
              turn.id === currentRun.id
                ? {
                    ...turn,
                    activity: [
                      ...turn.activity,
                      {
                        id: crypto.randomUUID(),
                        kind: "steer" as const,
                        text: prompt,
                      },
                    ],
                  }
                : turn,
            )
          : [
              ...item.turns,
              {
                id: crypto.randomUUID(),
                prompt: prompt || "请看看这张图片",
                images: item.images,
                status: "running" as const,
                tick: 0,
                activity: [],
                answer: "",
              },
            ];
        return {
          ...item,
          title: item.turns.length
            ? item.title
            : (prompt || "图片对话").slice(0, 22),
          draft: "",
          images: [],
          turns,
        };
      }),
    );
    setFollowing(true);
    setNotice(currentRun ? "已调整当前任务方向，没有加入等待队列。" : "");
  }
  function stop() {
    if (!connected) return;
    updateSession({
      turns: session.turns.map((turn) =>
        turn.id === currentRun?.id ? { ...turn, status: "stopping" } : turn,
      ),
    });
  }
  function disconnect() {
    setDisconnectedSnapshot(sessions);
    setConnected(false);
  }
  function scenario(status: RunStatus | "empty") {
    // 演示切换只改变当前示例；不会发起网络请求。
    updateSession({
      turns: status === "empty" ? [] : [demoTurn(status)],
      title: status === "empty" ? "新对话" : session.title,
    });
    setFollowing(true);
  }
  function addImages(files: FileList | File[] | null) {
    if (!files) return;
    if (!model.vision) {
      setNotice("文字模型不支持图片，请先切换到视觉模型。");
      return;
    }
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
    updateSession({ images: [...session.images, ...attachments].slice(0, 4) });
  }
  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text);
      setNotice("已复制。");
    } catch {
      setNotice("浏览器未允许复制，请选中文字手动复制。");
    }
  }
  function fork(index: number) {
    const created = {
      ...newSession(session.project),
      title: `${session.title} · 分叉`,
      agent: session.agent,
      model: session.model,
      effort: session.effort,
      turns: session.turns
        .slice(0, index + 1)
        .map((turn) => ({ ...turn, id: crypto.randomUUID() })),
    };
    setSessions((items) => [created, ...items]);
    selectSession(created.id);
    setNotice("已从这个回答创建独立的示例会话。");
  }

  useEffect(() => {
    const timer = window.setInterval(
      () =>
        setSessions((items) =>
          items.map((item) => ({
            ...item,
            turns: item.turns.map((turn) => {
              if (turn.status === "stopping")
                return { ...turn, status: "stopped" as const };
              if (turn.status !== "running") return turn;
              const tick = turn.tick + 1;
              const next = sampleActivity[tick - 1];
              if (tick >= 12) {
                const steers = turn.activity.filter(
                  (event) => event.kind === "steer",
                );
                return {
                  ...turn,
                  tick,
                  status: "completed" as const,
                  answer: steers.length
                    ? `已按你的调整继续处理：“${steers.at(-1)!.text}”。\n\n${sampleAnswer}`
                    : sampleAnswer,
                };
              }
              return {
                ...turn,
                tick,
                activity: next
                  ? [...turn.activity, { ...next, id: `${turn.id}-${next.id}` }]
                  : turn.activity,
              };
            }),
          })),
        ),
      2000,
    );
    return () => clearInterval(timer);
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
    savePreference("panel-width", String(panelWidth));
  }, [panel, panelWidth]);
  useEffect(() => {
    if (following && scrollArea.current)
      scrollArea.current.scrollTop = scrollArea.current.scrollHeight;
  }, [session.turns, activeId, following, settings]);
  useEffect(() => {
    if (!notice) return;
    const timer = setTimeout(() => setNotice(""), 5500);
    return () => clearTimeout(timer);
  }, [notice]);
  useEffect(() => {
    if (connected) return;
    const timer = setTimeout(() => {
      setConnected(true);
      setNotice("已重新连接并同步示例状态；没有重发消息。");
    }, 7000);
    return () => clearTimeout(timer);
  }, [connected]);
  useEffect(
    () => () => {
      objectUrls.current.forEach((url) => URL.revokeObjectURL(url));
    },
    [],
  );
  useEffect(
    () =>
      registerDemoTool((state) => {
        if (!connected) throw new Error("请等待演示连接恢复");
        setSettings(false);
        setFollowing(true);
        setSessions((items) =>
          items.map((item) =>
            item.id === activeId
              ? { ...item, turns: state === "empty" ? [] : [demoTurn(state)] }
              : item,
          ),
        );
      }),
    [activeId, connected],
  );

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
              <Button
                variant="ghost"
                className="open-project"
                onClick={() => setProjectDialog(true)}
              >
                <FolderOpen />
                打开项目
                <Plus className="ml-auto" />
              </Button>
              <div className="sidebar-heading">项目</div>
              <nav className="project-list" aria-label="项目与会话">
                {projects.map((project) => (
                  <Collapsible
                    defaultOpen
                    key={project}
                    className="project-group"
                  >
                    <div className="project-heading">
                      <CollapsibleTrigger className="project-trigger">
                        <ChevronRight className="disclosure-chevron" />
                        <Folder />
                        <span>{project}</span>
                      </CollapsibleTrigger>
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={`在 ${project} 新建会话`}
                        onClick={() => createSession(project)}
                      >
                        <Plus />
                      </Button>
                    </div>
                    <CollapsibleContent>
                      {sessions
                        .filter((item) => item.project === project)
                        .map((item) => (
                          <button
                            key={item.id}
                            className={`session-row ${item.id === activeId && !settings ? "selected" : ""}`}
                            onClick={() => selectSession(item.id)}
                          >
                            <span>{item.title}</span>
                            {item.turns.some(
                              (turn) => turn.status === "running",
                            ) && (
                              <span
                                aria-label="运行中"
                                className="running-dot"
                              />
                            )}
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
                  onClick={() => setSettings(true)}
                >
                  <Settings />
                  设置
                </Button>
                <div className="local-caption">
                  <Circle />
                  本机工作空间<span>原型</span>
                </div>
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
              <span>{settings ? "设置" : session.project}</span>
              {!settings && (
                <>
                  <span className="breadcrumb-divider">/</span>
                  <span className="title-truncate">{session.title}</span>
                </>
              )}
            </div>
            <div className="topbar-actions">
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="demo-control"
                    aria-label="原型演示"
                  >
                    <FlaskConical />
                    <span>原型演示</span>
                    <ChevronDown />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuLabel>仅切换示例数据</DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  {(
                    [
                      ["empty", "空会话"],
                      ["running", "运行中"],
                      ["completed", "已完成"],
                      ["failed", "失败"],
                      ["stopped", "已停止"],
                    ] as const
                  ).map(([value, label]) => (
                    <DropdownMenuItem
                      key={value}
                      disabled={!connected}
                      onSelect={() => {
                        setSettings(false);
                        scenario(value);
                      }}
                    >
                      {label}
                    </DropdownMenuItem>
                  ))}
                  <DropdownMenuSeparator />
                  <DropdownMenuItem disabled={!connected} onSelect={disconnect}>
                    <WifiOff />
                    模拟断线与自动重连
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
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
                agents={agents}
                setAgents={setAgents}
                usedAgentIds={sessions.map((item) => item.agent)}
                theme={theme}
                setTheme={setTheme}
                onBack={() => setSettings(false)}
              />
            ) : (
              <section className="chat" aria-label="聊天">
                {!connected && (
                  <div className="connection-banner" role="status">
                    <WifiOff />
                    <span>
                      连接已断开，正在重连。后台任务继续，草稿已保留。
                    </span>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setConnected(true)}
                    >
                      <RefreshCw />
                      立即重连
                    </Button>
                  </div>
                )}
                <div
                  className="messages"
                  ref={scrollArea}
                  onWheel={(event) => {
                    if (event.deltaY < 0) setFollowing(false);
                  }}
                  onTouchMove={() => setFollowing(false)}
                  onScroll={(event) => {
                    const node = event.currentTarget;
                    setFollowing(
                      node.scrollHeight - node.scrollTop - node.clientHeight <
                        16,
                    );
                  }}
                >
                  <div className="message-column">
                    {visibleTurns.length === 0 ? (
                      <div className="empty-chat">
                        <div className="empty-symbol">
                          <Command />
                        </div>
                        <h1>从一个想法开始。</h1>
                        <p>在 {session.project} 中，你想一起完成什么？</p>
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
                                updateSession({ draft: text });
                                input.current?.focus();
                              }}
                            >
                              {text}
                            </Button>
                          ))}
                        </div>
                      </div>
                    ) : (
                      visibleTurns.map((turn, index) => (
                        <WorkProcess
                          key={turn.id}
                          turn={turn}
                          onInspect={() => setFollowing(false)}
                          onCopy={copy}
                          onFork={() => {
                            if (connected) fork(index);
                            else setNotice("重连完成后才能分叉。");
                          }}
                        />
                      ))
                    )}
                  </div>
                </div>
                <div className="composer-area">
                  <div className="composer-column">
                    {!following && (
                      <Button
                        variant="outline"
                        size="sm"
                        className="back-latest"
                        onClick={() => setFollowing(true)}
                      >
                        <ArrowDown />
                        回到最新
                      </Button>
                    )}
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
                      {(session.draft.startsWith("/") ||
                        session.draft.startsWith("$")) && (
                        <div className="suggestions">
                          {(session.draft.startsWith("/")
                            ? ["compact 压缩上下文", "help 查看命令"]
                            : [
                                "ablation-review 消融审查",
                                "skill-creator 创建技能",
                              ]
                          ).map((text) => (
                            <button
                              key={text}
                              onClick={() => {
                                updateSession({
                                  draft: `${session.draft[0]}${text.split(" ")[0]} `,
                                });
                                input.current?.focus();
                              }}
                            >
                              <BookOpen />
                              <span>{text}</span>
                            </button>
                          ))}
                        </div>
                      )}
                      {session.images.length > 0 && (
                        <div className="attachments">
                          {session.images.map((image) => (
                            <div key={image.id}>
                              <img src={image.url} alt={image.name} />
                              <button
                                aria-label={`移除图片 ${image.name}`}
                                onClick={() =>
                                  updateSession({
                                    images: session.images.filter(
                                      (item) => item.id !== image.id,
                                    ),
                                  })
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
                          currentRun
                            ? "发送以调整当前任务…"
                            : "说说你的想法，或输入 / 查看命令"
                        }
                        value={session.draft}
                        onChange={(event) =>
                          updateSession({ draft: event.target.value })
                        }
                        onKeyDown={(event) => {
                          if (
                            event.key === "Enter" &&
                            !event.shiftKey &&
                            !event.nativeEvent.isComposing &&
                            event.keyCode !== 229
                          ) {
                            event.preventDefault();
                            send();
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
                                disabled={!model.vision}
                                onClick={() => imageInput.current?.click()}
                              >
                                <Plus />
                              </Button>
                            </TooltipTrigger>
                            <TooltipContent>
                              {model.vision
                                ? "添加图片，也可以粘贴"
                                : "当前模型不支持图片"}
                            </TooltipContent>
                          </Tooltip>
                          <Select
                            value={session.agent}
                            onValueChange={(agent) => updateSession({ agent })}
                          >
                            <SelectTrigger
                              className="agent-select"
                              aria-label="选择 Agent"
                            >
                              <Bot />
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              {agents.map((agent) => (
                                <SelectItem key={agent.id} value={agent.id}>
                                  {agent.name}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </div>
                        <div className="composer-right">
                          <Popover>
                            <PopoverTrigger asChild>
                              <button
                                className="usage"
                                aria-label="上下文用量 36%"
                              >
                                <span className="usage-ring" />
                                <span>36%</span>
                              </button>
                            </PopoverTrigger>
                            <PopoverContent className="usage-popover">
                              <h3>上下文用量</h3>
                              <p>36,000 / 100,000 tokens</p>
                              <p className="muted">
                                36% · 原型示例值，不代表真实模型窗口。
                              </p>
                            </PopoverContent>
                          </Popover>
                          <Popover open={modelMenu} onOpenChange={setModelMenu}>
                            <PopoverTrigger asChild>
                              <Button
                                variant="ghost"
                                size="sm"
                                className="model-trigger"
                              >
                                {model.name}
                                <span className="muted">
                                  · {session.effort}
                                </span>
                                <ChevronDown />
                              </Button>
                            </PopoverTrigger>
                            <PopoverContent
                              align="end"
                              className="model-popover"
                            >
                              <h3>模型与思考</h3>
                              <p className="metadata">
                                {currentRun
                                  ? "更改将在下一轮运行生效"
                                  : "用于下一次发送"}
                              </p>
                              <div className="model-options">
                                {models.map((item) => (
                                  <button
                                    key={item.id}
                                    className={
                                      item.id === model.id ? "selected" : ""
                                    }
                                    onClick={() =>
                                      updateSession({
                                        model: item.id,
                                        effort: item.efforts.includes(
                                          session.effort,
                                        )
                                          ? session.effort
                                          : item.efforts[0],
                                      })
                                    }
                                  >
                                    <div>
                                      <strong>{item.name}</strong>
                                      <span className="metadata">
                                        {item.description}
                                      </span>
                                    </div>
                                    {item.id === model.id && <Check />}
                                  </button>
                                ))}
                              </div>
                              <Label>思考强度</Label>
                              <div className="effort-options">
                                {model.efforts.map((effort) => (
                                  <Button
                                    key={effort}
                                    size="sm"
                                    variant={
                                      session.effort === effort
                                        ? "default"
                                        : "outline"
                                    }
                                    onClick={() => updateSession({ effort })}
                                  >
                                    {effort}
                                  </Button>
                                ))}
                              </div>
                              <Button
                                className="w-full"
                                variant="secondary"
                                onClick={() => setModelMenu(false)}
                              >
                                完成
                              </Button>
                            </PopoverContent>
                          </Popover>
                          {currentRun && (
                            <Button
                              variant="outline"
                              size="icon"
                              aria-label="停止任务"
                              disabled={
                                !connected || currentRun.status === "stopping"
                              }
                              onClick={stop}
                            >
                              <Square className="stop-icon" />
                            </Button>
                          )}
                          <Button
                            size="icon"
                            className="send-button"
                            aria-label={
                              currentRun ? "调整当前任务" : "发送消息"
                            }
                            disabled={
                              !connected ||
                              currentRun?.status === "stopping" ||
                              (!session.draft.trim() && !session.images.length)
                            }
                            onClick={send}
                          >
                            <ArrowUp />
                          </Button>
                        </div>
                      </div>
                    </div>
                    <div className="composer-caption">
                      <span>
                        {currentRun
                          ? "运行中发送直接调整方向，不排队"
                          : "Enter 发送 · Shift + Enter 换行"}
                      </span>
                      <span>交互原型 · 不调用真实模型</span>
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
        <Dialog open={projectDialog} onOpenChange={setProjectDialog}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>打开项目</DialogTitle>
              <DialogDescription>
                原型只添加一个项目分组，不读取本机文件。正式版会打开系统目录选择器。
              </DialogDescription>
            </DialogHeader>
            <Label htmlFor="project-name">项目名称</Label>
            <Input
              id="project-name"
              value={projectName}
              onChange={(event) => setProjectName(event.target.value)}
              placeholder="例如 My Project"
              onKeyDown={(event) => {
                if (event.key === "Enter" && projectName.trim()) {
                  createSession(projectName.trim());
                  setProjectDialog(false);
                  setProjectName("");
                }
              }}
            />
            <DialogFooter>
              <Button variant="outline" onClick={() => setProjectDialog(false)}>
                取消
              </Button>
              <Button
                disabled={!projectName.trim()}
                onClick={() => {
                  createSession(projectName.trim());
                  setProjectDialog(false);
                  setProjectName("");
                }}
              >
                <FolderOpen />
                打开示例项目
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </TooltipProvider>
  );
}
