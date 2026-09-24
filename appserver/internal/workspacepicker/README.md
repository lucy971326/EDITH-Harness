# workspacepicker

调用系统原生目录选择器，只返回路径或取消。

```text
Pick -> macOS: osascript
     -> Linux: zenity / kdialog
     -> Windows: IFileDialog
```

从 `picker.go` 看入口，再读对应的 `picker_*.go`。不创建会话、不修改设置；由调用方处理选择结果。
