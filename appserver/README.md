# appserver

> Harness 的网络前台：验明请求，交给真正负责业务的人，再把结果送回去。

```text
Client → WebSocket → clientconn → rpc.Registry → 具名 Handler
                                                ├→ HarnessProduct
                                                └→ 公共内核服务
```

## 三块基础设施

```text
internal/rpc/              方法登记、Schema 校验、Session 写请求排队
internal/clientconn/       一个 Client 的初始化、通知、订阅、断线清理
internal/workspacepicker/  macOS / Linux / Windows 原生目录选择
```

它们不拥有 Session、Run、设置或账本。

## 根目录怎么读

```text
server.go         唯一 Server 与方法调用入口
listener.go       HTTP 服务启动、关闭
lifecycle.go      整个 Server 的准入与收尾
websocket.go      静态页面 /rpc 分流与 WebSocket 适配
harness*.go       Harness 产品接口
agents.go 等      公共服务接口
filesystem.go     文件读取、版本保存、目录、元数据与监听接口
workspace.go      目录选择的网络接口
```

## 并发只记三句话

```text
同一 Session 写请求：排队
不同 Session：        并行
Stop：                直接执行
```

- Connection 断开只清自己的网络资源，不停止已接受的 Run。
- 每个订阅只在自己的响应写出后开放通知。
- 慢 Client 直接断开，不能反压 Runner。
