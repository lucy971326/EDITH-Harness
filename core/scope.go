package core

// ForChild 从当前作用域派生一个子作用域。名字只用于诊断；子作用域继承父能力，
// 关闭后只收自己的东西，不影响父作用域。
func (a *App) ForChild(name string) *App {
	return &App{
		parent:      a,
		scopeName:   name,
		services:    make(map[string]any),
		listeners:   make(map[string][]Listener),
		middlewares: make(map[string][]any),
		tasks:       make(map[string][]Task),
		restricted:  make(map[string]bool),
		warn:        a.warn,
	}
}

// Parent 返回父作用域；全局根返回 nil。
// 流水线一类的跨层逻辑靠它沿链聚合（父层在外、子层在内）。
func (a *App) Parent() *App {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.parent
}

// Restrict 裁掉若干能力名：之后从父继承的这些名字查不到。
// 裁不到自己注册的——那叫替换，不叫裁剪。
func (a *App) Restrict(names ...string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, name := range names {
		a.restricted[name] = true
	}
}

// lookupService 查一个能力：先自己的表（命中即遮蔽父的）；
// 被裁的名字不再向上问；其余沿父链递归到根。
func (a *App) lookupService(name string) (any, bool) {
	a.mu.Lock()
	service, exists := a.services[name]
	blocked := a.restricted[name]
	a.mu.Unlock()

	if exists {
		return service, true
	}
	if blocked || a.parent == nil {
		return nil, false
	}
	return a.parent.lookupService(name)
}
