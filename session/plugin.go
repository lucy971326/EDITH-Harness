package session

import (
	"fmt"

	"harness/core"
)

// Plugin 从持久化插件领取 Journal，再把账本管家装进场地（能力名 "sessions"）。
type Plugin struct{}

// Name 返回插件名。
func (Plugin) Name() string {
	return "session"
}

// Start 领取 Journal 建账本管家并登记到场地；缺 Journal 表示组装不完整。
func (Plugin) Start(app *core.App) error {
	journal, err := core.Resolve[Journal](app, "journal")
	if err != nil {
		return fmt.Errorf("session 缺少 Journal：%w", err)
	}
	store := NewStore(journal, app)
	app.RegisterService("sessions", store)
	return nil
}

// Get 从场地取账本管家。
func Get(app *core.App) (*Store, error) {
	return core.Resolve[*Store](app, "sessions")
}
