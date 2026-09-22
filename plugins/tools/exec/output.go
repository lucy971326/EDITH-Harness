package exec

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"harness/kernel/machine"
	"harness/kernel/tools"
)

const (
	defaultOutputTokens = 10_000
	minimumOutputTokens = 64
	maxOutputTokens     = 1024 * 1024 / 4
)

func toolEntries(processes machine.AgentProcesses, paths machine.Machine) []tools.Tool {
	return []tools.Tool{
		newExecCommandTool(processes, paths),
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
	raw := strings.ToValidUTF8(string(output.Output), "�")
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
