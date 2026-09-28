# Site frontend

EDITH Harness 网站的 Next.js 前端；与本机 Harness UI 分开构建。身份接入由 Clerk CLI 生成，当前绑定开发应用。

网站视觉参照 [EDITH 视觉规范](../../docs/AGENTS/VISUAL_DESIGN.md)；网站布局与组件在本前端实现，不复制应用 CSS。

```powershell
npm install
npm run dev
```

本地打开 `http://localhost:3000`。首次配置在本目录执行 `clerk auth login` 和 `clerk init --app <Clerk 应用 ID>`；CLI 写入的 `.env.local` 不提交。检查使用 `npm run lint`、`npm run build` 与 `clerk doctor`。

账号控制台 `/account` 在页面上执行 Clerk 登录保护。云端 Go 接口与设备连接尚未接入。
