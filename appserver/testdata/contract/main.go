// 仅供生成链测试使用；不进入实际接口目录。
package main

import (
	"fmt"
	"os"
	"time"

	"harness/appserver"
)

// 数据。覆盖当前三个接口尚未使用的可选项和枚举，以及数组引用。
type Example struct {
	Name  string    `json:"name"`
	Note  string    `json:"note,omitempty"`
	Mode  string    `json:"mode" jsonschema:"enum=read,enum=write"`
	Items []Item    `json:"items"`
	At    time.Time `json:"at"`
}

// 数据。被数组引用的嵌套结构。
type Item struct {
	Value string `json:"value"`
}

func main() {
	definition, err := appserver.Describe(appserver.Method[Example, Example]{Name: "test/example"})
	if err == nil {
		_, err = os.Stdout.Write(definition.InputSchema)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
