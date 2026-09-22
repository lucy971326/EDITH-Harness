# apply_patch

【角色】把模型给出的 Codex 格式补丁转换为本机文件修改。

【提供能力】向 `tools` 登记 `apply_patch`，支持新增、修改和删除文件。

【依赖】通过 `machine.FileSystem` 读取版本与解析路径；通过 `machine.AgentFiles` 携带可信 Policy 批量提交。实际写入由 machine 落实沙箱与版本检查。

【边界】本包负责补丁语法、上下文匹配、换行保留、并发修改保护和已提交变化；Turn Diff、账本与界面由调用方管理。
