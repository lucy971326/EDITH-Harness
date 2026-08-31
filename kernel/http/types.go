// Package http 提供进程内的 HTTP 路径登记处。
package http

// 数据。HTTP 服务器的监听配置。
type Config struct {
	Host string
	Port int
}
