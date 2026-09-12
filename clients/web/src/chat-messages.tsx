import {
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Button } from "@/components/ui/button";
import { ArrowDown } from "./icons";
import { chatTurns } from "./state/chat-process";
import { WorkProcess } from "./work-process";
import type { Snapshot } from "../../contracts/run.ts";

export function ChatMessages({
  snapshot,
  sessionID,
  stoppingRunID,
  forkingEntryID,
  forkDisabled = true,
  onFork,
  children,
}: {
  snapshot: Snapshot | null;
  sessionID: string | null;
  stoppingRunID?: string;
  forkingEntryID?: string;
  forkDisabled?: boolean;
  onFork?: (runID: string, boundaryEntryID: string) => void;
  children?: ReactNode;
}) {
  const scroll = useRef<HTMLDivElement>(null);
  const follow = useRef(true);
  const userScroll = useRef(false);
  const [showLatest, setShowLatest] = useState(false);
  const turns = useMemo(
    () => (snapshot ? chatTurns(snapshot) : []),
    [snapshot],
  );

  useLayoutEffect(() => {
    follow.current = true;
    userScroll.current = false;
    setShowLatest(false);
  }, [sessionID]);
  useLayoutEffect(() => {
    if (follow.current && scroll.current)
      scroll.current.scrollTop = scroll.current.scrollHeight;
  }, [snapshot, sessionID]);

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
    </div>
  );
}
