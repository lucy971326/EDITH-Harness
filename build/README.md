# Desktop 构建输入

```text
build/
├─ config.yml       Wails 开发模式配置（CLI 默认位置）
├─ version.txt      唯一产品版本号
├─ windows/         Windows 图标、DPI manifest、版本资源生成器
└─ macos/           macOS 图标与 .app 打包脚本
```

`Taskfile.yml` 调用这些文件；`make desktop-build` 是入口。品牌图标源文件在 `clients/public/edith-icon.svg`，运行时 PNG 在 `clients/desktop/edith-icon.png`。

`.build/` 是被 Git 忽略的构建输出，包含程序和临时资源；它与本目录用途不同。
