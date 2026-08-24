// Command dsh 读取 ~/.harness 并组装最小 Harness 运行时。
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"harness/compose"
)

func main() {
	userHome, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取用户目录失败：", err)
		os.Exit(1)
	}
	workspace, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取工作目录失败：", err)
		os.Exit(1)
	}
	runtime, err := compose.Open(filepath.Join(userHome, ".harness"), workspace)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	runtime.App.Close()
}
