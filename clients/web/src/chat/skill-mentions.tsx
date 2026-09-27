import { BookOpenCheck } from "../icons";

export type SkillPart =
  | { kind: "text"; value: string }
  | { kind: "skill"; name: string; value: string };

export function replaceSkillText(text: string, start: number, end: number, inserted: string): string {
  return text.slice(0, start) + inserted + text.slice(end);
}

// 只装饰当前目录里的完整 Skill 名称；原始消息始终保留 $name 文本。
export function skillParts(text: string, names: ReadonlySet<string>): SkillPart[] {
  if (!names.size) return [{ kind: "text", value: text }];
  const parts: SkillPart[] = [];
  const pattern = /\$([a-z0-9]+(?:-[a-z0-9]+)*)/g;
  let start = 0;
  for (const match of text.matchAll(pattern)) {
    const index = match.index;
    const end = index + match[0].length;
    const before = index ? text[index - 1] : "";
    const after = text[end] ?? "";
    if (!names.has(match[1]) || (before && /[\w$]/.test(before)) ||
      (after && /[\w-]/.test(after))) continue;
    if (index > start) parts.push({ kind: "text", value: text.slice(start, index) });
    parts.push({ kind: "skill", name: match[1], value: match[0] });
    start = end;
  }
  if (start < text.length || !parts.length)
    parts.push({ kind: "text", value: text.slice(start) });
  return parts;
}

export function SkillTag({ name }: { name: string }) {
  return (
    <span className="skill-token" aria-label={`Skill ${name}`} title={`Skill · ${name}`}>
      <BookOpenCheck aria-hidden="true" />
      <span>{name}</span>
    </span>
  );
}
