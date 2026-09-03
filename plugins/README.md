# plugins

这里放静态编译进 Harness 的具体插件，按它们填充的契约所有者归档：

```text
plugins/
├─ kernel/   内核服务的提供者、内核登记处的填充者
└─ web/      Web 产品及其页面插槽填充物
```

- `kernel/machine/local` 提供 `machine` 服务。
- `kernel/loops/react`、`kernel/tools/*` 填内核登记处。
- `web/chat` 是填入 Web products 登记处的 Chat 产品。
- `web/chat/<slot>/*` 填 Chat 自己的页面插槽。
- `web/settings/*` 填 Web 公共 `settings.section`。

目录只表达契约归属，不增加 Host 层级，也不代表动态加载。TUI、ACP 等真实实现出现后再建立对应目录。
