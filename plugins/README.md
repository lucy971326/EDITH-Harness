# 插件地图

插件都静态编译进 Harness；目录表达“它填谁的契约”，不代表动态加载或新的 Host 层级。React Client 不是后台插件。

```text
Host
├─ 内核服务
│  persist → session → llm
│  events / tools / loops / skills / agents / commands / runner / subagents
│
└─ 提供者与登记处填充者
   machine/local       → machine
   loops/react         → loops
   skills/builtin      → skills
   skills/filesystem   → skills
   commands/compact    → commands
   tools/*             → tools
```

插件只负责解析依赖、构造能力并登记；业务实现留在所属包。前端边界见根目录 `WEB_UI.md`。
