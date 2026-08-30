# tools

【它是什么】Tool 登记处插件。

【提供能力】注册登记处服务 `tools`。

【使用能力】无。

【填充插槽】自身不填；具体 Tool 插件来登记。

## 代码主干

```text
tools.New(Args + tags)
  → 自动生成 JSON Schema

Register
  → 编译 Schema 并按名称登记

Call
  → 校验 Allow 名单
  → 校验 JSON 与 Schema
  → 转成 Args
  → 调具体 Tool
```

登记处负责注册、授权和校验；不直接操作 machine。
