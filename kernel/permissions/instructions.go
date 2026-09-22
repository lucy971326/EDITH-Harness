package permissions

import (
	"fmt"
	"strings"
)

// Instructions 描述当前实际权限；参数申请从不等于授权。
func Instructions(policy Policy, reviewer ReviewerKind) string {
	var text strings.Builder
	text.WriteString("[Harness 运行环境更新：当前执行权限；替代此前的权限说明]\n")
	if policy.Unrestricted {
		text.WriteString("当前为完全访问模式，不添加 Agent 文件或网络沙箱限制。\n")
	} else {
		text.WriteString("可以读取宿主当前用户可读文件；仅以下目录可写（各根的 .git、.agents、.harness 默认受保护）：\n")
		for _, root := range policy.WriteRoots {
			fmt.Fprintf(&text, "- %s\n", root)
		}
		fmt.Fprintf(&text, "当前是否允许联网：%t。\n", policy.Network)
	}
	switch reviewer {
	case HumanReviewer:
		text.WriteString("额外权限由用户审批。\n")
	case ModelReviewer:
		text.WriteString("额外权限由已配置的智能审核器审批；拒绝则不执行，不确定或审核不可用时转人工审批。现有权限内的操作不触发智能审核。\n")
	}
	if reviewer == HumanReviewer || reviewer == ModelReviewer {
		text.WriteString("exec_command 可随命令填写 extra_permissions（writeRoots 为已有的绝对目录，network 为是否需要联网）和 justification。可以在首次执行前申请；未申请的命令按现有权限执行。沙箱失败后按需要重新申请，不盲目重复已产生副作用的命令。apply_patch 根据目标路径自动申请目录权限；尚不存在的受保护目录需要用户先创建，不能通过申请父目录解除保护。批准仅用于本次操作，不修改会话模式；已有进程保留启动权限。\n")
	}
	text.WriteString("这些沙箱限制覆盖本机命令和补丁工具；其他工具不能据此假定受到相同隔离。")
	return text.String()
}
