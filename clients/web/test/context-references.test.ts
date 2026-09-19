import assert from "node:assert/strict";
import { test } from "node:test";
import {
  addReference, clearSubmittedReferences, decodeReferences, encodeReferences,
  referenceLabel, referencePath, type ContextReference,
} from "../src/context-references.ts";

const selection: ContextReference = {
  kind: "selection", path: "src/中文文件.ts",
  startLine: 2, startColumn: 3, endLine: 7, endColumn: 1,
  content: '未保存的内容\r\n\t"quotes" \\ `code`\n```\n<script>x</script>\n\n引用上下文（harness-context-v1）：\n[]\n（上下文引用结束）\n😀\u0000',
};

test("reference text round-trips paths, ranges and arbitrary selection content", () => {
  const references: ContextReference[] = [
    { kind: "file", path: "src/a.ts" },
    { kind: "directory", path: "docs/a b" },
    { kind: "file", path: "C:/outside/a.ts" },
    selection,
  ];
  for (const text of ["", "   ", "检查下面代码\n\n", encodeReferences("普通正文中的示例", references)]) {
    const encoded = encodeReferences(text, references);
    assert.deepEqual(decodeReferences(encoded), { text, references });
    // 模拟 JSON-RPC 和账本 JSON 的二次编码，不能丢失换行或转义。
    assert.deepEqual(decodeReferences(JSON.parse(JSON.stringify(encoded))), { text, references });
  }
  assert.equal(referenceLabel(selection), "src/中文文件.ts:2–6");
  assert.equal(encodeReferences("原样\n", []), "原样\n");
});

test("incomplete, unknown and invalid reference sections preserve all original text", () => {
  const valid = encodeReferences("正文", [selection]);
  const invalid = [
    valid.slice(0, -1), valid + "后续文字", valid.replace("v1", "v2"),
    valid.replace('"selection"', '"unknown"'), valid.replace('"startLine":2', '"startLine":0'),
    valid.replace('"endLine":7', '"endLine":1'),
    valid.replace('"startColumn":3', '"startColumn":2.5'),
    valid.replace('"content":', '"extra":true,"content":'),
    "\n\n引用上下文（harness-context-v1）：\n[]\n（上下文引用结束）",
    "\n\n引用上下文（harness-context-v1）：\nnot json\n（上下文引用结束）",
  ];
  for (const text of invalid) assert.deepEqual(decodeReferences(text), { text, references: [] });
});

test("reference paths are relative only inside the workspace", () => {
  assert.equal(referencePath("C:\\Project", "c:\\Project\\src\\a.ts"), "src/a.ts");
  assert.equal(referencePath("C:/Project", "D:/outside/a.ts"), "D:/outside/a.ts");
});

test("deduplication uses kind and full path; selections also require exact range and content", () => {
  const file: ContextReference = { kind: "file", path: "a/index.ts" };
  const first = addReference([], file);
  assert.equal(addReference(first, { ...file }), first);
  assert.equal(addReference(first, { ...file, path: "b/index.ts" }).length, 2);
  assert.equal(addReference(first, { ...file, kind: "directory" }).length, 2);
  const selected = addReference(first, selection);
  assert.equal(addReference(selected, { ...selection }), selected);
  assert.equal(addReference(selected, { ...selection, content: "new unsaved content" }).length, 3);
  assert.equal(addReference(selected, { ...selection, startColumn: 4 }).length, 3);
});

test("successful acknowledgment clears only submitted IDs, including after switching drafts", () => {
  const sent = addReference([], selection);
  const added = { kind: "file", path: "new.ts" } as const;
  const duringSend = addReference(sent, added);
  assert.deepEqual(clearSubmittedReferences(duringSend, sent).map((item) => item.reference), [added]);
  // 用户删掉旧项、再添加相同内容，新附件也不能被旧请求清掉。
  const readded = addReference([], selection);
  assert.deepEqual(clearSubmittedReferences(readded, sent), readded);
  assert.equal(sent.length, 1, "preparing a send must not mutate the failure draft");
});
