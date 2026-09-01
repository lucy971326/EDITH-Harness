package web

import (
	nethttp "net/http"
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
