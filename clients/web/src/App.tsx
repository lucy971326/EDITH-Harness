import { useEffect, useRef, useState, type CSSProperties } from "react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
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
  ArrowUp,
  X,
  Settings,
  Bot,
  Circle,
  Command,
} from "./icons";
import { SettingsPage } from "./settings-page";
import { WorkspaceTabs } from "./workspace-tabs";

type Attachment = { id: string; name: string; url: string };

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
  const imageInput = useRef<HTMLInputElement>(null);
  const input = useRef<HTMLTextAreaElement>(null);
  const objectUrls = useRef<string[]>([]);

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
    if (!notice) return;
    const timer = setTimeout(() => setNotice(""), 5500);
    return () => clearTimeout(timer);
  }, [notice]);
  useEffect(
    () => () => {
      objectUrls.current.forEach((url) => URL.revokeObjectURL(url));
    },
    [],
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
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="open-project-wrap">
                    <Button variant="ghost" className="open-project" disabled>
                      <FolderOpen />
                      打开项目
                      <Plus className="ml-auto" />
                    </Button>
                  </span>
                </TooltipTrigger>
                <TooltipContent>后台尚未接入</TooltipContent>
              </Tooltip>
              <div className="sidebar-heading">项目</div>
              <nav className="project-list" aria-label="项目与会话">
                <p className="metadata sidebar-empty">
                  还没有项目。后台尚未接入。
                </p>
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
                  本机工作空间<span>未接入</span>
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
              <span>{settings ? "设置" : "聊天"}</span>
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
                <div className="messages">
                  <div className="message-column">
                    <div className="empty-chat">
                      <div className="empty-symbol">
                        <Command />
                      </div>
                      <h1>从一个想法开始。</h1>
                      <p>后台尚未接入。输入会留在本页，不会发送。</p>
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
                              setDraft(text);
                              input.current?.focus();
                            }}
                          >
                            {text}
                          </Button>
                        ))}
                      </div>
                    </div>
                  </div>
                </div>
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
                                    images.filter((item) => item.id !== image.id),
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
                        placeholder="说说你的想法"
                        value={draft}
                        onChange={(event) => setDraft(event.target.value)}
                        onKeyDown={(event) => {
                          if (
                            event.key === "Enter" &&
                            !event.shiftKey &&
                            !event.nativeEvent.isComposing &&
                            event.keyCode !== 229
                          ) {
                            event.preventDefault();
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
                            未加载
                          </Button>
                        </div>
                        <div className="composer-right">
                          <span className="usage" aria-label="上下文用量未接入">
                            —
                          </span>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="model-trigger"
                            disabled
                          >
                            未加载
                            <span className="muted">· 思考</span>
                            <ChevronDown />
                          </Button>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span>
                                <Button
                                  size="icon"
                                  className="send-button"
                                  aria-label="发送消息"
                                  disabled
                                >
                                  <ArrowUp />
                                </Button>
                              </span>
                            </TooltipTrigger>
                            <TooltipContent>后台尚未接入</TooltipContent>
                          </Tooltip>
                        </div>
                      </div>
                    </div>
                    <div className="composer-caption">
                      <span>后台尚未接入，Enter 暂不发送</span>
                      <span>发送、Steer、停止和分叉将在后续步骤接入</span>
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
