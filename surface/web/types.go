// Package web 提供通用 Web 舞台、产品登记处与 HTTP 路由登记处。
package web

import (
	nethttp "net/http"

	"github.com/a-h/templ"
)

// 数据。Product 描述 Web 左栏中的一个产品入口。
type Product struct {
	ID       string
	Name     string
	Icon     string
	BasePath string
	Order    int
}

// 数据。一个已登记设置栏目的静态身份。
type SettingsSectionDefinition struct {
	ID    string
	Title string
	Order int
}

// 契约。一个可填入 Web 公共设置页的插件栏目。
type SettingsSection interface {
	Definition() SettingsSectionDefinition
	Render() (templ.Component, error)
}

// 契约。Service 是 Web 暴露给静态产品插件与设置栏目的登记口。
type Service interface {
	RegisterProduct(product Product) error
	RegisterRoute(pattern string, handler nethttp.Handler) error
	Products() []Product
	RegisterSettingsSection(section SettingsSection) error
	SettingsSections() []SettingsSectionDefinition
	SettingsSection(id string) (SettingsSection, bool)
}

// 数据。Config 控制 Web 服务器监听地址。
type Config struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}
