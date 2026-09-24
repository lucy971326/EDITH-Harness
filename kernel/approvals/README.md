# approvals

管理一次操作的批准、拒绝与等待；批准只改变本次权限。

```text
Tool -> Authorize -> permissions 比较
                         +-> 已允许
                         +-> 人工 / LLM / Jev -> 本次 Policy
                                            -> machine 执行
```

## 从哪里读

- `types.go / service.go`：申请身份、待审批表、回答、订阅与取消。
- `context.go`：从账本追溯真实用户授权，委派说明不冒充授权。
- `settings.go`：全局审核设置；`review.go / jev.go`：两种智能审核。
- `mcp.go`：项目配置版本信任与 MCP 调用审批。

信息不足或智能审核失败转人工；停止取消等待。Jev 使用 `jev-1.13.0`，批准置信度门槛为 **0.5**，以 `jev.go` 常量为准。待审批只在内存，订阅发送最新完整列表，断线不取消 Run。

审核设置在 `~/.harness/approvals/settings.json`；Jev 密钥读取 `~/.harness/config.yaml` 的 `jev.apiKey`，修改后重启，密钥不返回前端。项目 MCP 信任单独保存；普通批准不落账、不执行命令或 Hook。
