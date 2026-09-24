# kernel

后台的执行、数据与协调能力。入口显式传入依赖，运行时直接调用。

```text
appserver
  +-> conversations -> runner -> loops / tools / llm
  |                       +-> session / agents / subagents
  +-> 各领域设置与查询

领域自己的文件格式 -> persist -> 本机文件
具体操作系统实现   -> plugins/machine/local
```

## 阅读地图

- [`conversations`](conversations/README.md)：会话操作；[`runner`](runner/README.md)：一轮执行与收尾。
- [`session`](session/README.md)：对话账本；[`session/settings`](session/settings/README.md)：下轮设置；[`agents`](agents/README.md)：Agent 设置与准备。
- [`tools`](tools/README.md)、[`loops`](loops/README.md)、[`skills`](skills/README.md)、[`commands`](commands/README.md)：有实际填充者的登记处。
- [`llm`](llm/README.md)：模型调用；[`subagents`](subagents/README.md)：父子会话协作；[`events`](events/README.md)：进程内同步通知。
- [`permissions`](permissions/README.md)：权限计算；[`approvals`](approvals/README.md)：审批；[`hooks`](hooks/README.md)：工具执行前检查。
- [`machine`](machine/README.md)：本机能力契约；[`persist`](persist/README.md)：可靠文件读写。

kernel 不依赖 appserver、plugins 或 Client。跨模块规则见 [设计书](../docs/设计书.md)，数据归属见 [DATA_MODEL](../DATA_MODEL.md)。
