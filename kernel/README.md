# kernel

> 可被不同产品复用的后台核心能力。

```text
host       进程服务表与插件生命周期
persist    可靠文件读写
session    对话账本
runner     一轮运行的开始、事件、取消和收尾
agents     组装 Agent 本轮配置
llm        模型登记与流式调用
loops      Agent 执行循环登记处
tools      工具登记处
skills     Skill 发现登记处
commands   平台命令登记处
events     进程内耐久事件发布
subagents  父子会话协作
machine    本机能力契约
```

依赖方向：`products / appserver / plugins → kernel`。kernel 不反向依赖外围。

每个目录的 README 只解释本模块；跨模块稳定规则看根目录 `docs/设计书.md`。
