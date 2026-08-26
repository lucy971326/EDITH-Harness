package commands

import (
	"context"
	"fmt"
	"strings"
)

// Execute 解析并运行一条斜杠行，返回给人看的结果。
// 调用方应先用 LooksLike 判断是否进入命令平面；不是斜杠行返回 error。
// 格式错或名字不认识返回 KindError，不返回 Go error，也不进模型。
func (r *Registry) Execute(ctx context.Context, sessionID string, line string) (Result, error) {
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	if !LooksLike(line) {
		return Result{}, fmt.Errorf("不是斜杠命令")
	}
	parsed, ok := Parse(line)
	if !ok {
		return Result{Kind: KindError, Text: "命令格式不对"}, nil
	}
	command, found := r.Find(parsed.Name)
	if !found {
		return Result{Kind: KindError, Text: fmt.Sprintf("没有这条命令：/%s", parsed.Name)}, nil
	}
	result := command.Run(ctx, Invocation{
		Name:      parsed.Name,
		RawInput:  parsed.RawInput,
		SessionID: sessionID,
	})
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	return normalizeResult(result), nil
}

func normalizeResult(result Result) Result {
	if result.Kind == KindSuccess {
		return result
	}
	if result.Kind != KindError {
		return Result{Kind: KindError, Text: "命令没有返回结果"}
	}
	if strings.TrimSpace(result.Text) == "" {
		return Result{Kind: KindError, Text: "命令失败"}
	}
	return result
}
