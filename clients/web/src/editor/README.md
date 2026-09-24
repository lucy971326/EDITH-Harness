# editor

管理已打开文件的草稿、保存、冲突与监听。

```text
workspace-tabs -> useEditorFiles -> RPC 文件接口 -> machine
                       |
                  EditorFile 状态 -> CodeEditor / FileTree
```

- `use-files.ts`：按工作区保留文件、延迟保存、监听与关闭清理。
- `files.ts`：文件状态、编码、保存／外部变化的状态转换。
- `code-editor.tsx / monaco.ts`：编辑器组件、模型和主题。
- `file-tree.tsx / links.ts`：目录树与文件位置解析。

标签编排在上层 [`workspace-tabs.tsx`](../workspace-tabs.tsx)。保存携带读取版本；外部变化与本地草稿冲突时保留草稿，由用户选择。异步结果只更新仍归本页持有的文件对象。
