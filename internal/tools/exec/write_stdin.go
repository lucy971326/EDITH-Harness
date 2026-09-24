package exec

import (
	"context"
	"fmt"
	"time"

	"harness/internal/machine"
	"harness/internal/tools"
)

const (
	defaultWriteYieldMS = int64(250)
	defaultPollYieldMS  = int64(5_000)
	minWriteYieldMS     = int64(250)
	minPollYieldMS      = int64(5_000)
	maxWriteYieldMS     = int64(30_000)
	maxPollYieldMS      = int64(300_000)
)

// 数据。write_stdin 的模型参数。
type WriteStdinArgs struct {
	ProcessID       int64  `json:"process_id" jsonschema:"minimum=1,description=Identifier of the running exec process."`
	Chars           string `json:"chars,omitempty" jsonschema:"description=Bytes to write to stdin. Empty or omitted polls without writing."`
	YieldTimeMS     *int64 `json:"yield_time_ms,omitempty" jsonschema:"description=Wait before yielding recent output."`
	MaxOutputTokens *int   `json:"max_output_tokens,omitempty" jsonschema:"minimum=64,description=Output token budget. Defaults to 10000 tokens."`
}

func newWriteStdinTool(processes machine.AgentProcesses) tools.Tool {
	return tools.New(
		"write_stdin",
		"Write characters to an existing exec process and return recent output.",
		func(ctx context.Context, call tools.Call, args WriteStdinArgs) (tools.Result, error) {
			maxTokens, err := outputTokenBudget(args.MaxOutputTokens)
			if err != nil {
				return tools.Result{}, err
			}

			yieldMS := defaultWriteYieldMS
			minimum := minWriteYieldMS
			maximum := maxWriteYieldMS
			if args.Chars == "" {
				yieldMS = defaultPollYieldMS
				minimum = minPollYieldMS
				maximum = maxPollYieldMS
			}
			if args.YieldTimeMS != nil {
				yieldMS = *args.YieldTimeMS
			}
			yieldMS = clamp(yieldMS, minimum, maximum)

			started := time.Now()
			output, err := processes.AgentInteract(ctx, machine.ProcessInteraction{
				OwnerID:   call.SessionID,
				ProcessID: args.ProcessID,
				Chars:     []byte(args.Chars),
				Wait:      time.Duration(yieldMS) * time.Millisecond,
			})
			if err != nil {
				return tools.Result{}, fmt.Errorf("write_stdin: %w", err)
			}
			return tools.Result{Content: renderProcessOutput(output, time.Since(started), maxTokens)}, nil
		},
	)
}
