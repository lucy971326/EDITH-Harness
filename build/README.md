# 构建与开发输入

```text
build/
├─ config.yml       Wails 开发模式配置（CLI 默认位置）
├─ version.txt      唯一产品版本号
├─ models.mjs       开发时生成供应商与模型能力快照
├─ desktop.mjs      工具检查、桌面编译、版本资源与 EXE / APP / 安装包生成
├─ windows/         Windows 图标、manifest、NSIS 安装脚本及上游许可证
└─ macos/           macOS 图标
```

品牌图标源文件在 `clients/public/edith-icon.svg`，Windows 应用和各平台托盘使用的 PNG 在 `clients/desktop/edith-icon.png`。macOS 的 `icon.icns` 使用同一 SVG 图形，画布视口由 `0 0 64 64` 扩至 `-8 -8 80 80`，四周留 10% 透明边距；macOS 应用从 `Info.plist` 读取该 ICNS，不向 Wails 传入运行时应用图标，以免覆盖 Dock 图标。重新生成时只修改 macOS 图标，不缩小 Web 或 Windows 的品牌图标。

## 构建职责

```text
Makefile                 日常命令、Web 构建、检查、正式桌面构建
  ├─ web-build           npm + Vite（与 Web 共用）
  └─ desktop.mjs         Go 编译、Windows 资源 / macOS APP、安装包

wails3 dev               Taskfile.yml：开发热重建与运行

build/config.yml         仅负责 Wails 开发监听与进程启动
```

`make desktop-build` 与 `make desktop-package` 先准备前端，再由 `desktop.mjs` 检查工具、构建桌面应用；`dev` 只重建桌面程序，首次前端准备由 `config.yml` 调用 `make web-build`。依赖安装统一使用 Make 的文件依赖规则。

保留 Taskfile 是为了接入 Wails CLI；保留 dev 配置是为了热更新和进程生命周期；NSIS 脚本承载安装／升级／卸载保护。这些职责不能用普通 `go build` 替代。Vite、TypeScript 与 npm 配置仍各归对应工具。

`.build/` 是被 Git 忽略的构建输出；根目录只放可运行程序和分发包，中间文件统一写入 `temp/`。临时测试与日志也放进 `temp/`，验收结束后清理。

```text
.build/
├─ EDITH.exe / EDITH.app    可运行应用（按平台生成）
├─ EDITH-版本-平台-架构…    安装程序 / DMG
└─ temp/                   版本资源 JSON、WebView2 引导程序等中间文件
```

Go 链接需要的 `.syso` 仍在 `cmd/harness-desktop/`，由构建自动生成，不提交 Git。

## 安装包

`make desktop-package` 先检查工具、构建应用，再制作本机架构的分发包：

```text
Windows → .build/EDITH-版本-windows-amd64或arm64-setup.exe
macOS   → .build/EDITH-版本-macos-amd64或arm64.dmg
```

Wails CLI 须与 `go.mod`、`clients/package.json` 对齐（当前 beta.26）：

```sh
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.26
```

Windows 另需 `winget install --id NSIS.NSIS --exact`，或将官方便携版放入 `%LOCALAPPDATA%\Programs\NSIS`。检查支持 PATH、NSIS 默认安装位置和上述用户目录；普通构建不会自动安装工具。NSIS 脚本改编自 Wails 官方模板，许可证保留在 `windows/WAILS-LICENSE.txt`。

Windows 仅当前用户安装，目录固定为 `%LOCALAPPDATA%\Programs\EDITH`。升级与卸载前须从托盘退出 EDITH；拒绝降级，卸载保留用户数据和安装目录中的其他文件。缺少 WebView2 时需要网络下载运行环境。静默 `/S` 的错误码：10 为应用或安装器运行中，11 为降级，20 为 WebView2 失败，30 为文件或登记失败，64/65 为系统或架构不符。

macOS 必须在 Mac 上打包；DMG 内拖拽 EDITH 到 Applications。更新前退出应用，再替换旧 `.app`。当前只有本机临时签名，不包含开发者签名、公证或自动更新。

## GitHub 发版

[Desktop CI and release](../.github/workflows/desktop.yml) 在 PR、`main` 更新和手动触发时运行 `make test`，随后分别在 macOS arm64 与 Windows amd64 上运行与本机相同的 `make desktop-package`，安装包保留在该次 Actions 的 Artifacts 中。测试或任一平台打包失败时，不会创建 Release。

发版只需一个指向 `main` 已合入提交的版本标签。标签须与 `build/version.txt` 相同，例如文件为 `0.1.0` 时使用 `v0.1.0`：

```sh
git switch main
git pull --ff-only
git tag -a v0.1.0 -m "EDITH v0.1.0"
git push origin v0.1.0
```

标签触发相同的检查和打包，并生成含两个安装包与 `SHA256SUMS.txt` 的 **GitHub Release 草稿**。在 Actions 成功后打开草稿，核对版本、文件名和说明，再手动发布；工作流不会自动公开下载页。后续版本先修改 `build/version.txt` 并合入 `main`，再推送对应标签。当前安装包未做分发签名或 macOS 公证，暂不承诺自动更新。

## 模型目录

`make models-update` 从 [models.dev](https://models.dev) 拉取数据，筛选支持的供应商和可调用工具的文本模型，生成 `internal/llm/catalog.json`。脚本另有明确的协议、思考参数和订阅模型映射；不是把任意元数据当请求参数发送。输出记录来源与内容摘要，更新后审查 diff 并提交快照。普通构建与运行不调用该脚本。

可用 `node build/models.mjs .build/temp/models-dev.json` 从保存的输入重现生成结果。缺失必要字段时失败，保留原快照。

供应商名单对齐本地 Pi 的 OpenAI／Anthropic 入口，ID、地区与套餐分支在 `models.mjs` 显式维护；不自动收录源站新增供应商。Bedrock、Google／Vertex、Azure 专用适配、Mistral Conversations、Radius 和 TypeSafe 排除，混合网关只生成支持的协议模型。Ant Ling 在源站缺少独立目录时采用 Pi 的明确模型定义。现有 xAI OAuth 作为独立认证入口保留。
