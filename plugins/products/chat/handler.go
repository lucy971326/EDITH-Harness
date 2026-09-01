package chat

import (
	nethttp "net/http"

	"github.com/a-h/templ"

	"harness/surface/web"
)

// 活对象。PageHandler 渲染 Chat 完整页面或 HTMX 主区域片段。
type PageHandler struct {
	web     web.Service
	product web.Product
}

func newPageHandler(webService web.Service, product web.Product) *PageHandler {
	return &PageHandler{web: webService, product: product}
}

func (h *PageHandler) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	content := web.Main(ChatPage())
	if r.Header.Get("HX-Request") != "true" {
		content = web.Page(h.web.Products(), h.product.ID, content)
	}
	templ.Handler(content).ServeHTTP(w, r)
}
