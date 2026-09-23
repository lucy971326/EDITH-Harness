package tools

import (
	"context"

	"harness/kernel/permissions"
)

type accessKey struct{}

// WithAccess 把本轮权限交给动态工具发现；调用权限仍由 Call 独立携带。
func WithAccess(ctx context.Context, access Access) context.Context {
	return context.WithValue(ctx, accessKey{}, access)
}

// AccessFromContext 返回工具发现的运行身份；缺少可信运行上下文时默认禁用 MCP。
func AccessFromContext(ctx context.Context) Access {
	access, ok := ctx.Value(accessKey{}).(Access)
	if !ok || access.Mode == "" {
		return Access{Mode: permissions.ReadOnly}
	}
	return access
}
