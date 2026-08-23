package core

// ForAgent 从当前作用域派生一个子作用域，给一个 agent 专用。
// 子继承父的服务表（同名可遮蔽），可用 Restrict 裁掉继承项；
// 子广播的事件上浮到父的观察者；子 Close 后这一切随它消失，父毫发无损。
func (a *App) ForAgent(agentName string) *App {
	return &App{
		parent:      a,
		agentName:   agentName,
		services:    make(map[string]any),
		listeners:   make(map[string][]Listener),
		middlewares: make(map[string][]any),
		tasks:       make(map[string][]Task),
		restricted:  make(map[string]bool),
		warn:        a.warn,
	}
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
