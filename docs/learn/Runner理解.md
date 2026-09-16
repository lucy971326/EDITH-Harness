# Runner 心智模型

厨师只管炒；开桌、写账、加菜、停火，全由 Runner 拦住。

一张桌（Session）同一时间只能炒一盘菜。不同桌可以同时炒。认的是 SessionID。

```text
Runner  总管的工具间
│
├─ live[桌子ID] ──────────────────────────────────────┐
│                                                     │
│     liveRun  这张桌上正在炒的那盘菜                    │
│     ├─ runID            单号                        │
│     ├─ cancel           停火开关                    │
│     ├─ steeringState    窗口：关着 / 开着收加菜 / 打烊 │
│     ├─ pendingInputs    托盘（加菜纸条在这等）         │
│     ├─ drafts           白板（正在蹦字，还没誊账本）    │
│     └─ afterEntrySeq    这盘菜从账本哪一行起做         │
│                                                     │
├─ sessions   记账本  ◄── 跑的时候用这笔往里写 ─────────┘
├─ persist    工单柜 runs.json（成功 / 失败 / 取消 / 中断）
├─ events     传菜铃（通知屏幕：新字、工具、整轮结束）
│
└─ 开菜前准备厨师
   ├─ settings   这张桌怎么跑（模型、Agent、工作区）
   ├─ agents     拼系统提示词和工具名单
   └─ loops      按 Kind 拿食谱（现在是 react）
```

账本是历史，`liveRun` 是这盘菜还在炒时桌上那些活的道具。
