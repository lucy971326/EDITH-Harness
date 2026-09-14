import { joinPath } from "./files.ts";

export interface FileLocation {
  path: string;
  line?: number;
  column?: number;
}

export function parseFileLink(
  href: string | undefined,
  workspace: string | null,
): FileLocation | null {
  if (!href || href.startsWith("#")) return null;
  let value: string;
  try {
    value = decodeURIComponent(href);
  } catch {
    return null;
  }
  if (/^[a-z][a-z\d+.-]*:\/\//i.test(value)) return null;

  let line: number | undefined;
  let column: number | undefined;
  const hashLocation = value.match(/#L(\d+)(?::(\d+))?$/i);
  const colonLocation = value.match(/:(\d+)(?::(\d+))?$/);
  const location = hashLocation ?? colonLocation;
  if (location) {
    line = Number(location[1]);
    column = location[2] ? Number(location[2]) : undefined;
    value = value.slice(0, -location[0].length);
  }

  const windowsAbsolute = /^[a-z]:[\\/]/i.test(value);
  const posixAbsolute = value.startsWith("/");
  if (!windowsAbsolute && !posixAbsolute) {
    if (!workspace || value.startsWith("mailto:")) return null;
    value = joinPath(workspace, value.replace(/^\.\/?/, ""));
  }
  return { path: value, line, column };
}
