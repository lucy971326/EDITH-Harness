// Package core 是进程内协作基础设施：服务表、事件总线、生死收摊。
// 不懂任何 agent 知识——不知道模型、工具、对话为何物。
package core

import (
	"fmt"
	"log"
	"sync"
)

// App 是公共场地：服务表 + 事件订阅表 + 清理栈。
type App struct {
	mu          sync.Mutex            // 保护下面四张表
	services    map[string]any        // 能力名 -> 能力对象；只在启动期动
	listeners   map[string][]Listener // 事件名 -> 观察者，按注册顺序
	middlewares map[string][]any      // 事件名 -> 中间件；any 因为泛型，RunChain 时断言
	tasks       map[string][]Task     // 事件名 -> 任务，按注册顺序
	cleanups    []func()              // 收摊函数栈，Close 时逆序执行
	warn        func(message string)  // 崩溃告警出口，默认 log，测试可替换
	closed      bool                  // 保证 Close 只生效一次
}

// New 建一个空 App。
func New() *App {
	return &App{
		services:    make(map[string]any),
		listeners:   make(map[string][]Listener),
		middlewares: make(map[string][]any),
		tasks:       make(map[string][]Task),
		warn:        func(message string) { log.Println("core:", message) },
	}
}

// Plugin 是插件的全部契约。
type Plugin interface {
	// Name 返回插件名，用于报错指名。
	Name() string
	// Start 在启动阶段往场地里放东西；返回 error 即启动失败，触发逆序回滚。
	Start(app *App) error
}

// Install 按传入顺序启动插件；中途失败则逆序回滚已启动部分，错误指名失败的插件。
func (a *App) Install(plugins ...Plugin) error {
	a.mu.Lock()
	base := len(a.cleanups)
	a.mu.Unlock()

	for _, plugin := range plugins {
		err := plugin.Start(a)
		if err != nil {
			a.rollback(base)
			return fmt.Errorf("插件 %s 启动失败：%w", plugin.Name(), err)
		}
	}
	return nil
}

// rollback 逆序执行 base 之上的收摊函数并从栈上摘除。
func (a *App) rollback(base int) {
	a.mu.Lock()
	doomed := a.cleanups[base:]
	a.cleanups = a.cleanups[:base]
	a.mu.Unlock()

	for i := len(doomed) - 1; i >= 0; i-- {
		a.runCleanup(doomed[i])
	}
}

// RegisterService 往服务表登记能力，键是能力名、不是插件名。
// 只在启动期调用；同名重复注册 panic——组装错误。
func (a *App) RegisterService(name string, service any) {
	a.mu.Lock()
	defer a.mu.Unlock()

	_, exists := a.services[name]
	if exists {
		panic(fmt.Sprintf("能力 %s 已注册，不能重复注册", name))
	}
	a.services[name] = service
}

// Resolve[T] 按能力名取出断言成 T 的服务。
// 未注册返回 error（组装不完整，可处置）；类型不符 panic（框架编程错误）。
func Resolve[T any](a *App, name string) (T, error) {
	a.mu.Lock()
	service, exists := a.services[name]
	a.mu.Unlock()

	var missing T
	if !exists {
		return missing, fmt.Errorf("能力 %s 未注册", name)
	}

	typed, ok := service.(T)
	if !ok {
		panic(fmt.Sprintf("能力 %s 的类型是 %T，与要取的 %T 不符", name, service, missing))
	}
	return typed, nil
}

// OnCleanup 登记收摊函数：起了 goroutine、开了句柄，在这里还。
func (a *App) OnCleanup(fn func()) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		panic("App 已关闭，不能再登记收摊函数")
	}
	a.cleanups = append(a.cleanups, fn)
}

// Close 逆序执行全部收摊函数；幂等；单个崩溃被隔离，不挡其余。
func (a *App) Close() {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	a.closed = true
	cleanups := a.cleanups
	a.cleanups = nil
	a.mu.Unlock()

	for i := len(cleanups) - 1; i >= 0; i-- {
		a.runCleanup(cleanups[i])
	}
}

// runCleanup 执行一个收摊函数，隔离其崩溃。
func (a *App) runCleanup(fn func()) {
	defer func() {
		r := recover()
		if r != nil {
			a.warn(fmt.Sprintf("收摊函数崩溃，已隔离：%v", r))
		}
	}()
	fn()
}
