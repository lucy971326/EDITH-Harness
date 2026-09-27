// CSS 构建会压缩十六进制颜色；Monaco 的 token theme 只接受完整的 RGB / RGBA。
export function monacoColor(value: string): string {
  const color = value.trim();
  if (/^#[\da-f]{3,4}$/i.test(color)) {
    return `#${[...color.slice(1)].map((digit) => digit + digit).join("")}`;
  }
  return color;
}
