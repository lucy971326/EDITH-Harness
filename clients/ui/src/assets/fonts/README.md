# 内置字体

`styles.css` 定义字体资源与公共 Token，`fonts.ts` 负责加载成功后切换与本机偏好。
正式构建由 Vite 打包，Web / Desktop 共用；不请求字体 CDN。

| 用途 | 文件 / 来源 | 处理 |
| --- | --- | --- |
| 固定 UI、默认内容 | MiSans：用户提供 Downloads/MiSans.zip | 原始 WOFF2，保留 400/500/600/700 |
| 可选内容 | [HarmonyOS Sans 官方包](https://developer.huawei.com/images/download/general/HarmonyOS-Sans.zip) | SC 原始 TTF，保留 400/500/700，不改写、不裁剪 |
| 默认代码 | npm @fontsource-variable/jetbrains-mono | 原包 WOFF2 |
| 可选代码 | [Cascadia Mono v2407.24](https://github.com/microsoft/cascadia-code/releases/tag/v2407.24) | 正体、斜体可变 TTF 转 WOFF2 |
| 可选代码 | [Maple Mono v7.9](https://github.com/subframe7536/maple-font/releases/tag/v7.9) | 非 CN 版，四个字形转 WOFF2 |
| 统一代码中文 | [Noto Sans Mono CJK SC](https://github.com/notofonts/noto-cjk/tree/main/Sans/Mono) | Regular/Bold OTF 转 WOFF2，完整字形 |

许可放在 `clients/public/font-licenses/`，外观设置仅展示字体名称、选择器和预览，不展示额外说明或许可链接。
MiSans、HarmonyOS Sans 保持官方下载文件内容；OFL 字体只做容器转换，不改字形。

代码字体的 Latin 字宽：JetBrains / Maple 为 0.6em，Cascadia 为 0.5859375em；
Noto Mono 中文为 1em。CSS 对同一份中文资源使用 120% / 117.1875% 的 `size-adjust`，
使常用汉字与全角标点占两格。终端自身也按 Unicode 宽度规则分格。
这是字体度量验证，不能替代不同平台 WebView 的光标与排版验收。

内容字体各自同时提供中英文，不串接另一套内容字体。常用文字不依赖系统字体；
生僻字、Emoji 等超出内置字形覆盖时，浏览器仍可能使用系统补字，不能保证覆盖全部 Unicode。
字体选择切换前加载所需资源；启动失败显示重试入口，不留下空白页。
