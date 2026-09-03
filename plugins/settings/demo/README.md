# plugins/settings/demo

验证 Web 公共设置插槽 `settings.section` 的独立演示插件。

## 职责

- 在 `Start` 时 Resolve `web.Service`。
- 向 Web 登记 `demo` 设置栏目（`ID: demo`、`Title: 演示设置`、`Order: 10`）。
- 注册 `POST /settings/demo` 路由处理表单保存。

## 状态归属说明

- **仅限内存**：本演示插件的状态（用户昵称）完全保存在本插件结构体的内存字段中。
- **不污染核心**：状态不写入 Session，不创建通用 Store 服务，也不与任何外部业务耦合。
- **重启后重置**：服务重启后，配置将自动恢复为初始默认值（`Explorer`）。
