# 会话操作

> Harness 核心会话操作与协调。

```text
appserver
   ↓
会话操作
   ├─ 创建 / 列出 / 获取会话
   ├─ 发送或 Steer
   ├─ 停止父子任务
   ├─ 生成 Snapshot
   ├─ 分叉会话
   └─ 接受平台命令
         ↓
   Session / Runner / Agents / LLM / Commands / Subagents
```

## 从哪里读

```text
service.go  业务流程
types.go    会话输入与 Snapshot
errors.go   可识别业务错误
```

## 边界

- 做：决定一次会话操作要按什么顺序调用公共服务。
- 不做：JSON-RPC 编解码、网页渲染、底层文件格式或模型实现。
- 公共的 Agent、Model、Skill 列表由 appserver 直接调用公共服务，不经这里转发。
