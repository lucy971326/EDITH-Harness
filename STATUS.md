# 项目状态

更新日期：2026-08-31

## 现在是什么

Harness 已有可测试的内核主链，但还没有可运行的应用入口和 Web 页面。

```text
用户输入 → Runner → Agent 设置 → React Loop → LLM / Tools
                    Loop 事件 → Runner → 完整消息落账 → events → UI
```

目前只能运行各包测试；还没有完整 Host 组装、真实模型调用或端到端聊天验证。

## 已完成

- Host 服务表与插件生命周期。
- JSONL 持久化、Session 账本与 SessionSettings。
- DeepSeek LLM、machine-local、read / write / edit / bash。
- Skills 登记处与 Agent 设置；尚无具体 Skill 插件。
- Loops 登记处、默认 React Loop。
- Events、Runner.Run / Steer / Stop。
- HTTP 服务器与路由登记处。
- `go test ./...`、`go vet ./...` 通过。

当前没有 sqlite、E2B、Runner.Spawn / Wait、Web/SSE 和真实应用入口。

## 下一步

```text
真实聊天页面原型
  → surface/web + kernel/pages 一起设计与实现
  → templ + HTMX + SSE 跑通聊天
  → cmd/harness 组装全部插件
```

页面原型在 `learn/prototypes/chat-page.html`，其中的交互是静态演示，尚未连接
Runner、HTTP 或 SSE。未来方案统一写入 `learn/插件设计书.md`，不再建立独立计划书目录。

## 已知问题

- HTTP 插件能关闭服务器，但还没有等待后台 Serve 退出；同时后台入口仍在普通包内打印日志，与 `AGENTS.md` 新规范不符。
- 还没有 `kernel/pages`、`surface/web`、`cmd/harness`，因此目前不能启动真实聊天应用。
- 现有测试覆盖各包主链，但没有完整插件组装和真实 LLM 的端到端测试。

## 运行前提

- Go 1.25。
- LLM 当前只支持 `deepseek/deepseek-v4-flash` 和 `deepseek/deepseek-v4-pro`。
- `persist.Plugin.Dir` 必须由未来入口明确指定；当前没有默认数据目录。
- 创建 Session 后必须写入对应 SessionSettings，Runner 才能开始本轮。
- machine-local 直接操作本机文件和进程，没有沙箱与路径限制。

新电脑自行创建 `~/.harness/config.yaml`：

```yaml
providers:
  deepseek:
    apiKey: <your-key>
    # baseURL: <optional>
```

## 换机与本机数据

- `reference/` 被 Git 忽略，换机时单独复制；其中的 DSH、pi、trpc-agent-go 等源码不是 Harness 的方案。
- 换机后执行 `go test ./...` 和 `go vet ./...` 验证环境；这不代表真实聊天已经跑通。
- `~/.harness/config.yaml` 不进仓库，新电脑按上面的格式重新创建。
- 账本、SessionSettings 和自建 Agent 保存在 persist 插件配置的目录，不属于源码。
