# internal

后台的执行、数据与协调能力。入口显式传入依赖，运行时直接调用。

```text
appserver
  +-> conversations -> runner -> loops / tools / llm
  |                       +-> session / agents / subagents
  +-> 各领域设置与查询

领域自己的文件格式 -> persist -> 本机文件
具体操作系统实现   -> machine/local
```

## 阅读地图

- [`appserver`](appserver/README.md)：协议与连接，内部细节只供接入层使用。

- [`conversations`](conversations/README.md)：会话操作；[`runner`](runner/README.md)：一轮执行与收尾。
- [`session`](session/README.md)：对话账本；[`session/settings`](session/settings/README.md)：下轮设置；[`agents`](agents/README.md)：Agent 设置与准备。
- [`tools`](tools/README.md)、[`loops`](loops/README.md)、[`skills`](skills/README.md)、[`commands`](commands/README.md)：有实际填充者的登记处。
- [`llm`](llm/README.md)：模型调用；[`subagents`](subagents/README.md)：父子会话协作；[`events`](events/README.md)：进程内同步通知。
- [`permissions`](permissions/README.md)：权限计算；[`approvals`](approvals/README.md)：审批；[`hooks`](hooks/README.md)：工具执行前检查。
- [`machine`](machine/README.md)：本机能力契约；[`persist`](persist/README.md)：可靠文件读写。

领域服务不依赖 appserver 或 Client；契约不反向依赖具体实现。跨模块规则见 [设计书](../docs/设计书.md)，数据归属见 [DATA_MODEL](../DATA_MODEL.md)。

## 具体实现

```text
tools/    exec / applypatch / mcp / subagents
loops/    react
skills/   builtin / filesystem
commands/ compact
machine/  local
```

具体实现依赖所属领域的契约，由 `cmd/harness` 构造、登记和关闭。目录嵌套不增加调用层级；构造失败清理与正常关闭责任不变。
