package subagents

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"harness/kernel/session"
	delegation "harness/kernel/subagents"
	"harness/kernel/tools"
)

func toolEntries(s *delegation.Subagents) []tools.Tool {
	return []tools.Tool{
		withIdentity("subagent_options", "List available agents (including their system instructions) and models with supported reasoning efforts.", func(ctx context.Context, call tools.Call, args optionsArgs) (tools.Result, error) {
			value, err := s.Options(ctx)
			return jsonResult(value, err)
		}),
		withIdentity("subagent_spawn", "Delegate asynchronously to a new child session. Returns immediately after startup. Only one delegation level is allowed. The child shares your workspace, not your history.", func(ctx context.Context, call tools.Call, args spawnArgs) (tools.Result, error) {
			value, err := s.Spawn(ctx, delegation.SpawnInput{
				ParentSessionID: call.SessionID, ParentRunID: call.RunID,
				Description: args.Description, AgentID: args.AgentID,
				Model: args.Model, ReasoningEffort: args.ReasoningEffort,
			})
			return jsonResult(value, err)
		}),
		withIdentity("subagent_send", "Append text instructions to your child. A busy child receives a steer; an idle child starts a new turn in the same session. Settings do not change. An error may occur after acceptance: inspect status before retrying.", func(ctx context.Context, call tools.Call, args sendArgs) (tools.Result, error) {
			value, err := s.Send(ctx, call.SessionID, args.TaskID, session.UserMessage{
				Blocks: []session.Block{{Kind: "text", Text: args.Text}},
			})
			return jsonResult(value, err)
		}),
		withIdentity("subagent_list", "List your children, their turn history, saved results and errors. Optionally select one task.", func(ctx context.Context, call tools.Call, args listArgs) (tools.Result, error) {
			value, err := s.List(call.SessionID, args.TaskID)
			reply := listResult{Tasks: value}
			if err != nil {
				reply.Error = err.Error()
			}
			result, marshalErr := jsonResult(reply, nil)
			result.IsError = err != nil
			return result, marshalErr
		}),
		withIdentity("subagent_wait", "Wait for one child's current turn, or query immediately with timeoutSeconds=0. Returns status and result location, not the answer text; use subagent_list to read saved answers. Timeout does not stop the child. Automatic notification and interruption by user steer are not yet available.", func(ctx context.Context, call tools.Call, args waitArgs) (tools.Result, error) {
			seconds := 60
			if args.TimeoutSeconds != nil {
				seconds = *args.TimeoutSeconds
			}
			if seconds < 0 || seconds > 60 {
				return tools.Result{}, fmt.Errorf("timeoutSeconds must be between 0 and 60")
			}
			value, err := s.Wait(ctx, call.SessionID, args.TaskID, time.Duration(seconds)*time.Second)
			return jsonResult(value, err)
		}),
		withIdentity("subagent_stop", "Request cancellation of your child's current run, not the parent. Records remain available. Use subagent_list or subagent_wait to observe completion.", func(ctx context.Context, call tools.Call, args stopArgs) (tools.Result, error) {
			err := s.Stop(ctx, call.SessionID, args.TaskID)
			return jsonResult(stopResult{TaskID: args.TaskID, StopRequested: err == nil}, err)
		}),
	}
}

// withIdentity 使用工具调用上下文身份，模型参数中不提供身份入口。
func withIdentity[Args any](name, description string, handler tools.Handler[Args]) tools.Tool {
	return tools.New(name, description, func(ctx context.Context, call tools.Call, args Args) (tools.Result, error) {
		if call.SessionID == "" || call.RunID == "" || call.ToolCallID == "" {
			return tools.Result{}, fmt.Errorf("subagent tool requires trusted session, run and tool call identity")
		}
		return handler(ctx, call, args)
	})
}

func jsonResult(value any, err error) (tools.Result, error) {
	if err != nil {
		return tools.Result{}, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{Content: string(data)}, nil
}
