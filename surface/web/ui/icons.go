package ui

// IsKnownIcon 判断图标是否已经登记在公共图标目录中。
func IsKnownIcon(name IconName) bool {
	switch name {
	case IconSettings, IconPlus, IconClose, IconChat, IconPanel, IconZap:
		return true
	default:
		return false
	}
}
