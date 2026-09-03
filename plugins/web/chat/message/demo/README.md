# plugins/web/chat/message/demo

Chat 的 `message.actions` 插槽的目录形状示例。

它展示独立消息动作插件的最小职责：在 `Start` 时 Resolve `chat.Service`，然后登记一个实现 `chat.MessageAction` 的动作。

此插件是占位，不安装进 `cmd/harness`，没有用户可用的功能。未来需要独立的消息动作时，在 `message/<name>/` 新建插件并替换这里的占位即可。
