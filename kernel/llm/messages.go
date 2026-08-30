package llm

import (
	"encoding/json"
	"fmt"

	"github.com/zendev-sh/goai/provider"

	"harness/kernel/session"
	"harness/kernel/tools"
)

func toProviderMessages(history []session.Message) ([]provider.Message, error) {
	out := make([]provider.Message, 0, len(history))
	for _, message := range history {
		role, err := toProviderRole(message.Role)
		if err != nil {
			return nil, err
		}
		if message.Role == session.RoleTool && (len(message.Blocks) != 1 || message.Blocks[0].Kind != "tool-result") {
			return nil, fmt.Errorf("llm: tool message needs exactly one tool-result block")
		}
		parts := make([]provider.Part, 0, len(message.Blocks))
		for _, block := range message.Blocks {
			part, err := toProviderPart(message.Role, block)
			if err != nil {
				return nil, err
			}
			parts = append(parts, part)
		}
		out = append(out, provider.Message{Role: role, Content: parts})
	}
	return out, nil
}

func toProviderRole(role session.Role) (provider.Role, error) {
	switch role {
	case session.RoleSystem:
		return provider.RoleSystem, nil
	case session.RoleUser:
		return provider.RoleUser, nil
	case session.RoleAssistant:
		return provider.RoleAssistant, nil
	case session.RoleTool:
		return provider.RoleTool, nil
	default:
		return "", fmt.Errorf("llm: unknown message role %q", role)
	}
}

func toProviderPart(role session.Role, block session.Block) (provider.Part, error) {
	if role == session.RoleTool && block.Kind != "tool-result" {
		return provider.Part{}, fmt.Errorf("llm: tool message contains %q block", block.Kind)
	}
	switch block.Kind {
	case "text":
		return provider.Part{Type: provider.PartText, Text: block.Text}, nil
	case "reasoning":
		return provider.Part{Type: provider.PartReasoning, Text: block.Text}, nil
	case "image":
		if block.Media == nil {
			return provider.Part{}, fmt.Errorf("llm: image block has no media")
		}
		return provider.Part{
			Type:      provider.PartImage,
			URL:       "data:" + block.Media.MIME + ";base64," + block.Media.Data,
			MediaType: block.Media.MIME,
		}, nil
	case "tool-call":
		if block.Tool == nil {
			return provider.Part{}, fmt.Errorf("llm: tool-call block has no tool")
		}
		return provider.Part{
			Type:       provider.PartToolCall,
			ToolCallID: block.Tool.ID,
			ToolName:   block.Tool.Name,
			ToolInput:  json.RawMessage(block.Tool.Args),
		}, nil
	case "tool-result":
		if role != session.RoleTool {
			return provider.Part{}, fmt.Errorf("llm: tool-result block needs tool role")
		}
		if block.Result == nil || block.Result.ID == "" || block.Result.Name == "" {
			return provider.Part{}, fmt.Errorf("llm: tool-result block needs id and name")
		}
		return provider.Part{
			Type:       provider.PartToolResult,
			ToolCallID: block.Result.ID,
			ToolName:   block.Result.Name,
			ToolOutput: block.Result.Content,
		}, nil
	default:
		return provider.Part{}, fmt.Errorf("llm: unknown block kind %q", block.Kind)
	}
}

func toProviderTools(definitions []tools.Definition) []provider.ToolDefinition {
	out := make([]provider.ToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		out = append(out, provider.ToolDefinition{
			Name:        definition.Name,
			Description: definition.Description,
			InputSchema: definition.InputSchema,
		})
	}
	return out
}
