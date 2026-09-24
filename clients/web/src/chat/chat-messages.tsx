import {
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Button } from "@/components/ui/button";
import { ArrowDown, Check } from "../icons";
import { ContextMenu, FloatingSurface } from "../components/context-menu";
import type { ContextReference, ReferenceAttachment } from "./context-references";
import { chatTurns } from "../state/chat-process";
import { WorkProcess } from "./work-process";
import type { Snapshot } from "../../../contracts/run.ts";
import type { RunDiffSummary } from "../../../contracts/run.ts";
import type { FileLocation } from "../editor/links";

interface AssistantSelection {
  reference: Extract<ContextReference, { kind: "assistant-selection" }>;
  anchor: { left: number; top: number; right: number; bottom: number };
}

interface AssistantMarker {
  id: string;
  number: number;
  left: number;
  top: number;
}

const noPendingReferences: ReferenceAttachment[] = [];

function textOffset(root: HTMLElement, node: Node, offset: number): number {
  const range = document.createRange();
  range.selectNodeContents(root);
  range.setEnd(node, offset);
  return range.cloneContents().textContent?.length ?? 0;
}

function textPoint(root: HTMLElement, offset: number): { node: Text; offset: number } | null {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  let consumed = 0;
  let last: Text | null = null;
  for (let node = walker.nextNode() as Text | null; node; node = walker.nextNode() as Text | null) {
    last = node;
    const length = node.data.length;
    if (offset <= consumed + length) return { node, offset: offset - consumed };
    consumed += length;
  }
  return last && offset === consumed ? { node: last, offset: last.data.length } : null;
}

function readAssistantMarkers(
  container: HTMLElement,
  references: ReferenceAttachment[],
): AssistantMarker[] {
  const answers = Array.from(container.querySelectorAll<HTMLElement>("[data-assistant-entry-id]"));
  let number = 0;
  const markers: AssistantMarker[] = [];
  for (const item of references) {
    const reference = item.reference;
    if (reference.kind !== "assistant-selection") continue;
    number++;
    const answer = answers.find((element) => element.dataset.assistantEntryId === reference.entryID);
    if (!answer) continue;
    const start = textPoint(answer, reference.startOffset);
    const end = textPoint(answer, reference.endOffset);
    if (!start || !end) continue;
    const range = document.createRange();
    range.setStart(start.node, start.offset);
    range.setEnd(end.node, end.offset);
    if (range.toString() !== reference.content) continue;
    const rects = range.getClientRects();
    const rect = rects[rects.length - 1] ?? range.getBoundingClientRect();
    if ((!rect.width && !rect.height) || rect.bottom < 0 || rect.top > window.innerHeight) continue;
    markers.push({
      id: item.id,
      number,
      left: Math.min(rect.right + 8, window.innerWidth - 36),
      top: rect.top,
    });
  }
  return markers;
}

function AssistantCommentEditor({
  selection,
  number,
  onSubmit,
  onClose,
}: {
  selection: AssistantSelection;
  number: number;
  onSubmit: (reference: Extract<ContextReference, { kind: "assistant-selection" }>) => void;
  onClose: () => void;
}) {
  const input = useRef<HTMLInputElement>(null);
  const [comment, setComment] = useState("");
  useLayoutEffect(() => input.current?.focus(), []);

  const submit = () => {
    const value = comment.trim();
    onSubmit(value ? { ...selection.reference, comment: value } : selection.reference);
  };
  return (
    <>
      <FloatingSurface
        anchor={selection.anchor}
        className="assistant-comment-editor"
        role="dialog"
        label="为选中内容添加评论"
        onClose={onClose}
      >
        <input
          ref={input}
          value={comment}
          placeholder="添加评论（可选）"
          aria-label="评论"
          onChange={(event) => setComment(event.target.value)}
          onKeyDown={(event) => {
            if (event.key !== "Enter" || event.nativeEvent.isComposing) return;
            event.preventDefault();
            submit();
          }}
        />
        <button type="button" aria-label="确认引用" onClick={submit}>
          <Check />
        </button>
      </FloatingSurface>
      <span
        className="assistant-comment-marker"
        aria-hidden="true"
        style={{ left: selection.anchor.right + 8, top: selection.anchor.top }}
      >
        {number}
      </span>
    </>
  );
}

function readAssistantSelection(container: HTMLElement): AssistantSelection | null {
  const selection = window.getSelection();
  if (!selection || selection.isCollapsed || selection.rangeCount !== 1) return null;
  const range = selection.getRangeAt(0);
  const startElement = range.startContainer instanceof Element
    ? range.startContainer : range.startContainer.parentElement;
  const endElement = range.endContainer instanceof Element
    ? range.endContainer : range.endContainer.parentElement;
  const answer = startElement?.closest<HTMLElement>("[data-assistant-entry-id]");
  if (!answer || answer !== endElement?.closest("[data-assistant-entry-id]") || !container.contains(answer))
    return null;
  const content = selection.toString();
  if (!content.trim()) return null;

  const rect = range.getClientRects()[0] ?? range.getBoundingClientRect();
  if (!rect.width && !rect.height) return null;

  return {
    reference: {
      kind: "assistant-selection",
      entryID: answer.dataset.assistantEntryId!,
      startOffset: textOffset(answer, range.startContainer, range.startOffset),
      endOffset: textOffset(answer, range.endContainer, range.endOffset),
      content,
    },
    anchor: { left: rect.left, top: rect.top, right: rect.right, bottom: rect.bottom },
  };
}

export function ChatMessages({
  snapshot,
  sessionID,
  stoppingRunID,
  forkingEntryID,
  forkDisabled = true,
  onFork,
  workspace,
  onOpenFile,
  onOpenDiff,
  onOpenSubagent,
  onAddReference,
  pendingReferences = noPendingReferences,
  children,
}: {
  snapshot: Snapshot | null;
  sessionID: string | null;
  stoppingRunID?: string;
  forkingEntryID?: string;
  forkDisabled?: boolean;
  onFork?: (runID: string, boundaryEntryID: string) => void;
  workspace?: string | null;
  onOpenFile?: (location: FileLocation) => void;
  onOpenDiff?: (runID: string, summary: RunDiffSummary) => void;
  onOpenSubagent?: (taskID: string) => void;
  onAddReference?: (reference: ContextReference) => void;
  pendingReferences?: ReferenceAttachment[];
  children?: ReactNode;
}) {
  const scroll = useRef<HTMLDivElement>(null);
  const follow = useRef(true);
  const userScroll = useRef(false);
  const [showLatest, setShowLatest] = useState(false);
  const [assistantSelection, setAssistantSelection] = useState<AssistantSelection | null>(null);
  const [commentSelection, setCommentSelection] = useState<AssistantSelection | null>(null);
  const [assistantMarkers, setAssistantMarkers] = useState<AssistantMarker[]>([]);
  const turns = useMemo(
    () => (snapshot ? chatTurns(snapshot) : []),
    [snapshot],
  );

  useLayoutEffect(() => {
    follow.current = true;
    userScroll.current = false;
    setShowLatest(false);
    setAssistantSelection(null);
    setCommentSelection(null);
  }, [sessionID]);
  useLayoutEffect(() => {
    if (follow.current && scroll.current)
      scroll.current.scrollTop = scroll.current.scrollHeight;
  }, [snapshot, sessionID]);
  useLayoutEffect(() => {
    if (scroll.current) setAssistantMarkers(readAssistantMarkers(scroll.current, pendingReferences));
  }, [pendingReferences, snapshot, sessionID]);
  useEffect(() => {
    const update = () => {
      if (scroll.current) setAssistantMarkers(readAssistantMarkers(scroll.current, pendingReferences));
    };
    window.addEventListener("resize", update);
    return () => window.removeEventListener("resize", update);
  }, [pendingReferences]);

  return (
    <div className="chat-history">
      <div
        className="messages"
        ref={scroll}
        onWheel={() => {
          userScroll.current = true;
        }}
        onTouchMove={() => {
          userScroll.current = true;
        }}
        onPointerDown={() => {
          userScroll.current = true;
        }}
        onPointerUp={(event) => {
          setAssistantSelection(readAssistantSelection(event.currentTarget));
        }}
        onKeyUp={(event) => {
          setAssistantSelection(readAssistantSelection(event.currentTarget));
        }}
        onKeyDown={(event) => {
          if (
            [
              "ArrowUp",
              "ArrowDown",
              "PageUp",
              "PageDown",
              "Home",
              "End",
              " ",
            ].includes(event.key)
          )
            userScroll.current = true;
        }}
        onScroll={(event) => {
          const element = event.currentTarget;
          const bottom =
            element.scrollHeight - element.clientHeight - element.scrollTop <
            48;
          if (!bottom) follow.current = false;
          // 展开/折叠造成的布局滚动，不能解除主动查看详情的暂停。
          else if (userScroll.current) follow.current = true;
          userScroll.current = false;
          setShowLatest(!follow.current);
          setAssistantMarkers(readAssistantMarkers(element, pendingReferences));
        }}
      >
        <div className="message-column">
          {turns.length === 0 && children}
          {turns.map((turn) => (
            <WorkProcess
              key={`${sessionID}:${turn.id}`}
              turn={turn}
              stopping={stoppingRunID === turn.id}
              forking={forkingEntryID === turn.prompt?.id}
              forkDisabled={forkDisabled}
              onFork={onFork}
              workspace={workspace}
              onOpenFile={onOpenFile}
              onOpenDiff={onOpenDiff}
              onOpenSubagent={onOpenSubagent}
              onInspect={() => {
                follow.current = false;
                userScroll.current = false;
                setShowLatest(true);
              }}
            />
          ))}
        </div>
      </div>
      {showLatest && (
        <Button
          variant="outline"
          size="sm"
          className="back-latest"
          onClick={() => {
            follow.current = true;
            userScroll.current = false;
            setShowLatest(false);
            if (scroll.current)
              scroll.current.scrollTop = scroll.current.scrollHeight;
          }}
        >
          <ArrowDown />
          回到最新
        </Button>
      )}
      {assistantSelection && onAddReference && (
        <ContextMenu
          anchor={assistantSelection.anchor}
          compact
          label="助手消息选区操作"
          items={[{
            label: "添加到对话",
            action: () => {
              setCommentSelection(assistantSelection);
            },
          }]}
          onClose={() => setAssistantSelection(null)}
        />
      )}
      {commentSelection && onAddReference && (
        <AssistantCommentEditor
          selection={commentSelection}
          number={pendingReferences.filter((item) => item.reference.kind === "assistant-selection").length + 1}
          onSubmit={(reference) => {
            onAddReference(reference);
            window.getSelection()?.removeAllRanges();
            setCommentSelection(null);
          }}
          onClose={() => setCommentSelection(null)}
        />
      )}
      {assistantMarkers.map((marker) => (
        <span
          key={marker.id}
          className="assistant-comment-marker"
          aria-label={`引用 ${marker.number}`}
          style={{ left: marker.left, top: marker.top }}
        >
          {marker.number}
        </span>
      ))}
    </div>
  );
}
