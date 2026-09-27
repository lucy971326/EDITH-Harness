import { useCallback, useEffect, useRef, useState } from "react";

// 契约。页面决定何时允许离开；保存中可拒绝，脏草稿可先确认。
export type NavigationGuard = (proceed: () => void) => void;

function currentPath() {
  return window.location.hash.slice(1) || "/";
}

// Hash 地址同时适用于 Web 与 Desktop，不改变服务器路由或会话订阅。
export function usePageNavigation() {
  const [path, setPath] = useState(currentPath);
  const current = useRef({ path, index: Number(window.history.state?.harnessPageIndex) || 0 });
  const guard = useRef<NavigationGuard | null>(null);
  const setGuard = useCallback((next: NavigationGuard | null) => {
    guard.current = next;
  }, []);

  const navigate = useCallback((next: string, replace = false) => {
    if (next === current.current.path) return;
    const proceed = () => {
      const index = current.current.index + (replace ? 0 : 1);
      const state = { ...window.history.state, harnessPageIndex: index };
      if (replace) window.history.replaceState(state, "", `#${next}`);
      else window.history.pushState(state, "", `#${next}`);
      current.current = { path: next, index };
      setPath(next);
    };
    if (guard.current) guard.current(proceed);
    else proceed();
  }, []);

  useEffect(() => {
    window.history.replaceState({ ...window.history.state, harnessPageIndex: current.current.index }, "");
    let restoring: (() => void) | null = null;
    let acceptedIndex: number | null = null;
    const onLocationChange = () => {
      const next = currentPath();
      const knownIndex = window.history.state?.harnessPageIndex;
      const index = typeof knownIndex === "number" ? knownIndex : current.current.index + 1;
      if (typeof knownIndex !== "number") {
        window.history.replaceState({ ...window.history.state, harnessPageIndex: index }, "");
      }
      if (restoring) {
        if (index !== current.current.index) return;
        const requestLeave = restoring;
        restoring = null;
        requestLeave();
        return;
      }
      if (index === current.current.index && next === current.current.path) return;
      if (acceptedIndex === index) {
        acceptedIndex = null;
      } else if (guard.current && index !== current.current.index) {
        // 先回到当前历史项再询问，取消时不改写目标项；确认后重走原来的返回/前进。
        const proceed = () => {
          acceptedIndex = index;
          window.history.go(index - current.current.index);
        };
        restoring = () => {
          if (guard.current) guard.current(proceed);
          else proceed();
        };
        window.history.go(current.current.index - index);
        return;
      }
      current.current = { path: next, index };
      setPath(next);
    };
    // 返回/前进触发 popstate；手动修改 Hash 也需要进入同一条保护路径。
    window.addEventListener("popstate", onLocationChange);
    window.addEventListener("hashchange", onLocationChange);
    return () => {
      window.removeEventListener("popstate", onLocationChange);
      window.removeEventListener("hashchange", onLocationChange);
    };
  }, []);

  return { path, navigate, setGuard };
}
