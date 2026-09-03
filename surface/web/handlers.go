package web

import (
	nethttp "net/http"

	"github.com/a-h/templ"
)

// 活对象。RootHandler 把应用根路径送到第一个已登记产品。
type RootHandler struct {
	products Service
}

func newRootHandler(products Service) *RootHandler {
	return &RootHandler{products: products}
}

// ServeHTTP 重定向到显示顺序中的第一个产品。
func (h *RootHandler) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	products := h.products.Products()
	if len(products) == 0 {
		nethttp.Error(w, "no web products registered", nethttp.StatusServiceUnavailable)
		return
	}
	nethttp.Redirect(w, r, products[0].BasePath, nethttp.StatusTemporaryRedirect)
}

// 活对象。SettingsHandler 处理 Web 公共设置页及其插件栏目的渲染。
type SettingsHandler struct {
	service Service
}

func newSettingsHandler(service Service) *SettingsHandler {
	return &SettingsHandler{service: service}
}

// ServeHTTP 渲染设置主页或插件设置片段。
func (h *SettingsHandler) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	sections := h.service.SettingsSections()
	targetID := r.PathValue("sectionID")
	if targetID == "" {
		targetID = appearanceSectionID
	}

	var content templ.Component
	if targetID == appearanceSectionID {
		content = SettingsAppearance()
	} else {
		section, ok := h.service.SettingsSection(targetID)
		if !ok {
			content = SettingsNotFound()
		} else {
			var err error
			content, err = section.Render()
			if err != nil || content == nil {
				content = SettingsRenderError()
			}
		}
	}

	if r.Header.Get("HX-Target") == "settings-content" {
		fragment := SettingsContentFragment(sections, targetID, content)
		templ.Handler(fragment).ServeHTTP(w, r)
		return
	}

	settingsPage := SettingsPage(sections, targetID, content)
	if r.Header.Get("HX-Request") == "true" {
		fragment := ProductFragment(templ.NopComponent, settingsPage)
		templ.Handler(fragment).ServeHTTP(w, r)
		return
	}

	fullPage := Page(h.service.Products(), "settings", templ.NopComponent, settingsPage)
	templ.Handler(fullPage).ServeHTTP(w, r)
}
