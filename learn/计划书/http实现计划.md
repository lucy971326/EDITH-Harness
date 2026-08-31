# 重写 kernel/http

## 总结

插件只在启动时登记，配置变化靠重启。HTTP 直接使用 `ServeMux`，不保存路由副本，不做单条注销。

## 核心契约

```go
type Config struct {
    Host string
    Port int
}

func (r *Registry) Register(pattern string, handler nethttp.Handler) error
```

- Handler 使用结构体实现 `ServeHTTP`。
- Host 只允许 `127.0.0.1` 和 `0.0.0.0`；端口 `0` 由系统分配。
- 使用标准 `ServeMux` 路由语法；冲突与非法模式返回错误。
- Host 上的 `http` 服务是 `*http.Registry`，只提供路由登记。
- `listener` 只是 `Start` 的局部变量；Plugin 只保存配置和 `net/http.Server`。
- `Plugin.Start` 绑定端口并启动；`Close` 立即关闭。
- 本阶段不实现 pages、Web UI、SSE Handler、鉴权、TLS 和中间件。

## 静态登记

- Tools、Loops、Skills 的 `Register` 只返回 error。
- 删除 React、read、write、edit、bash 插件的 `unregister` 状态。
- Events 监听者会在运行中离场，继续返回注销函数。

## 测试

- 路径、方法和路径参数分发。
- 非法、重复、冲突路由不污染登记处。
- 并发请求与登记安全。
- 端口绑定、Host 服务挂载和关闭行为。
- Tools、Loops、Skills 登记、重复拒绝、读取与调用。
- `go test -race ./kernel/http ./kernel/tools ./kernel/loops ./kernel/skills`、`go test ./...`、`go vet ./...`。
