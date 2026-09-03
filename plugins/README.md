# 插件地图

插件都静态编译进 Harness；目录表达“它填谁的契约”，不代表动态加载或新的 Host 层级。

```text
Host
├─ 内核默认插件
│  persist → session → llm
│  events / tools / loops / skills / agents / runner
│
├─ 内核提供者与填充者
│  machine-local → machine
│  react         → loops
│  read/write/edit/bash → tools
│
└─ Web
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

先读本文件定位，再读目标插件 README；只有需要实现时才进入代码。
