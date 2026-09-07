// Command contracts 从产品声明导出契约，不安装任何插件。
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"harness/products/harness"
)

func main() {
	definitions, err := harness.Definitions()
	if err == nil {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		err = encoder.Encode(definitions)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
