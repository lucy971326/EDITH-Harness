//go:build !linux

package desktop

func notificationDaemonReady() bool { return true }
