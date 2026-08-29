package host

// 契约。往 Host 上挂一份能力。
// Start 里 RegisterService；Close 里放掉自己开的资源。
// Start 失败或还没 Start 时，Close 也必须安全。
// Host 本身不是 Plugin。
type Plugin interface {
	Name() string
	Start(h *Host) error
	Close() error
}
