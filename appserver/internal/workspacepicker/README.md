# workspacepicker

调用操作系统原生目录选择器：

```text
macOS     osascript
Linux     zenity / kdialog
Windows   IFileDialog
```

只返回用户选择的路径或取消，不创建 Session，不读取 Product。
