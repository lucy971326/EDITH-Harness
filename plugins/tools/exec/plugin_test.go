package exec_test

import (
	"testing"

	"harness/kernel/approvals"
	"harness/kernel/tools"
	machinelocal "harness/plugins/machine/local"
	exectool "harness/plugins/tools/exec"
)

func TestRegisterBothTools(t *testing.T) {
	service := approvals.New()
	t.Cleanup(func() { _ = service.Close() })
	m, err := machinelocal.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	registry := tools.NewRegistry()
	err = exectool.Register(registry, m, m, service)
	if err != nil {
		t.Fatal(err)
	}
	definitions := registry.List()
	if len(definitions) != 2 || definitions[0].Name != "exec_command" || definitions[1].Name != "write_stdin" {
		t.Fatalf("definitions = %#v", definitions)
	}
}
