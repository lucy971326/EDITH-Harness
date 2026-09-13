package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"harness/kernel/tools"
)

const maxResultBytes = 50 * 1024

func renderResult(result *sdk.CallToolResult) (tools.Result, error) {
	if result == nil {
		return tools.Result{}, fmt.Errorf("mcp: server returned nil result")
	}
	textParts := make([]string, 0, len(result.Content))
	for _, content := range result.Content {
		text, ok := content.(*sdk.TextContent)
		if !ok {
			return tools.Result{
				Content: fmt.Sprintf("MCP result content type %T is not supported", content),
				IsError: true,
			}, nil
		}
		textParts = append(textParts, text.Text)
	}
	plain := strings.Join(textParts, "\n")
	if result.StructuredContent == nil {
		return tools.Result{Content: tools.TruncateHead(plain), IsError: result.IsError}, nil
	}
	structured, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return tools.Result{}, fmt.Errorf("mcp: encode structured result: %w", err)
	}
	if len(structured) > maxResultBytes {
		return tools.Result{Content: "MCP structured result exceeds 50 KiB", IsError: true}, nil
	}
	if plain == "" {
		return tools.Result{Content: string(structured), IsError: result.IsError}, nil
	}
	separator := "\n\nStructured result:\n"
	plain = truncateBytes(plain, maxResultBytes-len(separator)-len(structured))
	return tools.Result{Content: plain + separator + string(structured), IsError: result.IsError}, nil
}

func truncateBytes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	notice := "\n\n[output truncated]"
	if limit <= len(notice) {
		return notice[:limit]
	}
	value = value[:limit-len(notice)]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value + notice
}
