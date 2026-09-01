// Package web 提供通用 Web 舞台、产品登记处与 HTTP 路由登记处。
package web

import nethttp "net/http"

// 数据。Product 描述 Web 左栏中的一个产品入口。
type Product struct {
	ID       string
	Name     string
	Icon     string
	BasePath string
	Order    int
}

// 契约。Service 是 Web 暴露给静态产品插件的登记口。
type Service interface {
	RegisterProduct(product Product) error
	RegisterRoute(pattern string, handler nethttp.Handler) error
	Products() []Product
}

// 数据。Config 控制 Web 服务器监听地址。
type Config struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}
