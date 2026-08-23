package core

import (
	"errors"
	"fmt"
	"sync"
)

// Listener 收到一个事件负载；只做观察，无返回值、无错误。
type Listener func(payload any)

// Middleware 是链上的一层拦截：拿本次输入，调 next 放行（可传改写后的值），
// 不调 next 即拦截短路，返回自己的结果。
type Middleware[P, R any] func(payload P, next func(P) R) R

// Task 是一个可失败的任务：返回非 nil error 即失败。
type Task func(payload any) error

// Subscribe 登记观察者，从下次广播起按注册顺序被调用；随时可挂，含运行期。
func (a *App) Subscribe(name string, listener Listener) {
	a.mu.Lock()
	a.listeners[name] = append(a.listeners[name], listener)
	a.mu.Unlock()
}

// Broadcast 在调用者自己的 goroutine 里，按注册顺序同步通知全部同名观察者。
// 观察者崩溃被隔离为告警；不起 goroutine、不等结果、不传错误。
// 子作用域广播完会继续上浮，父的观察者也听得见；父广播不下沉。
func (a *App) Broadcast(name string, payload any) {
	for _, listener := range a.snapshotListeners(name) {
		a.runListener(name, listener, payload)
	}
	if a.parent != nil {
		a.parent.Broadcast(name, payload)
	}
}

// snapshotListeners 拷一份当前观察者名单再还锁，回调里再订阅、再广播都不会死锁。
func (a *App) snapshotListeners(name string) []Listener {
	a.mu.Lock()
	defer a.mu.Unlock()

	snapshot := make([]Listener, len(a.listeners[name]))
	copy(snapshot, a.listeners[name])
	return snapshot
}

// runListener 调用单个观察者，隔离其崩溃。
func (a *App) runListener(name string, listener Listener, payload any) {
	defer func() {
		r := recover()
		if r != nil {
			a.warn(fmt.Sprintf("事件 %s 的观察者崩溃，已隔离：%v", name, r))
		}
	}()
	listener(payload)
}

// Intercept 往链上挂一层中间件；先挂的在最外层，最后到达 body。
func Intercept[P, R any](a *App, name string, middleware Middleware[P, R]) {
	a.mu.Lock()
	a.middlewares[name] = append(a.middlewares[name], middleware)
	a.mu.Unlock()
}

// RunChain 沿中间件由外到内执行，最内层是 body；无中间件时等价于直接调 body。
// 错误和 panic 原样上抛，不吞——安全场景失败即拒。
func RunChain[P, R any](a *App, name string, payload P, body func(P) R) R {
	middlewares := snapshotMiddlewares[P, R](a, name)

	run := body
	for i := len(middlewares) - 1; i >= 0; i-- {
		outer := middlewares[i]
		inner := run
		run = func(p P) R {
			return outer(p, inner)
		}
	}
	return run(payload)
}

// snapshotMiddlewares 拷出断言成 Middleware[P,R] 的名单（包级函数：方法不许带类型参数）；
// 类型不符 panic——同名事件的链必须类型一致，混用是编程错误。
func snapshotMiddlewares[P, R any](a *App, name string) []Middleware[P, R] {
	a.mu.Lock()
	defer a.mu.Unlock()

	stored := a.middlewares[name]
	middlewares := make([]Middleware[P, R], len(stored))
	for i, middleware := range stored {
		typed, ok := middleware.(Middleware[P, R])
		if !ok {
			panic(fmt.Sprintf("事件 %s 的第 %d 层中间件类型不符", name, i+1))
		}
		middlewares[i] = typed
	}
	return middlewares
}

// RegisterTask 登记任务，按注册顺序参与 RunSequentially / RunConcurrently。
func (a *App) RegisterTask(name string, task Task) {
	a.mu.Lock()
	a.tasks[name] = append(a.tasks[name], task)
	a.mu.Unlock()
}

// RunSequentially 按注册顺序逐个执行任务，第一个失败即中断并返回其错误。
// 任务的 panic 原样上抛，不吞。
func RunSequentially(a *App, name string, payload any) error {
	for _, task := range a.snapshotTasks(name) {
		err := task(payload)
		if err != nil {
			return fmt.Errorf("事件 %s 的任务失败：%w", name, err)
		}
	}
	return nil
}

// RunConcurrently 并发执行全部任务，等全部结束后聚合所有失败返回。
// 单个任务崩溃转为一条失败收进聚合，不炸进程。
func RunConcurrently(a *App, name string, payload any) error {
	tasks := a.snapshotTasks(name)

	var wg sync.WaitGroup
	failures := make([]error, len(tasks))
	for i, task := range tasks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			failures[i] = runTask(task, payload)
		}()
	}
	wg.Wait()
	return errors.Join(failures...)
}

// snapshotTasks 拷一份当前任务名单再还锁。
func (a *App) snapshotTasks(name string) []Task {
	a.mu.Lock()
	defer a.mu.Unlock()

	snapshot := make([]Task, len(a.tasks[name]))
	copy(snapshot, a.tasks[name])
	return snapshot
}

// runTask 执行单个任务，把崩溃转为失败。
func runTask(task Task, payload any) (err error) {
	defer func() {
		r := recover()
		if r != nil {
			err = fmt.Errorf("任务崩溃，已转为失败：%v", r)
		}
	}()
	return task(payload)
}
