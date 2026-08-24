# M3 计划书 —— llm + tools（大白话版）

> llm 管"问模型"，tools 管"模型用工具"。M3 只造规矩，不造真货：
> 真模型适配器、真工具全是将来的插件，测试用假的 + 真实响应样本。

## llm 包 = 插座排

```
loop / 压缩插件（要用模型的）
      ↓ 只认识插座排，从不 import deepseek
  ┌─ llm 包 ──────────────┐
  │ ① 统一的对话格式（消息、流式分片）│  ← 全系统说同一种话
  │ ② 适配器登记表                  │  ← Request.Provider → "deepseek" 适配器
  │ ③ 报错统一                      │  ← 适配器随便抛错，出口统一成一种样子
  └───────────────────────┘
      ↑ 将来插上（全是插件）：deepseek / openai / 假适配器(测试)
```

**对话格式用 M2 的 `chat/` 公共词汇，llm 不另造一套**（终审修正）；流式增量类型也放 chat/，别让账本反过来依赖 llm。

请求必须明确写 `Provider`、`Model`、`Thinking`，插座排不再有默认服务商，也不需要 `StreamWith` / `CompleteWith`。`Complete` 只是 `Stream(..., nil)` 的便捷入口。DeepSeek 的 Thinking 只认 `off`、`high`、`max`；其他值在出网前报 `bad_request`。

M6 的真适配器在 `llmplugins/deepseek/`：只用官方 `github.com/openai/openai-go/v3` 的 Chat Completions 流，显式传配置中的 API key 和 BaseURL。DeepSeek 扩展的 `reasoning_effort` / `reasoning_content` 经 SDK 的 ExtraFields 和原始扩展字段处理，不手写 HTTP 或 SSE。

测试要加一份**真实响应样本**：OpenAI、Anthropic 各存几份真实流式响应，喂给解析层——防止我们闭门造车把格式想歪。

## tools 包 = 一条流水线

模型说"调 bash"之后：

```
① 先记账"要调了"    ← 先记后干：崩了也知道有这回事
② 前置检查链        ← 插件地盘：放行 / 拒 / 要问人
③ 问人             ← 问了没人答 = 拒
④ 守卫             ← 只能拒，不能翻案
⑤ 环绕链           ← 超时/重试挂这
⑥ 执行工具本体
⑦ 后置检查链        ← 可改结果 / 附话
⑧ 出错全变成正常错误返回（进程不崩）
⑨ 记账"结果"        ← 终局入账
```

**顺序从此冻结，改顺序 = 破坏性变更。**

记账主权：tool/call、tool/start、tool/result 由 tools **独占**记录（loop 不重复记，否则模型看见两次调用）。

**执行开始边界（红队终审）**：真正开始执行前先记 `tool/start`——"开没开跑"必须持久化，否则重启后无法区分 skipped 和未知，只能瞎猜。恢复规则：**无 start → `tool/result{status:skipped}`（不另发明 Kind）；有 start 无 result → 结果未知，进待裁决**；裁决出口必须落成 tool/result（成功/失败/跳过 三选一）后才允许继续，禁止"无 result 继续"（五轮缺件A）。tools 拿 session 的窄写入口记这三类，插件不能冒写（API 强制）。**start 写在守卫/问人之后、本体之前，每个 call 只写一次**——写早了，被拒批的调用也会变成副作用窗口。

登记处：工具报名（名字 / 参数说明 / 执行函数），复用 M1 的两层 map——agent 可用自己的同名版本盖掉全局的。

## 两条铁律

- **参数不可改**：记了账的调用，谁都不许改参数；
- **先记后干**：执行中崩溃 = 账上"有调用无结果" = 结果未知，**禁止自动重试**（邮件可能已发出）。

## 验收（最硬的五条）

1. 假适配器抛错 → 消费方只见统一错误样；
2. 守卫拒了 → 前置链再怎么放行也翻不回；
3. 问人没人答 = 拒；
4. 工具 panic → 返回错误，进程不崩；
5. 真实响应样本 → 解析 → 无损还原；
6. tool/call 只记一次（tools 独占）；tool/start 先于执行落盘；已开跑被取消 → 无 result 而非 skipped；非 tools 的插件冒写 tool/* 被窄写入口拒绝。
