package session

import (
	"errors"

	"harness/core"
)

// Plugin 把账本管家装进场地（能力名 "sessions"）。
// Journal 是长期配置：决定账写到哪里；session 是必装的第一个插件。
type Plugin struct {
	Journal Journal // 账本落盘器；实现可以替换
}

// Name 返回插件名。
func (Plugin) Name() string {
	return "session"
}

// Start 用注入的 Journal 建账本管家并登记到场地。
// Journal 缺失表示组装不完整，返回 error；Store 的类型错由 core.Resolve 负责 panic。
func (p Plugin) Start(app *core.App) error {
	if p.Journal == nil {
		return errors.New("session 缺少 Journal")
	}

	store := NewStore(p.Journal, app)
	app.RegisterService("sessions", store)
	return nil
}

// Get 从场地取账本管家。
func Get(app *core.App) (*Store, error) {
	return core.Resolve[*Store](app, "sessions")
}
