package llm

import (
	"encoding/json"
	"errors"
	anthropic "github.com/anthropics/anthropic-sdk-go"
	openai "github.com/openai/openai-go/v3"
	"strings"
)

// ErrContextWindow 表示供应商明确拒绝了过大的上下文；认证、限流等错误不属于它。
var ErrContextWindow = errors.New("模型上下文超限")

// InputBudget 返回模型输入预算，预留输出与安全余量；未知窗口不自动压缩。
func (c *Client) InputBudget(model string) int {
	d, ok := c.state().models[model]
	if !ok || d.ContextWindow <= 0 {
		return 0
	}
	output := d.MaxOutput
	if output <= 0 {
		output = min(16384, d.ContextWindow/4)
	}
	return max(1, d.ContextWindow-output-max(1024, d.ContextWindow/10))
}

// EstimateInput 保守估算完整请求，不将图片和加密续接内容当作精确 token 数。
func EstimateInput(input Input) int {
	tokens := estimateText(input.System) + 16
	for _, message := range input.History {
		tokens += 8
		for _, b := range message.Blocks {
			tokens += estimateText(b.Text)
			if b.Tool != nil {
				tokens += estimateText(b.Tool.Name+b.Tool.Args) + 16
			}
			if b.Result != nil {
				tokens += estimateText(b.Result.Name+b.Result.Content) + 16
			}
			if b.Media != nil {
				tokens += 4096
			}
			if b.Continuation != nil {
				tokens += len(b.Continuation.Data) / 3
			}
		}
	}
	for _, tool := range input.Tools {
		data, _ := json.Marshal(tool)
		tokens += estimateText(string(data)) + 16
	}
	return tokens
}

func estimateText(text string) int {
	units := 0
	for _, r := range text {
		if r < 128 {
			units++
		} else {
			units += 6
		}
	}
	return (units + 2) / 3
}

func contextError(code, message string) error {
	text := strings.ToLower(message)
	if code == "context_length_exceeded" || code == "context_window_exceeded" ||
		strings.Contains(text, "maximum context length") || strings.Contains(text, "prompt is too long") ||
		strings.Contains(text, "exceeds the context window") || strings.Contains(text, "context window exceeded") {
		return ErrContextWindow
	}
	return nil
}

func classifyContextError(err error) error {
	var openAIError *openai.Error
	if errors.As(err, &openAIError) && (openAIError.StatusCode == 400 || openAIError.StatusCode == 413) {
		if contextError(openAIError.Code, openAIError.Message) != nil {
			return errors.Join(ErrContextWindow, err)
		}
	}
	var anthropicError *anthropic.Error
	if errors.As(err, &anthropicError) && (anthropicError.StatusCode == 400 || anthropicError.StatusCode == 413) {
		if contextError("", anthropicError.Error()) != nil {
			return errors.Join(ErrContextWindow, err)
		}
	}
	return err
}
