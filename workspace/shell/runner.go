// Package shell 在 process 上提供一次性命令能力。
package shell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"harness/core"
	"harness/workspace/process"
)

const (
	serviceName        = "shell"
	defaultOutputLimit = 64 * 1024
)

// Result 是一条命令的输出、退出码和截断标记。
type Result struct {
	Stdout    string // 标准输出的可保留部分
	Stderr    string // 错误输出的可保留部分
	ExitCode  int    // 真实退出码
	Truncated bool   // 任意一路输出超过上限
}

// Runner 启动一条命令，等它结束后返回结果。
type Runner interface {
	Run(ctx context.Context, command process.Command) (Result, error)
}

// runner 组合长期进程能力和每条输出流的保留上限。
type runner struct {
	spawner     process.Spawner // 真正启动进程的底座
	outputLimit int             // 每路输出最多保留的字节数
}

// capture 一直接收输出，但只保留前 limit 个字节。
type capture struct {
	data      bytes.Buffer // 已保留的前缀
	limit     int          // 最多保留字节数
	truncated bool         // 后面是否还丢过数据
}

// New 在一个 Spawner 上建立 Runner；outputLimit 是 stdout 和 stderr 各自的保留上限。
func New(spawner process.Spawner, outputLimit int) (Runner, error) {
	if spawner == nil {
		return nil, errors.New("shell 缺少 process.Spawner")
	}
	if outputLimit <= 0 {
		outputLimit = defaultOutputLimit
	}
	return &runner{spawner: spawner, outputLimit: outputLimit}, nil
}

// Get 从 App 取出 shell 能力。
func Get(app *core.App) (Runner, error) {
	return core.Resolve[Runner](app, serviceName)
}

// Run 关掉 stdin，同时收 stdout/stderr，最后返回命令的真实退出结果。
func (r *runner) Run(ctx context.Context, command process.Command) (Result, error) {
	handle, err := r.spawner.Spawn(ctx, command)
	if err != nil {
		return Result{}, err
	}
	err = handle.Stdin().Close()
	if err != nil {
		_ = handle.Close()
		return Result{}, fmt.Errorf("关闭命令 stdin 失败：%w", err)
	}

	stdout := &capture{limit: r.outputLimit}
	stderr := &capture{limit: r.outputLimit}
	readErrs := make(chan error, 2)
	go func() {
		_, copyErr := io.Copy(stdout, handle.Stdout())
		readErrs <- copyErr
	}()
	go func() {
		_, copyErr := io.Copy(stderr, handle.Stderr())
		readErrs <- copyErr
	}()

	exit, waitErr := handle.Wait()
	firstReadErr := <-readErrs
	secondReadErr := <-readErrs
	result := Result{
		Stdout:    stdout.text(),
		Stderr:    stderr.text(),
		ExitCode:  exit.Code,
		Truncated: stdout.truncated || stderr.truncated,
	}
	if waitErr != nil {
		return result, waitErr
	}
	if firstReadErr != nil || secondReadErr != nil {
		return result, errors.Join(firstReadErr, secondReadErr)
	}
	return result, nil
}

// Write 让命令继续吐字，超过上限的部分只丢掉、不堵住进程。
func (c *capture) Write(data []byte) (int, error) {
	room := c.limit - c.data.Len()
	if room > len(data) {
		room = len(data)
	}
	if room > 0 {
		_, _ = c.data.Write(data[:room])
	}
	if room < len(data) {
		c.truncated = true
	}
	return len(data), nil
}

// text 丢掉被字节上限切断的半个 UTF-8 字符，不把乱码交给模型。
func (c *capture) text() string {
	data := c.data.Bytes()
	for len(data) > 0 && !utf8.Valid(data) {
		data = data[:len(data)-1]
	}
	return string(data)
}
