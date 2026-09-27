import assert from "node:assert/strict";
import { test } from "node:test";
import {
  applyExternalFile,
  applySavedFile,
  decodeFile,
  encodeFile,
  joinPath,
  needsCloseConfirmation,
  projectEditorState,
  type EditorFile,
} from "../src/editor/files.ts";
import { parseFileLink } from "../src/editor/links.ts";

test("UTF-8 file content survives the JSON-RPC base64 boundary", () => {
  const content = "你好，Monaco 👋\n";
  assert.equal(decodeFile(encodeFile(content)), content);
});

test("file links resolve relative paths and line locations", () => {
  assert.deepEqual(parseFileLink("src/main.ts:12:4", "C:\\work"), {
    path: "C:\\work\\src/main.ts",
    line: 12,
    column: 4,
  });
  assert.equal(parseFileLink("https://example.com/a.ts:12", "C:\\work"), null);
  assert.equal(joinPath("/work", "src"), "/work/src");
});

test("a save response preserves edits typed while the save was running", () => {
  const file = sampleFile();
  file.content = "second edit";
  applySavedFile(file, "first edit", "b".repeat(64));
  assert.equal(file.savedContent, "first edit");
  assert.equal(file.content, "second edit");
  assert.equal(file.status, "dirty");
});

test("an external change reloads clean files and protects dirty drafts", () => {
  const clean = sampleFile();
  assert.equal(applyExternalFile(clean, "disk edit", "b".repeat(64)), "reloaded");
  assert.equal(clean.content, "disk edit");

  const dirty = sampleFile();
  dirty.content = "local edit";
  dirty.status = "dirty";
  assert.equal(applyExternalFile(dirty, "disk edit", "b".repeat(64)), "conflict");
  assert.equal(dirty.content, "local edit");
  assert.equal(dirty.diskContent, "disk edit");
  assert.equal(needsCloseConfirmation(dirty), true);
  assert.equal(needsCloseConfirmation(clean), false);
});

test("each project restores its own active file", () => {
  const projects = new Map();
  projectEditorState(projects, "C:\\one").activePath = "one.go";
  projectEditorState(projects, "C:\\two").activePath = "two.go";
  assert.equal(projectEditorState(projects, "C:\\one").activePath, "one.go");
});

function sampleFile(): EditorFile {
  return {
    path: "C:\\work\\main.go",
    name: "main.go",
    content: "original",
    savedContent: "original",
    hash: "a".repeat(64),
    status: "saved",
  };
}
