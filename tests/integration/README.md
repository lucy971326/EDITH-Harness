# integration

跨层网络验收：真实服务与 RPC，模型使用本机 HTTP 替身，数据使用临时目录。

```text
TS 验收 Client → appserver → conversations → Runner → ReAct
                                                   → 本机模型替身
```

`network_test.go` 显式组装服务，验证连接、发送、停止及重启等路径；不读取用户会话。普通领域单测留在源码旁。

仓库根目录运行 `npm --prefix clients run rpc:test`；`make agent-check` 的 Go 测试也会覆盖此包。无新增测试框架。
