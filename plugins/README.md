# 插件地图

插件都静态编译进 Harness；目录表达“它填谁的契约”，不代表动态加载或新的 Host 层级。

下图记录当前组装事实。其中 Web 一支是迁移期旧实现；目标 React Client 不再作为后台页面插件，也不保留页面插槽。

```text
Host
├─ 内核默认插件
│  persist → session → llm
│  events / tools / loops / skills / agents / runner
│
├─ 内核提供者与填充者
│  machine-local → machine
│  react         → loops
│  skills/filesystem → skills
│  read/write/edit/bash → tools
│
└─ 迁移期旧 Web
   surface/web   → web（产品、路由、settings.section）
   chat          → web products + chat（四个 Chat 内部登记处）
   web/demo      → web products（演示产品）
   settings/demo → settings.section（演示填充者）
```

## 怎么读一个插件

每个非 demo 插件 README 都用同一套五问：

```text
【它是什么】   这块的职责
【使用能力】   Resolve 哪项服务、调用什么
【提供能力】   注册哪项服务或提供什么条目
【填充插槽】   向哪个登记处 Register
【谁在用】     哪个插件或浏览器实际消费它
```

内核插件仍按本图定位。旧 Web 只做必要修复；新 Client 规则见根目录 `WEB_UI.md`。
