package tools

import (
	"context"
	"fmt"
	"log"

	"harness/core"
	"harness/session"
)

// ExecuteCall 走完九步流水线。顺序从此冻结：
//
//	① 记账"要调了" → ② 前置检查链 → ③ 问人 → ④ 守卫 → ⑤ 记执行边界
//	→ ⑥ 环绕链+本体 → ⑦ 后置链 → ⑧ 崩溃兜底 → ⑨ 记账"结果"
//
// 三条铁律都在这兑现：先记后干（①落不上就不开跑）；
// 参数不可改（Call 是值，链上拿到也改不动）；
// 终局必入账（⑨兜住一切路径）。
// 唯一不记终局的出口：已开跑后被取消——账上留着 start 无 result，
// 那是"结果未知"的记号，恢复时进待裁决，禁止当 skipped 糊弄。
func (r *Registry) ExecuteCall(ctx context.Context, app *core.App, book *session.Session, call Call) (result Result) {
	callRecorded := false
	resultRecorded := false

	// ⑧ 崩溃兜底：链上、本体、问人，崩在哪都变成 failed 结果，进程不倒。
	defer func() {
		if rec := recover(); rec != nil {
			result = Result{
				Output: fmt.Sprintf("工具 %s 崩溃：%v", call.Name, rec),
				Status: session.ResultFailed,
			}
			if callRecorded && !resultRecorded {
				r.recordFinal(book, call, result, &resultRecorded)
			}
		}
	}()

	// ① 先记"要调了"——账记不上就没有然后。
	_, err := book.RecordToolCall(session.ToolCallData{
		ID:       call.ID,
		Name:     call.Name,
		Argument: call.Argument,
	})
	if err != nil {
		return Result{Output: "记账失败，拒绝执行：" + err.Error(), Status: session.ResultFailed}
	}
	callRecorded = true

	// 终局小工具：落⑨并记住落过了。
	finish := func(status string, output string) Result {
		final := Result{Output: output, Status: status}
		r.recordFinal(book, call, final, &resultRecorded)
		return final
	}

	tool, found := r.Lookup(call.Name, call.Agent)
	if !found {
		return finish(session.ResultFailed, "没有 "+call.Name+" 这个工具")
	}

	// ② 前置检查链：插件说了算——放行 / 拒 / 要问人。
	decision := runScopedChain(app, PreExecute, PreCall{Call: call, Tool: tool.Schema},
		func(PreCall) Decision { return Decision{Kind: Allow} })

	// ③ 问人：说了要问，就得有人答；没人答 = 拒。
	if decision.Kind == Ask {
		asker, err := core.Resolve[Approver](app, "approval")
		if err != nil {
			return finish(session.ResultSkipped, "要问人但没人答（没挂 approval 能力）："+decision.Reason)
		}
		decision = asker.Approve(call)
	}
	if decision.Kind != Allow {
		return finish(session.ResultSkipped, decision.Reason)
	}

	// ④ 守卫：最后一道闸，只能拒，不能翻案——放行的决定到这也可能被否。
	reason := runScopedChain(app, Guard, call, func(Call) string { return "" })
	if reason != "" {
		return finish(session.ResultSkipped, reason)
	}

	// 开跑前取消：没碰过本体，skipped。
	if ctx.Err() != nil {
		return finish(session.ResultSkipped, "开跑前取消了")
	}

	// ⑤ 执行开始边界：问完、守完才落这笔——写早了，被拒的调用也会被当成开跑过。
	// 从这里起"开没开跑"账上有据，重启不再靠猜。
	_, err = book.RecordToolStart(call.ID)
	if err != nil {
		return finish(session.ResultSkipped, "记执行边界失败，不敢开跑："+err.Error())
	}

	// ⑥ 环绕链：超时、重试挂这；本体在链尾，中间件可以包着它做文章。
	outcome := runScopedChain(app, Execute, call, func(c Call) Outcome {
		output, err := tool.Execute(ctx, c.Argument)
		return Outcome{Output: output, Err: err}
	})

	// 已开跑后被取消：不记终局，账上留 start 无 result = 结果未知，交给 M4 待裁决。
	// 宁可待裁决，不假装知道结果。
	if ctx.Err() != nil {
		return Result{Output: outcome.Output, Status: ResultUnknown}
	}

	draft := Result{Output: outcome.Output, Status: session.ResultSuccess}
	if outcome.Err != nil {
		draft = Result{Output: outcome.Err.Error(), Status: session.ResultFailed}
	}

	// ⑦ 后置链：插件可以改结果（截断超长输出、附话）。
	final := runScopedChain(app, PostExecute, PostCall{Call: call, Result: draft},
		func(p PostCall) Result { return p.Result })

	// ⑨ 终局入账。
	r.recordFinal(book, call, final, &resultRecorded)
	return final
}

// recordFinal 落⑨。落盘都坏了的话只能留痕——账本自会拒绝坏账。
func (r *Registry) recordFinal(book *session.Session, call Call, final Result, recorded *bool) {
	_, err := book.RecordToolResult(session.ToolResultData{
		CallID: call.ID,
		Output: final.Output,
		Status: final.Status,
	})
	if err != nil {
		log.Printf("tools: 第⑨笔记账失败（%s %s）：%v", call.Name, call.ID, err)
	}
	*recorded = true
}

// runScopedChain 沿作用域链聚合一条链：父层在外、子层在内、本体在最里。
// 全局插件挂 root 的链，管得住每个 agent；agent 自己的定制永远在更内层。
func runScopedChain[P, R any](app *core.App, name string, payload P, body func(P) R) R {
	if app == nil {
		return body(payload)
	}
	return runScopedChain[P, R](app.Parent(), name, payload, func(p P) R {
		return core.RunChain(app, name, p, body)
	})
}
