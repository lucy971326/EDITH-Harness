# plugins

静态编译的具体能力实现，由 `cmd/harness` 显式构造、登记和关闭。

```text
machine/local     -> kernel/machine 契约
loops/react       -> loops 登记处
skills/*          -> skills 登记处
commands/compact  -> commands 登记处
tools/*           -> tools 登记处
```

目录表示实现哪种能力，不再有统一的 Start / Resolve / Close 插件外壳。需要长期资源的实现仍负责 Close，由入口安排调用。

先看各目录 README，再看 `New / Register` 入口；执行规则属于具体实现，共用契约属于 kernel。
