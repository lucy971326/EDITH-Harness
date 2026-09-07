import { execFileSync } from 'node:child_process';
import { mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { compile } from 'json-schema-to-typescript';

// 只生成契约文件；不读取用户配置、不安装产品、不启动网络。
const root = fileURLToPath(new URL('../', import.meta.url));
const output = path.join(root, 'appserver/generated');
const check = process.argv.includes('--check');
const catalog = JSON.parse(execFileSync('go', ['run', './cmd/contracts'], { cwd: root, encoding: 'utf8' }));
const files = new Map([['catalog.json', JSON.stringify(catalog, null, 2) + '\n']]);
const imports = [];
const methods = [];

for (const method of catalog) {
  const stem = method.name.replaceAll('/', '-');
  const typeStem = method.name.split('/').map(part => part[0].toUpperCase() + part.slice(1)).join('');
  for (const [key, suffix, typeSuffix] of [['inputSchema', 'params', 'Params'], ['outputSchema', 'result', 'Result']]) {
    const typeName = typeStem + typeSuffix;
    const filename = `${stem}.${suffix}`;
    const schema = { ...forTypeScript(method[key]), title: typeName };
    files.set(`${filename}.d.ts`, await compile(schema, typeName, { bannerComment: '// Generated from Go contracts. Do not edit.', unknownAny: true }));
    imports.push(`import type { ${typeName} } from './${filename}';`);
  }
  methods.push(`  '${method.name}': { params: ${typeStem}Params; result: ${typeStem}Result };`);
}
files.set('index.d.ts', '// Generated from Go contracts. Do not edit.\n' + imports.join('\n') + '\n\nexport interface Methods {\n' + methods.join('\n') + '\n}\n');

if (!check) await mkdir(output, { recursive: true });
for (const [name, content] of files) {
  const target = path.join(output, name);
  if (check) {
    const actual = await readFile(target, 'utf8');
    if (actual !== content) throw new Error(`Stale contract: ${name}; run npm run contracts:generate`);
  } else {
    await writeFile(target, content);
  }
}
for (const name of await readdir(output)) {
  if (!files.has(name)) throw new Error(`Unexpected generated contract: ${name}`);
}
console.log(`${check ? 'Checked' : 'Generated'} ${files.size} contract files.`);

function forTypeScript(schema) {
  if (Array.isArray(schema)) return schema.map(forTypeScript);
  if (!schema || typeof schema !== 'object') return schema;
  const mapped = Object.fromEntries(Object.entries(schema).map(([key, value]) => [key, forTypeScript(value)]));
  // TS 的 {} 接受标量；只对 Go 生成的封闭空对象做等价映射，不改运行时 Schema。
  if (schema.type === 'object' && schema.additionalProperties === false &&
      Object.keys(schema.properties ?? {}).length === 0 && !schema.patternProperties &&
      !schema.allOf && !schema.anyOf && !schema.oneOf) {
    mapped.tsType = 'Record<string, never>';
  }
  return mapped;
}
