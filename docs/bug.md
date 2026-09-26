# 待修复 Bug

仅暂存当前未修复的问题；修复后删除对应条目。

## 损坏的会话文件使项目列表整体失败，且错误不可诊断

- 触发：已有会话的 `sessions/<id>/meta.json` 损坏或 ID 不匹配，或 `settings.json` 损坏、缺失时，整个会话列表加载失败；页面显示「无法加载项目列表：Internal error」。干净数据目录首次启动报错的原因尚未确认。
- 根因：`jsonl.List` 与 `conversations.List` 遇坏项直接返回错误；`clientconn.rpcError` 把未知业务错误统一翻译成 "Internal error"，连接层使用 `discardLogger`，原始错误未记录。
- 修法方向：Internal error 分支至少往服务端日志打一行原始错误；List 遇单文件损坏时跳过并报告，不整体失败。
