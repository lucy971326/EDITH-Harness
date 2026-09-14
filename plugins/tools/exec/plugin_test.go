package exec_test

import (
	"testing"

	"harness/kernel/host"
	"harness/kernel/tools"
	machinelocal "harness/plugins/machine/local"
	exectool "harness/plugins/tools/exec"
)

func TestPluginRegistersBothTools(t *testing.T) {
	h := host.NewHost()
	t.Cleanup(func() {
		if err := h.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, plugin := range []host.Plugin{
		machinelocal.New(),
		tools.NewPlugin(),
		exectool.New(),
	} {
		err := h.Install(plugin)
		if err != nil {
			t.Fatal(err)
		}
	}

	registry, err := host.Resolve[tools.Tools](h, "tools")
	if err != nil {
		t.Fatal(err)
	}
	definitions := registry.List()
	if len(definitions) != 2 || definitions[0].Name != "exec_command" || definitions[1].Name != "write_stdin" {
		t.Fatalf("definitions = %#v", definitions)
	}
}
