package exec

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"harness/internal/approvals"
	"harness/internal/machine"
	"harness/internal/tools"
)

const (
	defaultOutputTokens = 10_000
	minimumOutputTokens = 64
	maxOutputTokens     = 1024 * 1024 / 4
)

func toolEntries(processes machine.AgentProcesses, paths machine.Machine, approvalService *approvals.Service) []tools.Tool {
	return []tools.Tool{
		newExecCommandTool(processes, paths, approvalService),
		newWriteStdinTool(processes),
	}
}

func outputTokenBudget(requested *int) (int, error) {
	if requested == nil {
		return defaultOutputTokens, nil
	}
	if *requested < minimumOutputTokens {
		return 0, fmt.Errorf("max_output_tokens must be at least %d", minimumOutputTokens)
	}
	if *requested > maxOutputTokens {
		return maxOutputTokens, nil
	}
	return *requested, nil
}

func renderProcessOutput(output machine.ProcessOutput, wallTime time.Duration, maxTokens int) string {
	raw := flattenTerminalOutput(string(output.Output))
	originalBytes := int64(len(output.Output)) + output.OmittedBytes
	originalTokens := (originalBytes + 3) / 4
	if output.OmittedBytes > 0 {
		raw = fmt.Sprintf("... %d bytes omitted ...\n%s", output.OmittedBytes, raw)
	}

	sections := []string{fmt.Sprintf("Wall time: %.4f seconds", wallTime.Seconds())}
	if output.Exited {
		sections = append(sections, fmt.Sprintf("Process exited with code %d", output.ExitCode))
	} else {
		sections = append(sections, fmt.Sprintf("Process running with process ID %d", output.ProcessID))
	}
	sections = append(sections, fmt.Sprintf("Original token count: %d", originalTokens), "Output:")
	header := strings.Join(sections, "\n")
	remaining := maxTokens*4 - len(header) - 1
	raw = truncateOutput(raw, remaining, originalTokens)
	return header + "\n" + raw
}

// flattenTerminalOutput 将终端原地重绘还原为适合聊天卡片的纯文本。
func flattenTerminalOutput(text string) string {
	text = strings.ToValidUTF8(text, "�")
	var output strings.Builder
	var line []rune
	cursor := 0

	for index := 0; index < len(text); {
		if text[index] == 0x1b {
			if index+1 >= len(text) {
				break
			}
			switch text[index+1] {
			case '[':
				end := index + 2
				for end < len(text) && (text[end] < 0x40 || text[end] > 0x7e) {
					end++
				}
				if end >= len(text) {
					return output.String() + string(line)
				}
				parameter := 0
				for offset := index + 2; offset < end && text[offset] >= '0' && text[offset] <= '9'; offset++ {
					parameter = parameter*10 + int(text[offset]-'0')
				}
				switch text[end] {
				case 'K':
					switch parameter {
					case 0:
						if cursor < len(line) {
							line = line[:cursor]
						}
					case 1:
						limit := min(cursor+1, len(line))
						for position := 0; position < limit; position++ {
							line[position] = ' '
						}
					case 2:
						line = line[:0]
					}
				case 'G':
					if parameter == 0 {
						parameter = 1
					}
					cursor = parameter - 1
				case 'C':
					if parameter == 0 {
						parameter = 1
					}
					cursor += parameter
				case 'D':
					if parameter == 0 {
						parameter = 1
					}
					cursor = max(0, cursor-parameter)
				}
				index = end + 1
				continue
			case ']', 'P', 'X', '^', '_':
				index += 2
				for index < len(text) {
					if text[index] == 0x07 {
						index++
						break
					}
					if text[index] == 0x1b && index+1 < len(text) && text[index+1] == '\\' {
						index += 2
						break
					}
					index++
				}
				continue
			default:
				end := index + 1
				for end < len(text) && text[end] >= 0x20 && text[end] <= 0x2f {
					end++
				}
				if end < len(text) {
					end++
				}
				index = end
				continue
			}
		}

		r, size := utf8.DecodeRuneInString(text[index:])
		index += size
		switch r {
		case '\r':
			cursor = 0
		case '\n':
			output.WriteString(string(line))
			output.WriteByte('\n')
			line = line[:0]
			cursor = 0
		case '\b':
			cursor = max(0, cursor-1)
		case '\t':
			next := (cursor/8 + 1) * 8
			for len(line) < next {
				line = append(line, ' ')
			}
			cursor = next
		default:
			if r < 0x20 || r == 0x7f {
				continue
			}
			for len(line) <= cursor {
				line = append(line, ' ')
			}
			line[cursor] = r
			cursor++
		}
	}
	output.WriteString(string(line))
	return output.String()
}

func truncateOutput(text string, maxBytes int, originalTokens int64) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(text) <= maxBytes {
		return text
	}
	warning := fmt.Sprintf("Warning: truncated output (original token count: %d)\n", originalTokens)
	separator := "\n... output truncated ...\n"
	contentBytes := maxBytes - len(warning) - len(separator)
	if contentBytes <= 0 {
		return validUTF8Prefix(warning, maxBytes)
	}
	headBytes := contentBytes / 2
	tailBytes := contentBytes - headBytes
	return warning + validUTF8Prefix(text, headBytes) + separator + validUTF8Suffix(text, tailBytes)
}

func validUTF8Prefix(text string, maxBytes int) string {
	if len(text) <= maxBytes {
		return text
	}
	end := maxBytes
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end]
}

func validUTF8Suffix(text string, maxBytes int) string {
	if len(text) <= maxBytes {
		return text
	}
	start := len(text) - maxBytes
	for start < len(text) && !utf8.RuneStart(text[start]) {
		start++
	}
	return text[start:]
}

func clamp(value, minimum, maximum int64) int64 {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
