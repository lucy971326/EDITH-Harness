import assert from "node:assert/strict";
import { test } from "node:test";
import type { SessionView } from "../../contracts/harness.ts";
import { groupSessions, sessionInList, workspaceName } from "../src/state/projects.ts";

function session(id: string, workspace: string, createdAt: string, title = id): SessionView {
  return {
    sessionID: id,
    title,
    createdAt,
    settings: { agentID: "default", model: "", reasoningEffort: "", workspace },
  };
}

test("groups by workspace, newest first, named by directory", () => {
  const groups = groupSessions([
    session("old", "/Users/me/Alpha", "2026-01-01T00:00:00Z", "旧"),
    session("new", "/Users/me/Alpha", "2026-02-01T00:00:00Z", "新"),
    session("other", "/Users/me/Beta", "2026-03-01T00:00:00Z", "另一个"),
  ]);
  assert.equal(groups.length, 2);
  assert.equal(groups[0].name, "Alpha");
  assert.deepEqual(groups[0].sessions.map((item) => item.sessionID), ["new", "old"]);
  assert.equal(groups[1].name, "Beta");
});

test("workspaceName uses the last path segment", () => {
  assert.equal(workspaceName("/Users/me/Harness/"), "Harness");
  assert.equal(workspaceName("C:\\Projects\\App"), "App");
});

test("sessionInList is used to decide whether a selection survived reconnect", () => {
  const sessions = [session("keep", "/tmp/a", "2026-01-01T00:00:00Z")];
  assert.equal(sessionInList(sessions, "keep"), true);
  assert.equal(sessionInList(sessions, "gone"), false);
});
