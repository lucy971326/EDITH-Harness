# Desktop 构建输入

```text
build/
├─ config.yml       Wails 开发模式配置（CLI 默认位置）
├─ version.txt      唯一产品版本号
├─ desktop-package.mjs  打包工具检查与 NSIS / DMG 调用
├─ windows/         Windows 图标、manifest、版本资源与 NSIS 安装脚本
└─ macos/           macOS 图标与 .app 打包脚本
```

`Taskfile.yml` 调用这些文件；`make desktop-build` 是入口。品牌图标源文件在 `clients/public/edith-icon.svg`，运行时 PNG 在 `clients/desktop/edith-icon.png`。

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
