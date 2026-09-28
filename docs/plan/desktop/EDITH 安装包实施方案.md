# EDITH 安装包实施方案

## 目标与约束

```text
make desktop-build       → 可运行的 EXE / APP
make desktop-package     → 构建并打包
                          ├─ EDITH-版本-windows-架构-setup.exe
                          └─ EDITH-版本-macos-架构.dmg
```

Wails Go 依赖、前端 runtime、CLI 统一为 beta.26。复用官方 NSIS 模板与 DMG 工具；版本取自 `build/version.txt`，产物进入 `.build/`。

## Windows

- NSIS 在构建电脑安装；仅当前用户安装到 `%LOCALAPPDATA%\Programs\EDITH`。
- 中文界面、开始菜单与卸载入口；桌面快捷方式可选，默认关闭。
- 缺少 WebView2 时联网安装，失败明确报错，不报告安装成功。
- 重装、升级保持身份与位置；拒绝低版本覆盖高版本。
- 安装、升级、卸载遇到 EDITH 运行（包括托盘）时提示退出后重试，不强制停止任务；文件替换失败报错。
- 卸载只删除安装器管理的文件与入口，保留 `~/.harness`、项目及安装目录中的其他文件。

## macOS

- 沿用 `.app` 与本机临时签名，调用 Wails 制作 DMG；保留 Bundle ID。
- EDITH 图标与 Applications 拖拽入口，简洁布局，不采用 Wails 品牌背景。
- 按本机构建架构输出；退出应用后通过 Finder 覆盖升级，保留用户数据。

## 构建与验收

- Makefile 提供入口，Taskfile 串联任务；脚本与资源归 `build/windows`、`build/macos`。
- 工具缺失或版本不符给出具体提示；普通构建不自动安装或升级工具。
- 不改业务接口；本批不做付费签名、公证、上传发布、自动更新。
- 改动完成后运行一次 `make agent-check`；正式发布前运行 `make test`。
- Windows 验证首次安装、启动、重装、升级、降级拒绝、运行拦截、卸载及数据保留。
- WebView2 缺失与失败用隔离环境验证，不卸载主机运行环境。
- Mac 由用户配合验证 DMG 构建、拖拽安装、启动、覆盖升级与数据保留。

## 进度

- [x] Wails beta.26 版本对齐、打包入口与工具检查；NSIS 3.12 官方便携版安装到当前用户目录。
- [x] Windows 安装脚本；隔离目录／登记验证安装、重装、升级、降级拒绝、运行拦截、文件占用失败、卸载与非安装器文件保留。
- [x] macOS DMG 流程。
- [x] `make agent-check`、`make desktop-package` 通过；最终 Windows 包实际安装、文件一致性、隔离用户目录启动冒烟、运行拦截及卸载通过。
- [ ] Mac 实机与隔离环境 WebView2 验收。

WebView2 安装失败已用独立测试注册表项和返回失败的模拟引导程序验证；真实缺失运行时的联网安装仍待隔离 Windows 环境验收。

Windows 启动冒烟确认进程存活、WebView2 环境创建和单实例拦截，不替代窗口视觉与聊天交互验收。Mac 上在仓库根目录运行 `make desktop-package`，再验证 DMG 拖拽安装与覆盖升级。本轮未发布，`make test` 留在正式发布前执行。
