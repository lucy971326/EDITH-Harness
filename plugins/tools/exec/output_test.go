package exec

import (
	"strings"
	"testing"
	"time"

	"harness/kernel/machine"
)

func TestRenderProcessOutput(t *testing.T) {
	content := renderProcessOutput(machine.ProcessOutput{
		ProcessID: 42,
		Output:    []byte("hello"),
	}, 1250*time.Millisecond, defaultOutputTokens)

	for _, want := range []string{
		"Wall time: 1.2500 seconds",
		"Process running with process ID 42",
		"Original token count: 2",
		"Output:\nhello",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("output missing %q:\n%s", want, content)
		}
	}
}

func TestRenderProcessOutputReportsBothCaps(t *testing.T) {
	content := renderProcessOutput(machine.ProcessOutput{
		ProcessID:    42,
		Output:       []byte(strings.Repeat("a", 1000)),
		Exited:       true,
		ExitCode:     7,
		OmittedBytes: 20,
	}, time.Second, minimumOutputTokens)

	for _, want := range []string{
		"Process exited with code 7",
		"Original token count: 255",
		"Warning: truncated output",
		"20 bytes omitted",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("output missing %q:\n%s", want, content)
		}
	}
	if len(content) > minimumOutputTokens*4 {
		t.Fatalf("output length = %d, budget = %d", len(content), minimumOutputTokens*4)
	}
}

func TestOutputTokenBudgetRejectsTooSmallValue(t *testing.T) {
	requested := minimumOutputTokens - 1
	_, err := outputTokenBudget(&requested)
	if err == nil {
		t.Fatal("outputTokenBudget() accepted a value too small for the result header")
	}
}
