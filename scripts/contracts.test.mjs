import { test } from 'node:test';
import { execFileSync } from 'node:child_process';
import { mkdtemp, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { compile } from 'json-schema-to-typescript';

test('Go → Schema → TS preserves optional, enum, array, reference and time shapes', async () => {
  const root = fileURLToPath(new URL('../', import.meta.url));
  const schema = JSON.parse(execFileSync('go', ['run', './appserver/testdata/contract'], { cwd: root, encoding: 'utf8' }));
  const dir = await mkdtemp(path.join(tmpdir(), 'harness-contract-types-'));
  try {
    await writeFile(path.join(dir, 'example.d.ts'), await compile(schema, 'Example'));
    await writeFile(path.join(dir, 'check.ts'), `
import type { Example } from './example';
const valid: Example = { name: 'test', mode: 'read', items: [{ value: 'x' }], at: '2026-09-07T10:00:00Z' };
const optional: Example = { ...valid, note: 'optional' };
// @ts-expect-error 枚举不能任意扩张。
const wrongEnum: Example = { ...valid, mode: 'execute' };
// @ts-expect-error 必填字段不能省略。
const missing: Example = { mode: 'read', items: [], at: '' };
// @ts-expect-error 引用的数组元素必须保持结构。
const wrongItems: Example = { ...valid, items: ['x'] };
// @ts-expect-error 时间映射为字符串。
const wrongTime: Example = { ...valid, at: new Date() };
// @ts-expect-error 可选字段仍有类型。
const wrongOptional: Example = { ...valid, note: 1 };
void [optional, wrongEnum, missing, wrongItems, wrongTime, wrongOptional];
`);
    await writeFile(path.join(dir, 'tsconfig.json'), JSON.stringify({ compilerOptions: { strict: true, noEmit: true, types: [] }, files: ['check.ts'] }));
    execFileSync(process.execPath, [path.join(root, 'node_modules/typescript/bin/tsc'), '-p', path.join(dir, 'tsconfig.json')], { encoding: 'utf8' });
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});
