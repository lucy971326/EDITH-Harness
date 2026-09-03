# plugins/composers/demo

验证 Chat 输入工具栏插槽 `composer.actions` 边界的演示插件。

## 作用

- 在 `Start` 时 Resolve `chat.Service`，登记 `demo` 输入动作。
- 渲染原生 `<details>` 下拉菜单，提供快捷模版文字插入功能。
- 遵循 `templ + 原生 HTML` 原则，选项全部使用 `type="button"`，零嵌套表单。
