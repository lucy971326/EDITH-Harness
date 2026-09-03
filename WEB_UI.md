# Web UI 规范

面向后续维护 Harness Web 表面的人与 AI。目标不是做一批相似页面，而是让所有 Web 产品和页面插槽共享一套稳定、克制、容易维护的视觉语言。

## 总原则

```text
统一视觉语言
→ 在确实重复的地方复用
→ 降低维护成本
```

不要为了复用，把产品独有的工作流硬抽成全局组件。

```text
应统一：按钮、输入框、导航、文字层级、状态色、主题、图标
各自拥有：Chat 工作过程、Movie 播放控制、某个插件的专属内容
```

## 三层设计系统

```text
surface/web
├─ Token
│  颜色、字体、字号、间距、圆角、边框、阴影、动画、主题
├─ 公共 UI 规则
│  ui-button-*、ui-input、ui-select、ui-nav-item、ui-card、
│  ui-text-*、ui-menu、ui-notice、ui-empty-state …
└─ 公共图标入口
   ui.Icon(ui.IconSettings)、ui.Icon(ui.IconPlus) …
```

### 1. Token

来源：`surface/web/assets/tokens.css`。

Token 是唯一的视觉事实来源。颜色、排版、间距、圆角与主题都在这里定义。

```text
禁止：产品或插槽自己写固定颜色、任意字号、任意圆角
允许：从现有 Token 选择；若确实缺少一类全局规则，再补 Token
```

亮色、暗色、跟随系统只切换 Token；产品与填充物不得各写一份 dark CSS。

### 2. 公共 UI 规则

来源：`surface/web/assets/input.css`。

`ui-*` 类将 Token 组合成可复用的界面规则。新增 Web 产品或插槽时，先选用已有规则：

```text
文字：ui-text-title / ui-text-section / ui-text-body / ui-text-meta / ui-text-code
操作：ui-button-primary / ui-button-secondary / ui-button-danger / ui-icon-button
表单：ui-input / ui-select / ui-textarea
容器：ui-page / ui-panel / ui-card / ui-menu / ui-notice / ui-empty-state
导航：ui-nav-item / ui-tab
```

只在一类元素会跨产品重复出现时，才新增公共 `ui-*` 规则；仅属于一个产品的内容，写在该产品自己的语义类中。

```text
正确：ui-workflow 属于 Chat，留在 Chat 专属规则
正确：ui-button-primary 属于所有 Web 产品，留在公共规则
错误：为了未来假设需求，把 Chat 工作过程做成全局组件
```

## 图标

来源：`surface/web/ui/icons.templ`。

Web 自身、Web 产品和页面插槽一律使用受控图标：

```go
@ui.Icon(ui.IconSettings)
@ui.Icon(ui.IconPlus)
```

不在正式 UI 使用 Emoji 充当图标，不从运行时读取 SVG，也不让插件传入任意 SVG 或图标字符串。

若现有图标不够，先在 `surface/web/ui` 增加一个静态编译的图标常量，再由各处使用。

## JavaScript 边界

优先级固定：

```text
静态页面 / 普通表单 / 局部刷新 → templ + 原生 HTML + HTMX
浏览器独有且服务端做不到的职责 → 少量 JavaScript
```

JS 仅可处理：

```text
SSE 实时投影
浏览器临时状态
拖拽、复制、输入填充
```

不得把业务状态、Session 状态或插件状态放进 JS；不引入前端状态管理框架。新增 JS 前，先说明 HTMX / 原生 HTML 无法完成的原因，并征得用户同意。

## 当前视觉方向

```text
暖白工作台 + 石墨文字 + 极淡分割线
黑色用于主要操作
蓝色只用于链接与焦点
少用卡片与普通阴影；卡片必须表达真实分组关系
```

保持桌面端信息密度。工作过程默认收起，工具默认是一行摘要；只有用户展开时才显示详细容器。

## 新页面或新插槽检查表

```text
[ ] 是否只使用 Token 与已有 ui-* 规则？
[ ] 是否使用 ui.Icon，而非 Emoji 或私有 SVG？
[ ] 是否同时检查亮色、暗色与窄桌面？
[ ] 是否保留键盘焦点与原生语义？
[ ] 是否避免无意义卡片、阴影、颜色与动画？
[ ] 是否能用 templ + HTMX 完成，而不是新增 JS？
```
