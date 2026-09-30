# CI/CD：自动检查与发版

这里的 CI/CD 指 GitHub 自动运行测试、制作桌面安装包，并在发版时准备下载页面。实际规则见 [Desktop 工作流](../../.github/workflows/desktop.yml)；本机打包命令与安装要求见 [打包说明](../../build/README.md)。

## 什么时候自动运行

| 操作 | GitHub 会做什么 |
| --- | --- |
| 只在本机改代码 | 不运行 |
| 推送普通分支，但没有打开 PR | 不运行 |
| 打开或更新 PR | 运行测试，制作 Mac 和 Windows 安装包 |
| 推送到 `main` | 运行同样的测试与打包，不公开新版本 |
| 推送 `v0.1.0` 这样的版本标签 | 检查标签、运行测试与打包，成功后生成 Release 草稿 |

PR 是申请把修改合入主线的页面。也可以在 GitHub Actions 中手动运行工作流；通常选择 `main` 时只检查和打包。

## GitHub 具体做什么

```text
PR / main / 版本标签
         ↓
      make test
         ↓ 通过后
  Mac DMG + Windows 安装包
         ↓ 仅版本标签
  校验文件 → Release 草稿 → 人工确认后公开
```

测试失败就不打包；任一平台打包失败就不生成 Release 草稿。PR 和 `main` 的安装包可在对应的 GitHub Actions 运行记录中下载，保留 7 天。当前自动构建的平台是 macOS Apple Silicon 和 Windows x64。

## 真正发版时

先将版本号写入 [`build/version.txt`](../../build/version.txt) 并合入 `main`。确认 `main` 的检查通过后，推送对应的 `v` 开头标签；例如版本文件是 `0.1.0`，标签就用 `v0.1.0`。标签必须指向已合入 `main` 的提交，否则工作流会失败。

标签检查与两平台打包都通过后，GitHub 创建包含两个安装包和 `SHA256SUMS.txt` 的 **Release 草稿**。草稿不会自动公开；核对文件与说明后，再人工发布。当前 Windows 安装包未签名，macOS 应用尚未公证。
