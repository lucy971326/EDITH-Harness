package llm

import (
	"encoding/json"
	"fmt"

	"github.com/zendev-sh/goai/provider"

	"harness/kernel/session"
)

func toProviderMessages(history []session.Message) ([]provider.Message, error) {
	out := make([]provider.Message, 0, len(history))
	for _, message := range history {
		role, err := toProviderRole(message.Role)
		if err != nil {
			return nil, err
		}
		parts := make([]provider.Part, 0, len(message.Blocks))
		for _, block := range message.Blocks {
			part, err := toProviderPart(block)
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
		return "", fmt.Errorf("llm: tool history is not implemented")
	default:
		return "", fmt.Errorf("llm: unknown message role %q", role)
	}
}

func toProviderPart(block session.Block) (provider.Part, error) {
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
		if !json.Valid([]byte(block.Tool.Args)) {
			return provider.Part{}, fmt.Errorf("llm: tool-call %q has invalid JSON arguments", block.Tool.Name)
		}
		return provider.Part{
			Type:       provider.PartToolCall,
			ToolCallID: block.Tool.ID,
			ToolName:   block.Tool.Name,
			ToolInput:  json.RawMessage(block.Tool.Args),
		}, nil
	default:
		return provider.Part{}, fmt.Errorf("llm: unknown block kind %q", block.Kind)
	}
}
