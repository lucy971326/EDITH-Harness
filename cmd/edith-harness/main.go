// Command edith-harness 读取 ~/.harness 并组装最小 Harness 运行时。
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"harness/compose"
	"harness/ui"
)

func main() {
	if len(os.Args) > 1 {
		if len(os.Args) != 2 || os.Args[1] != "--dump-config" {
			fmt.Fprintln(os.Stderr, "用法：edith-harness [--dump-config]")
			os.Exit(2)
		}
		userHome, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取用户目录失败：", err)
			os.Exit(1)
		}
		err = compose.DumpConfig(filepath.Join(userHome, ".harness"), os.Stdout)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取用户目录失败：", err)
		os.Exit(1)
	}
	runtime, err := compose.Open(filepath.Join(userHome, ".harness"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer runtime.App.Close()
	screen, err := ui.Get(runtime.App)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	err = screen.Run(context.Background())
	if err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
