# Desktop 构建输入

```text
build/
├─ config.yml       Wails 开发模式配置（CLI 默认位置）
├─ version.txt      唯一产品版本号
├─ desktop.mjs      工具检查、桌面编译、版本资源与 EXE / APP / 安装包生成
├─ windows/         Windows 图标、manifest、NSIS 安装脚本及上游许可证
└─ macos/           macOS 图标
```

品牌图标源文件在 `clients/public/edith-icon.svg`，运行时 PNG 在 `clients/desktop/edith-icon.png`。

## 构建职责

```text
Makefile                 日常命令、Web 构建、检查
  └─ Wails / Taskfile    薄接入：build / package / build:dev / run
       └─ desktop.mjs   唯一桌面构建流程
            ├─ make web-build → npm + Vite（与 Web 共用）
            ├─ Go 编译 + Windows 资源 / macOS APP
            └─ NSIS / Wails DMG（仅 package）

build/config.yml         仅负责 Wails 开发监听与进程启动
```

`desktop.mjs` 的 `build` 与 `package` 都先检查工具，再完整构建前端和桌面应用；`dev` 只重建桌面程序，首次前端准备由 `config.yml` 调用 `make web-build`。依赖安装统一使用 Make 的文件依赖规则，不再在 Taskfile 维护第二套缓存判断。

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
