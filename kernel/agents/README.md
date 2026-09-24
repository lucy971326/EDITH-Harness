# agents

拥有 Agent 设置、文件格式、本轮准备与引用协调。`NewStore` 读写 `agents/<id>.json`，`NewService` 接入工具、Loop、Skills 和会话设置。

普通工具按 Agent 设置准备，MCP 与 Skills 按作用域发现。Agent 删除与会话引用写入、目录读取互斥，避免悬空引用与并发删除读取失败。

不自动改写旧工具名或过滤已保存的工具选择；不运行 Loop、不写对话账本。
