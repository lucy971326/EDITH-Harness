# Client UI 规范

本篇描述目标 React Client。`surface/web`、`plugins/web`、Templ、HTMX、POST 与 SSE 只是待清理的旧入口；当前可运行范围见 `STATUS.md`，不要把它们当作新页面的长期基础。

## 1. 调用边界

```text
React 组件
   ↓ 调用类型化 Client 方法
普通 TypeScript：状态、Snapshot、订阅与重连
   ↓
WebSocket / 标准 JSON-RPC 2.0
   ↓
app-server → Product / 公共服务 → kernel
```

- HTTP 只提供 Vite 构建后的静态资源。
- 所有业务调用、通知、订阅和服务端反向请求走同一条 WebSocket。
- React 负责显示和局部交互；普通 TypeScript 负责连接、协议、状态归并与恢复。
- Client 共用 `clients/contracts/` 中手写的 TypeScript 契约；接口修改时同步 Go 与 TS，不自动生成。
- UI 不直接拼 JSON-RPC 封套，不知道 Go Host、Runner 或 Product 的具体实现。
- Wails 首版承载同一套前端并连接同一 WebSocket，不另写一套 IPC 业务层。

## 2. 状态边界

```text
后台事实        Session、SessionSettings、Agent、Product 状态、Run 状态
Client 投影     从 Snapshot + 后续事件得到的当前画面
Client 临时状态 当前路由、选择、折叠、面板宽度、主题、输入草稿
```

Client 不写账本，不把本地状态冒充业务事实。刷新可丢失的界面状态可保存在内存或浏览器本地存储；需要跨端一致的事实必须由后台拥有。

运行详情采用“先建立订阅边界，再取得 Snapshot，最后应用边界后的事件”或等价无空档方案。重连时重新初始化、恢复 Snapshot 和待回答请求，再续订；不要求重放每个文字 Delta，但最终投影不能漏耐久事实。

`harness/session/subscribe` 把订阅 ID 与初始 Snapshot 一起返回，随后发送 `harness/run/event` 通知。Client 先采用 Snapshot，再应用 `updateSeq` 大于快照边界且 `seqEpoch` 相同的通知；重叠耐久消息按 Entry.ID 去重。生成中的草稿与实时增量、最终 Entry 共用 Entry.ID。`clients/test` 仅用于协议验收，不作为 React 页面或正式 SDK 的目录模板。

App-server 的运行订阅负责把内核并发通知排成连续序号，Client 不再造一套乱序队列。输出位置以消息的 `afterSeq`／草稿的 `afterEntrySeq` 为准，不把本轮 Run 的起点套给插话后的所有回答。

- 直接以 Snapshot 的 `entries + runs/drafts` 作为聊天投影；实时更新同一份数据，完整 Entry 替换同 ID 草稿。快照省略空草稿时，首个增量仍可建立它；已落账的 ID 不被迟到增量重建。
- 只订阅选中会话；切设置不断订阅，切会话解除旧订阅。迟到的旧订阅响应也要解除，不能覆盖新选择。订阅响应处理时同步安装 Snapshot，随后才接受通知；失败不是空历史。
- 序号重复忽略，缺口或 epoch 变化重新订阅，不排队猜顺序。恢复完成前禁用发送和停止。
- 自动重连间隔为 1／2／4／8／10 秒，之后上限 10 秒；手动重连替换旧尝试，卸载清理定时器和连接。只恢复读取，不重发业务操作。
- 选中会话 ID 存本标签页 `sessionStorage`，刷新后恢复订阅；首次不自动选中，只在明确不存在时清除。输入草稿和压缩后的图片预览按会话保存在内存，刷新不承诺恢复。

请求 ID 只匹配响应。发送、回答等有副作用操作如需安全重试，必须使用后台定义的业务操作 ID，不能拿 JSON-RPC `id` 代替。

未知通知可忽略；服务端反向请求必须明确回答“不支持”或交给支持它的 Client，不能静默吞掉。慢 Client 断开或丢弃增量，不得反压 Runner。

## 3. 前端代码组织

目标目录在迁移时建立为 `clients/web/`：

```text
clients/web/
├─ src/client/       JSON-RPC 连接、手写契约适配、订阅与恢复
├─ src/state/        后台投影 reducer 与页面级状态
├─ src/components/   跨产品基础组件
├─ src/products/     Harness 等产品界面
├─ src/styles/       Token、主题与公共语义样式
└─ src/icons/        受控图标入口
```

- 不使用 Next.js，不引入服务端 React。
- TypeScript 写法直白、类型明确，少语法糖和高级类型技巧。
- 没有真实复用前，不建立通用 Store、组件框架或插件化 UI。
- 聊天阅读路线：`src/client/rpc.ts` 收发 → `src/client/chat.ts` 订阅／恢复 → `src/state/chat.ts` 统一投影 → `src/chat-messages.tsx` 渲染。`App` 保留连接、会话选择、草稿和发送／停止流程；`sidebar.tsx` 与 `composer.tsx` 只展示并回调，不发 RPC。不为符合目标目录图预建空壳。
- 后台插件传数据，不传 HTML、React 组件或任意 SVG。
- 目标架构不保留旧页面插槽；产品内部扩展由真实需求再设计，不迁移 demo 插槽。

## 4. 视觉系统

保留现有已经验证的视觉方向，换实现不换体验：

```text
暖白工作台 + 石墨文字 + 极淡分割线
黑色用于主要操作
蓝色只用于链接与焦点
少用卡片与普通阴影；卡片必须表达真实分组
```

Token 是颜色、排版、间距、圆角、边框、阴影、动画和主题的唯一视觉事实来源。亮色、暗色、跟随系统只切换 Token；产品不得各写一份 dark 样式。

公共组件只承载跨产品重复的基础元素：按钮、输入框、选择框、导航、文字层级、状态、菜单、提示、空状态。Chat 的 Composer、运行过程、消息动作等产品事实留在 Harness 产品界面。

正式 UI 使用受控图标，不用 Emoji 充当图标，不接受后台传入的任意 SVG。缺少图标时先扩充 Client 的静态图标集合。

## 5. Chat 与运行投影

- 运行时工作过程展开，正常完成自动收起，最终回答留在过程外；失败与停止保持过程展开。工具组和单个详情默认收起，按三级结构查看，长内容限高滚动。具体迁移交互见 `docs/plan/ProductDefine.md`。
- Snapshot 与实时事件进入同一个 reducer；内容定位为 SessionID → RunID → EntryID → BlockSeq。工具结果按 ToolCall.ID 回填，Steer 分段看 afterSeq，不能按 ID 或落账 Seq 让生成中的内容跳位。
- 只有已落账且未标 incomplete 的最终回答能成为最终结果；运行中的临时正文和半截保存不能冒充完成。
- 本轮最后一条助手消息满足完整正文、无工具调用／压缩摘要且运行成功，才将 text 移到过程外；按落账次序选最后一条，不向前借旧说明。afterSeq 必须覆盖本轮最后输入，防止 Steer 后空输出误用旧回答。`runs.json` 引入前的旧历史没有运行状态时，只兼容外置已落账、完整且结构上合格的末条助手正文；工具状态仍显示“状态未记录”。无 RunID 的旧条目不伪造轮次。
- 展示由 Snapshot 只读派生；工具按同一 Run 内的 ToolCall.ID 回填。未返回结果显示“等待结果”，不推断正在执行；结束仍无结果标“结果未记录”，未知运行状态则标“状态未记录”。
- 子任务回报、压缩摘要在过程内提供折叠入口；纯问答没有其他过程内容时不画空过程壳。普通增量不覆盖手动折叠选择。
- Markdown 只用于用户消息和最终回答，渲染后必须清洗；推理、工具参数和结果默认按纯文本显示。
- 图片、模型、思考档位、Agent、命令和 Skill 候选都通过类型化接口取得，不从旧页面 HTML 中解析。Agent 公共服务由 appserver 直接调用，不经 Product 转发。
- 关闭窗口或断开连接不停止后台任务；只有明确 Stop 才取消。
- 输入确认后才清掉本次提交的文字和图片；提交期间的新编辑和其他会话草稿不能被迟到确认清除。等待确认期间禁止重复提交；超时／断线保留草稿并提示核对历史，不按正文猜测是否接受。
- 忙时携带 `expectedRunID` 直接插话，无等待队列；原轮已结束／关闭插话／换轮返回冲突，文字留下，由用户确认后再次发送。停止响应只是已接收请求，界面等 Snapshot／结束事件，不把抢先成功改成取消。
- Agent 选择后立即保存；模型与思考合用一个菜单，组成有效组合后立即保存。已保存的 SessionSettings 是界面真相，`send` 不重复携带设置；运行中前后端都禁改。标题和设置不在每条 Delta 上重复拉取。
- 图片选择或粘贴后由浏览器压为 WebP：最长边 2048px、单张不超过 2MB、最多 4 张，GIF 只取首帧。压缩期间禁发；模型不支持图片时不能附加或切换过去。后台仍重新校验，不能信任 Client。
- 最近一次模型调用的真实上下文用量来自当前 Snapshot 的 RunState，显示 `inputTokens + cacheReadTokens`，入口位于模型／思考左侧；不维护第二份前端用量状态。
- Enter 提交、Shift＋Enter 换行，输入法选词不提交；向上阅读时暂停自动滚动。

## 6. 可访问性与验收

新页面至少检查：

```text
[ ] 业务调用是否只经过类型化 Client？
[ ] Snapshot 与事件之间是否无空档，重连后是否恢复一致？
[ ] 是否把业务事实误存进 React / 浏览器状态？
[ ] 是否只使用 Token、公共基础组件和受控图标？
[ ] 是否支持亮色、暗色、窄桌面、键盘焦点与减少动态效果？
[ ] 慢连接、断线和未知通知是否不会阻塞后台？
[ ] 是否保留结论优先、工具配对、Markdown 清洗和 Stop 语义？
```

迁移完成前，旧 Web 只做必要修复；迁移完成后整体删除旧 Templ / HTMX / POST / SSE 页面链和演示插槽，不维护双轨。
