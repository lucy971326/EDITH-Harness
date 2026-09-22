package permissions

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

var protectedNames = [...]string{".git", ".agents", ".harness"}

// Modes 返回独立的模式清单，智能审批等待模型审核接入。
func Modes() []ModeChoice {
	return []ModeChoice{
		{ID: ReadOnly, Label: "只读", Available: true},
		{ID: AskForApproval, Label: "请求批准", Available: true},
		{ID: ApproveForMe, Label: "智能审批", Available: false},
		{ID: FullAccess, Label: "完全访问", Available: true},
	}
}

// NormalizeMode 为旧设置补默认值，拒绝未知模式。
func NormalizeMode(mode Mode) (Mode, error) {
	switch mode {
	case "":
		return AskForApproval, nil
	case ReadOnly, AskForApproval, ApproveForMe, FullAccess:
		return mode, nil
	default:
		return "", fmt.Errorf("permissions: unknown mode %q", mode)
	}
}

// BuildPolicy 将模式和可信运行位置翻译为权限，不读取环境变量或访问文件系统。
func BuildPolicy(mode Mode, workspace string, tempDirs []string) (Policy, ReviewerKind, error) {
	mode, err := NormalizeMode(mode)
	if err != nil {
		return Policy{}, NoReviewer, err
	}
	err = validatePaths([]string{workspace})
	if err != nil {
		return Policy{}, NoReviewer, err
	}
	err = validatePaths(tempDirs)
	if err != nil {
		return Policy{}, NoReviewer, err
	}
	if mode == FullAccess {
		return Policy{Unrestricted: true, Network: true}, NoReviewer, nil
	}
	if mode == ReadOnly {
		return Policy{}, HumanReviewer, nil
	}

	policy := Policy{WriteRoots: []string{workspace}}
	for _, dir := range tempDirs {
		if !slices.Contains(policy.WriteRoots, dir) {
			policy.WriteRoots = append(policy.WriteRoots, dir)
		}
	}
	reviewer := HumanReviewer
	if mode == ApproveForMe {
		reviewer = ModelReviewer
	}
	return policy, reviewer, nil
}

// Evaluate 只比较申请与已有权限；不猜测 Shell 命令的副作用。
// 非法路径返回 Deny 和错误，其余不足的权限必须申请审批。
func Evaluate(policy Policy, requested ExtraPermissions) (Requirement, error) {
	err := validatePaths(policy.WriteRoots)
	if err != nil {
		return Deny, err
	}
	err = validatePaths(requested.WriteRoots)
	if err != nil {
		return Deny, err
	}
	if policy.Unrestricted {
		return Allow, nil
	}
	if requested.Network && !policy.Network {
		return Ask, nil
	}
	for _, path := range requested.WriteRoots {
		if !policy.canWrite(path) {
			return Ask, nil
		}
	}
	return Allow, nil
}

// ApplyDecision 只合入原申请并返回独立副本；拒绝时不交付执行权限。
// 调用方必须使用同一申请收到的决定，不能把一次批准挪给其他操作。
func ApplyDecision(request ApprovalRequest, decision Decision) (Policy, error) {
	if !decision.Approved {
		return Policy{}, fmt.Errorf("permissions: approval denied: %s", decision.Reason)
	}
	_, err := Evaluate(request.Current, request.Requested)
	if err != nil {
		return Policy{}, err
	}
	policy := request.Current
	policy.WriteRoots = slices.Clone(policy.WriteRoots)
	if policy.Unrestricted {
		return policy, nil
	}
	// 不折叠父子根：保护目录内的显式根是一次精确授权，具有独立语义。
	for _, path := range request.Requested.WriteRoots {
		if !policy.canWrite(path) {
			policy.WriteRoots = append(policy.WriteRoots, path)
		}
	}
	policy.Network = policy.Network || request.Requested.Network
	return policy, nil
}

func (p Policy) canWrite(path string) bool {
	allowed := false
	for _, root := range p.WriteRoots {
		if containsPath(root, path) {
			allowed = true
		}
		for _, name := range protectedNames {
			protected := filepath.Join(root, name)
			if !containsPath(protected, path) {
				continue
			}
			// 宽泛的父目录授权不能解除保护，必须明确授权保护目录内的路径。
			explicit := false
			for _, granted := range p.WriteRoots {
				if containsPath(protected, granted) && containsPath(granted, path) {
					explicit = true
					break
				}
			}
			if !explicit {
				return false
			}
		}
	}
	return allowed
}

func containsPath(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func validatePaths(paths []string) error {
	for _, path := range paths {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsRune(path, '\x00') {
			return fmt.Errorf("permissions: expected a clean absolute path, got %q", path)
		}
	}
	return nil
}
