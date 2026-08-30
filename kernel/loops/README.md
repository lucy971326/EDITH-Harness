# loops

【它是什么】运行范式登记处插件。

【提供能力】注册登记处服务 `loops`。

【使用能力】无。

【填充插槽】自身不填；React、Graph 等插件来登记。

## 代码主干

```text
Register(Loop) → 按 Kind 登记
Get(Kind)      → Runner 取得一种 Loop
Loop.Run       → 消费 Invocation，产出 Event
Checkpoint     → 安全位置接收 Steer
```

登记的是可复用程序；正在运行的活 Run 在 Runner 中。
