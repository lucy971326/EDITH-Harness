# 项目状态

更新日期：2026-09-01

## 现在是什么

Harness 已有可测试的内核主链和可启动的 Web 产品壳，但尚未接入真实聊天。

```text
用户输入 → Runner → Agent 设置 → React Loop → LLM / Tools
                    Loop 事件 → Runner → 完整消息落账 → events → UI
```

运行 `go run ./cmd/harness` 会读取 `harness.yaml`、自动选择空闲端口并打开 Chat 产品壳。当前入口只组装 Web 与 Chat；完整内核、真实模型调用和端到端聊天仍未接入。

## 已完成

- Host 服务表与插件生命周期。
- JSONL 持久化、Session 账本与 SessionSettings。
- DeepSeek LLM、machine-local、read / write / edit / bash。
- Skills 登记处与 Agent 设置；尚无具体 Skill 插件。
- Loops 登记处、默认 React Loop。
- Events、Runner.Run / Steer / Stop。
- `surface/web`：HTTP Server、产品与路由登记处、templ 通用页面壳。
- `plugins/products/chat`：默认 Chat 产品入口与静态页面壳。
- `cmd/harness`：读取 YAML、组装插件、打开浏览器并正确关闭服务器。
- 本地嵌入 HTMX `2.0.10`、SSE 扩展 `2.2.4` 与 Tailwind 编译产物。
- `go test ./...`、`go vet ./...` 通过。

当前没有 sqlite、E2B、Runner.Spawn / Wait、Chat SSE 和真实聊天组装。

## 下一步

```text
SessionMeta 与项目 / 会话列表
  → Chat 读取 Session + SessionSettings
  → 按 Workspace 组成项目列表
  → 实现新建项目与原生目录选择器
  → 再接 Runner、JSON SSE 与真实消息区
```

页面原型在 `learn/prototypes/chat-page.html`；真实阶段 1 页面已经由 `surface/web` 与
`plugins/products/chat` 提供，但尚未连接 Session、Runner 或 SSE。`playground/sse-draft` 是已验证的消息区画法：SSE 与 History JSON 都交同一个 `paint()`；尚未接入内核。

## 已知问题

- 当前 `cmd/harness` 只验证 Web + Chat 产品壳，尚未组装现有内核服务。
- Chat 页面仍是静态壳，项目、会话和输入控件没有真实数据。
- 现有测试覆盖各包主链，但没有完整插件组装和真实 LLM 的端到端测试。

## 运行前提

- Go 1.25。
- 修改 templ 后运行 `go tool templ generate`。
- 修改样式前运行 `npm install`，修改后运行 `npm run web:build`。
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

- `reference/` 源码被 Git 忽略，只有防止 Go 递归测试的 `reference/go.mod` 入库；换机时参考源码单独复制。
- 换机后执行 `go test ./...` 和 `go vet ./...` 验证环境；这不代表真实聊天已经跑通。
- `~/.harness/config.yaml` 不进仓库，新电脑按上面的格式重新创建。
- 账本、SessionSettings 和自建 Agent 保存在 persist 插件配置的目录，不属于源码。
