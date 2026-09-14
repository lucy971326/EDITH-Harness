# machine-local

【它是什么】本机 `machine` 服务提供者插件。

【使用能力】不 Resolve 其他服务。

【提供能力】注册服务 `machine`：取得主目录、读写文件、版本保存、目录与元数据、文件监听、路径解析和本机进程。

【填充插槽】不填。

【谁在用】`read`、`write`、`edit`、`bash` Tool 与 appserver 文件 RPC 调用它完成真实操作。

【不做】不决定哪些 Tool 可用，也不限制本机文件访问范围。
