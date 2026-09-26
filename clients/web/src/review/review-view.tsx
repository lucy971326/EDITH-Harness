import { useEffect, useRef, useState } from "react";
import { DiffEditor } from "@monaco-editor/react";
import { Button } from "@/components/ui/button";
import { ResizeHandle } from "@/components/resize-handle";
import type { RPCClient } from "../client/rpc";
import { Columns2, FileText, PanelRight, Rows3, Undo2 } from "../icons";
import {
  defineEditorThemes,
  editorFontFamily,
  editorFontSize,
  editorLanguage,
  useEditorTheme,
} from "../editor/monaco";
import type {
  FileDiffSummary,
  RunDiffFile,
  RunDiffSummary,
} from "../../../contracts/run.ts";

function storedTreeWidth(): number {
  try {
    return Math.max(
      180,
      Math.min(
        520,
        Number(localStorage.getItem("harness-web:review-tree-width")) || 280,
      ),
    );
  } catch {
    return 280;
  }
}

function displayPath(workspace: string | null, path: string): string {
  if (!workspace) return path;
  const prefix = workspace.replace(/[\\/]$/, "");
  if (!path.toLowerCase().startsWith(prefix.toLowerCase())) return path;
  const remainder = path.slice(prefix.length);
  if (!/^[\\/]/.test(remainder)) return path;
  const relative = remainder.replace(/^[\\/]/, "");
  return relative || path;
}

function operationLabel(operation: FileDiffSummary["operation"]): string {
  return { add: "新增", update: "修改", delete: "删除" }[operation];
}

export function ReviewView({
  sessionID,
  workspace,
  initialSummary,
  client,
  runActive,
  readDiff,
  revertDiff,
}: {
  sessionID: string;
  workspace: string | null;
  initialSummary: RunDiffSummary;
  client: RPCClient | null;
  runActive: boolean;
  readDiff?: (runID: string, path: string) => Promise<RunDiffFile>;
  revertDiff?: (
    runID: string,
    path: string,
    expectedRevision: number,
  ) => Promise<RunDiffSummary>;
}) {
  const [summary, setSummary] = useState(initialSummary);
  const [selectedPath, setSelectedPath] = useState(
    initialSummary.files[0]?.path ?? "",
  );
  const [file, setFile] = useState<RunDiffFile | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [reverting, setReverting] = useState(false);
  const [treeWidth, setTreeWidth] = useState(storedTreeWidth);
  const [treeOpen, setTreeOpen] = useState(false);
  const [editorWidth, setEditorWidth] = useState(800);
  const [sideBySide, setSideBySide] = useState(true);
  const editorPane = useRef<HTMLElement>(null);
  const treePane = useRef<HTMLElement>(null);
  const readRequestID = useRef(0);
  const revertRequestID = useRef(0);
  const summaryRef = useRef(summary);
  const selectedRef = useRef(selectedPath);
  const theme = useEditorTheme();
  summaryRef.current = summary;
  selectedRef.current = selectedPath;

  useEffect(() => {
    if (initialSummary.revision < summaryRef.current.revision) return;
    if (initialSummary.revision > summaryRef.current.revision) {
      revertRequestID.current++;
      setReverting(false);
    }
    setSummary(initialSummary);
    if (!initialSummary.files.some((item) => item.path === selectedRef.current))
      setSelectedPath(initialSummary.files[0]?.path ?? "");
  }, [initialSummary]);

  useEffect(() => {
    try {
      localStorage.setItem("harness-web:review-tree-width", String(treeWidth));
    } catch {
      /* 本机偏好不可写时仅保留本页状态。 */
    }
  }, [treeWidth]);

  useEffect(() => {
    const element = editorPane.current;
    if (!element) return;
    const observer = new ResizeObserver((entries) => {
      setEditorWidth(entries[0]?.contentRect.width ?? element.clientWidth);
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    const generation = ++readRequestID.current;
    setFile(null);
    setError("");
    if (!selectedPath) return;
    if (!client?.connected) {
      setError("后台未连接");
      return;
    }
    const revision = summary.revision;
    setLoading(true);
    const read =
      readDiff ??
      ((runID: string, path: string) =>
        client.readRunDiff(sessionID, runID, path));
    void read(summary.runID, selectedPath)
      .then((result) => {
        if (generation !== readRequestID.current) return;
        if (
          result.runID !== summary.runID ||
          result.path !== selectedPath ||
          result.revision !== revision
        ) {
          setError("Diff 已更新，正在等待同步");
          return;
        }
        setFile(result);
      })
      .catch((cause) => {
        if (generation !== readRequestID.current) return;
        setError(cause instanceof Error ? cause.message : "读取 Diff 失败");
      })
      .finally(() => {
        if (generation === readRequestID.current) setLoading(false);
      });
    return () => {
      readRequestID.current++;
    };
  }, [
    client,
    selectedPath,
    sessionID,
    summary.revision,
    summary.runID,
    readDiff,
  ]);

  async function revertSelected() {
    const path = selectedRef.current;
    const current = summaryRef.current;
    if (!path || !client?.connected || runActive || reverting) return;
    const index = current.files.findIndex((item) => item.path === path);
    if (index < 0) return;

    const generation = ++revertRequestID.current;
    readRequestID.current++;
    setReverting(true);
    setError("");
    try {
      const revert =
        revertDiff ??
        ((runID: string, target: string, revision: number) =>
          client.revertRunDiff(sessionID, runID, target, revision));
      const result = await revert(current.runID, path, current.revision);
      if (generation !== revertRequestID.current) return;
      if (
        summaryRef.current.runID !== current.runID ||
        summaryRef.current.revision !== current.revision
      )
        return;
      setSummary(result);
      setFile(null);
      const currentSelection = selectedRef.current;
      setSelectedPath(
        result.files.find((item) => item.path === currentSelection)?.path ??
          result.files[Math.min(index, result.files.length - 1)]?.path ??
          "",
      );
    } catch (cause) {
      if (generation !== revertRequestID.current) return;
      setError(cause instanceof Error ? cause.message : "撤销失败");
    } finally {
      if (generation === revertRequestID.current) setReverting(false);
    }
  }

  const renderSideBySide = editorWidth >= 720 && sideBySide;
  const selected = summary.files.find((item) => item.path === selectedPath);

  return (
    <div className="review-view">
      <div className="review-toolbar">
        <span title={selectedPath}>
          {selected ? displayPath(workspace, selected.path) : "审查更改"}
        </span>
        <div className="review-toolbar-actions">
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={renderSideBySide ? "使用单栏 Diff" : "使用双栏 Diff"}
            disabled={editorWidth < 720}
            onClick={() => setSideBySide(!sideBySide)}
          >
            {renderSideBySide ? <Rows3 /> : <Columns2 />}
          </Button>
          <Button
            variant="ghost"
            size="xs"
            disabled={!selected || runActive || reverting || !client?.connected}
            title={runActive ? "运行结束后才能撤销" : undefined}
            onClick={() => void revertSelected()}
          >
            <Undo2 />
            {reverting ? "撤销中…" : "撤销此文件"}
          </Button>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={treeOpen ? "收起变更文件列表" : "展开变更文件列表"}
            aria-expanded={treeOpen}
            onClick={() => setTreeOpen(!treeOpen)}
          >
            <PanelRight />
          </Button>
        </div>
      </div>

      <div className="review-workspace">
        <section
          ref={editorPane}
          className="review-editor-pane"
          aria-label="Diff 审查"
        >
          {summary.files.length === 0 ? (
            <div className="workspace-chooser">
              <FileText />
              <strong>本轮更改已全部撤销</strong>
            </div>
          ) : error ? (
            <div className="workspace-chooser" role="alert">
              <FileText />
              <strong>无法显示更改</strong>
              <span>{error}</span>
            </div>
          ) : loading || !file ? (
            <div className="editor-loading">正在读取 Diff…</div>
          ) : (
            <DiffEditor
              key={`${file.runID}:${file.path}:${file.revision}`}
              className="monaco-host"
              original={file.oldContent ?? ""}
              modified={file.newContent ?? ""}
              originalLanguage={editorLanguage(file.path)}
              modifiedLanguage={editorLanguage(file.path)}
              theme={theme}
              beforeMount={defineEditorThemes}
              loading={<div className="editor-loading">正在加载 Diff…</div>}
              options={{
                automaticLayout: true,
                contextmenu: true,
                fontFamily: editorFontFamily,
                fontSize: editorFontSize,
                lineHeight: 21,
                minimap: { enabled: false },
                readOnly: true,
                renderSideBySide,
                scrollBeyondLastLine: false,
                smoothScrolling: true,
              }}
            />
          )}
        </section>

        {treeOpen && (
          <aside
            ref={treePane}
            className="review-tree-pane"
            style={{ width: treeWidth }}
          >
            <ResizeHandle
              label="调整变更文件列表宽度"
              value={treeWidth}
              min={180}
              max={520}
              growToward="left"
              onResize={(width) => {
                if (treePane.current)
                  treePane.current.style.width = `${width}px`;
              }}
              onChange={setTreeWidth}
            />
            <div
              className="review-file-list"
              role="listbox"
              aria-label="本轮变更文件"
            >
              {summary.files.map((item) => (
                <button
                  key={item.path}
                  role="option"
                  aria-selected={item.path === selectedPath}
                  title={`${item.path} · ${operationLabel(item.operation)}`}
                  aria-label={`${displayPath(workspace, item.path)}，${operationLabel(item.operation)}，新增 ${item.additions} 行，删除 ${item.deletions} 行`}
                  onClick={() => setSelectedPath(item.path)}
                >
                  <FileText />
                  <span>
                    <strong>{displayPath(workspace, item.path)}</strong>
                  </span>
                  <i className="diff-additions">+{item.additions}</i>
                  <i className="diff-deletions">-{item.deletions}</i>
                </button>
              ))}
            </div>
          </aside>
        )}
      </div>
    </div>
  );
}
