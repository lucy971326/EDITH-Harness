# Harness Web

正式 React 前端。当前是未接后台的空壳：保留已拍板的布局、Token 和辅助工作区，项目、会话和聊天操作尚未接入。

```sh
cd clients/web
npm ci
npm run dev
```

开发页默认 `http://127.0.0.1:5173`。不要用这个入口代替 `go run ./cmd/harness` 的旧 Web。

```sh
npm run build
```

类型检查后产出 `dist/`。本步不嵌入 Go，也不改根目录构建。

视觉基准仍是 `prototypes/web`，请对照那个原型，不要把本目录里的空态当成产品完成。
