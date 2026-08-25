package presets

import (
	"fmt"
	"sort"
	"sync"

	"harness/core"
)

// Store 是 Agent 模式版本的存取口；具体介质由持久化插件提供。
type Store interface {
	Create(revision Revision) error
	Update(revision Revision) error
	Get(id string) (Revision, error)
	GetRevision(id string, number int) (Revision, error)
	List() ([]Revision, error)
	Archive(id string) error
}

// Service 是其他插件管理 Agent 模式的公共入口。
type Service interface {
	Create(preset Preset) error
	Update(preset Preset) error
	Get(id string) (Preset, error)
	GetRevision(id string, number int) (Revision, error)
	List() ([]Preset, error)
	Archive(id string) error
}

// registry 把当前模式写入不可变版本库。
type registry struct {
	mu    sync.Mutex
	store Store
}

// New 用给定存储创建 Agent 模式管理入口。
func New(store Store) Service {
	return &registry{store: store}
}

// Create 保存一个新模式；它总是从未归档的第 1 版开始。
func (r *registry) Create(preset Preset) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	preset.Revision = 1
	preset.Archived = false
	err := Validate(preset)
	if err != nil {
		return err
	}
	return r.store.Create(clone(preset))
}

// Update 保存当前模式的下一个版本；调用者给内容，版本和归档状态沿用当前版本。
func (r *registry) Update(preset Preset) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, err := r.store.Get(preset.ID)
	if err != nil {
		return err
	}
	preset.Revision = current.Revision + 1
	preset.Archived = current.Archived
	err = Validate(preset)
	if err != nil {
		return err
	}
	return r.store.Update(clone(preset))
}

// Get 读取一个模式的当前版本副本。
func (r *registry) Get(id string) (Preset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	revision, err := r.store.Get(id)
	if err != nil {
		return Preset{}, err
	}
	return clone(revision), nil
}

// GetRevision 读取一个模式的指定历史版本副本。
func (r *registry) GetRevision(id string, number int) (Revision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	revision, err := r.store.GetRevision(id, number)
	if err != nil {
		return Revision{}, err
	}
	return clone(revision), nil
}

// List 按 id 列出每个模式的当前版本副本。
func (r *registry) List() ([]Preset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	listed, err := r.store.List()
	if err != nil {
		return nil, err
	}
	presets := make([]Preset, len(listed))
	for i, revision := range listed {
		presets[i] = clone(revision)
	}
	sort.Slice(presets, func(i int, j int) bool {
		return presets[i].ID < presets[j].ID
	})
	return presets, nil
}

// Archive 追加一个标记为归档的新版本；已归档模式保持不变。
func (r *registry) Archive(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.store.Archive(id)
}

// Plugin 把 Agent 模式管理处装进 App（能力名 "presets"）。
type Plugin struct{}

// Name 返回插件名。
func (Plugin) Name() string {
	return "presets"
}

// Start 领取模式库，再登记 Agent 模式管理能力。
func (Plugin) Start(app *core.App) error {
	store, err := core.Resolve[Store](app, "preset-store")
	if err != nil {
		return fmt.Errorf("presets 要先有 Agent 模式库（preset-store）：%w", err)
	}
	app.RegisterService("presets", New(store))
	return nil
}

// Get 从 App 取 Agent 模式管理入口。
func Get(app *core.App) (Service, error) {
	return core.Resolve[Service](app, "presets")
}
