# clients

一份 React UI、一份构建产物；Web 与 Desktop 各自提供接入能力。

```text
main.tsx -> 选择平台 -> ui/src/App
                       |
          openSocket / openExternal
           |                   |
   web/platform.ts     desktop/platform.ts
      WebSocket           Wails Stream
           |                   |
      web/*.go          desktop/wails.go
           +---------+---------+
              appserver.ServeStream
```

- [`ui/`](ui/README.md)：共用页面、草稿、RPC、订阅和服务端投影。
- [`web/`](web/README.md)：浏览器能力、HTTP 与 WebSocket 传输。
- [`desktop/`](desktop/README.md)：Wails 能力与 Go 传输；窗口入口仍在 `cmd/harness-desktop`。
- [`contracts/`](contracts/README.md)：手写 TS 契约；[`test/`](test/README.md)：无界面网络验收 Client。
- `package.json / package-lock.json`：唯一前端依赖清单与锁文件；`vite.config.ts` 构建 `dist/`，`assets.go` 将其嵌入两端。

业务事实归服务端。共用 UI 不判断平台、不导入两端适配器；平台只提供当前需要的连接与打开网址能力，不保存会话状态。
