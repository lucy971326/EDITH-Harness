package web

import (
	"fmt"
	nethttp "net/http"
	"sort"
	"sync"
)

// 活对象。Registry 保存 Web 服务暴露的产品名单、设置栏目和 HTTP 路由表。
type Registry struct {
	productsMu sync.RWMutex
	products   map[string]Product
	paths      map[string]string
	sections   *settingsSectionRegistry
	mux        *nethttp.ServeMux
}

func newRegistry() *Registry {
	return &Registry{
		products: make(map[string]Product),
		paths:    make(map[string]string),
		sections: newSettingsSectionRegistry(),
		mux:      nethttp.NewServeMux(),
	}
}

// RegisterProduct 往 Web 左栏登记一个产品入口。
func (r *Registry) RegisterProduct(product Product) error {
	if product.ID == "" {
		return fmt.Errorf("web: product ID is empty")
	}
	if product.Name == "" {
		return fmt.Errorf("web: product %q name is empty", product.ID)
	}
	if product.BasePath == "" || product.BasePath[0] != '/' {
		return fmt.Errorf("web: product %q has invalid base path %q", product.ID, product.BasePath)
	}

	r.productsMu.Lock()
	defer r.productsMu.Unlock()
	if _, exists := r.products[product.ID]; exists {
		return fmt.Errorf("web: product %q already registered", product.ID)
	}
	if owner, exists := r.paths[product.BasePath]; exists {
		return fmt.Errorf("web: product base path %q already used by %q", product.BasePath, owner)
	}
	r.products[product.ID] = product
	r.paths[product.BasePath] = product.ID
	return nil
}

// RegisterRoute 登记一条标准 net/http 路由。
func (r *Registry) RegisterRoute(pattern string, handler nethttp.Handler) (err error) {
	if pattern == "" {
		return fmt.Errorf("web: route pattern is empty")
	}
	if handler == nil {
		return fmt.Errorf("web: register route %q with nil handler", pattern)
	}

	defer recoverRoute(pattern, &err)
	r.mux.Handle(pattern, handler)
	return nil
}

// Products 返回按显示顺序排列的产品副本。
func (r *Registry) Products() []Product {
	r.productsMu.RLock()
	products := make([]Product, 0, len(r.products))
	for _, product := range r.products {
		products = append(products, product)
	}
	r.productsMu.RUnlock()

	sort.Slice(products, func(i, j int) bool {
		if products[i].Order == products[j].Order {
			return products[i].ID < products[j].ID
		}
		return products[i].Order < products[j].Order
	})
	return products
}

// RegisterSettingsSection 往 Web 公共设置页登记一个插件栏目。
func (r *Registry) RegisterSettingsSection(section SettingsSection) error {
	return r.sections.RegisterSettingsSection(section)
}

// SettingsSections 返回按显示顺序排列的设置栏目列表。
func (r *Registry) SettingsSections() []SettingsSectionDefinition {
	return r.sections.SettingsSections()
}

// SettingsSection 按 ID 查找已登记的设置栏目。
func (r *Registry) SettingsSection(id string) (SettingsSection, bool) {
	return r.sections.SettingsSection(id)
}

// ServeHTTP 把请求交给已登记的路由。
func (r *Registry) ServeHTTP(w nethttp.ResponseWriter, request *nethttp.Request) {
	r.mux.ServeHTTP(w, request)
}

func recoverRoute(pattern string, err *error) {
	recovered := recover()
	if recovered == nil {
		return
	}
	*err = fmt.Errorf("web: register route %q: %v", pattern, recovered)
}
