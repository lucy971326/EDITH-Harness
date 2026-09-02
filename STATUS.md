# 项目状态

更新日期：2026-09-02

## 现在是什么

Harness 已完成阶段 1、2、3：它是可启动的 Web 聊天产品，具备项目/会话管理、真实 Runner 调度与 SSE 流式界面。

```text
浏览器 POST → Chat → Runner → Agent 设置 → React Loop → LLM / Tools
浏览器 SSE  ← Chat ← Runner 稳定事件 ← 完整消息先落账
```

运行 `go run ./cmd/harness` 会读取 `harness.yaml`，组装完整服务链，自动选择空闲端口并打开 Chat。

## 已完成

### 阶段 1：Web 基础

- `surface/web`：HTTP Server、产品/路由登记处、templ 通用页面壳。
- `plugins/products/chat`：Chat 产品注册及页面。
- 本地嵌入 HTMX `2.0.10`、SSE 扩展 `2.2.4`；Tailwind 编译到 `surface/web/static/site.css`。

### 阶段 2：项目与会话

- `SessionMeta` 独立保存 `ID / Title / CreatedAt`；元数据是空会话存在的依据。
- 新会话显示「新对话」；首条用户消息落账后自动改名。
- Chat 按 `Workspace` 分组展示项目与会话；项目不是独立数据。
- Win / macOS / Linux 原生目录选择；取消返回 Chat，真实错误才显示。

### 阶段 3：真实聊天

- 启动链已完整组装：`persist → session → llm → machine-local → tools → events → loops/react → skills → agents → runner → web → chat`。
- 每轮生成 `RunID`，写入本轮耐久消息与 SSE；History Snapshot 和 SSE 共用前端 reducer / `paint()`。同一 Run 默认合并为一张助手卡，只有耐久 Steer 才切成前后片段；落账完成不会让实时卡片跳位。
- Chat 支持普通发送、停止、Steer、FollowUp；FollowUp 的顺序由 Runner 按 Session 管理，不属于 Chat。
- `Runner.Start` 同步占住 Session，再在 Runner 管理的 goroutine 运行；`Runner.Close` 会取消并等待仍在运行的 Run。
- Runner 对界面只发布稳定事件：开始、文本/推理 Delta、工具开始/完成、耐久消息、结束状态。
- 每次 SSE 重连重新同步 History；慢客户端被断开，不会阻塞 Run。
- 模型与思考档位是独立选择框；首次发送前两者必选，换模型会清空档位。

## 验证

- `go test ./...` 通过。
- `go test -race ./kernel/runner ./kernel/session ./kernel/llm ./plugins/products/chat ./cmd/harness` 通过。
- `go vet ./...` 通过。
- `git diff --check` 通过。
- 已做真实浏览器页面与布局检查。

## 下一步

```text
阶段 4：页面插槽
  ├─ message.actions：复制、分叉、插件动作
  ├─ dock：持续状态
  ├─ composer.actions：附件等输入动作
  ├─ sidepanel：文件面板
  └─ settings.section：插件设置二级列表

阶段 5：完整验收
  ├─ 轨迹页、Session 分叉、模型/档位后续修改
  └─ 产品切换、SSE 重连、慢客户端、取消、关闭与浏览器验收
```

## 运行前提

- Go 1.25。
- 修改 `.templ` 后运行 `go tool templ generate`。
- 修改样式后运行 `npm run web:build`；首次需要 `npm install`。
- 数据目录由 `harness.yaml` 的 `dataDir` 指定；账本、SessionSettings 和自建 Agent 不属于源码。
- machine-local 直接操作本机文件和进程，没有沙箱与路径限制。
- 本机需要 `~/.harness/config.yaml` 配置 LLM Provider：

```yaml
providers:
  deepseek:
    apiKey: <your-key>
    # baseURL: <optional>
```

## 近期提交

- `e08d515 docs: document real chat runtime`
- `e6fc46c feat: implement real chat stage 3`
- `dc784e5 feat: add project-grouped session navigation`
