# config

一句话：**配置插件**，普通配置进抽屉柜，钥匙进保险柜。

```text
DeepSeek 等能力插件
        │ 登记自己的抽屉 / 领钥匙
        ▼
      config
        ├─ settings      config.yaml 的 settings:
        └─ credentials   credentials.yaml
```

- 提供能力：`"settings"`、`"credentials"`
- 领取能力：无
- 填充插槽：暂不填充插槽
- 先读：`plugin.go` → `service.go` → `settings.go` → `credentials.go`
