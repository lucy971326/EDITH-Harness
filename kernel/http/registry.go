package http

import (
	"fmt"
	nethttp "net/http"
)

// 活对象。挂在 Host 的 http 键上的路径登记处。
type Registry struct {
	mux *nethttp.ServeMux
}

func newRegistry() *Registry {
	return &Registry{mux: nethttp.NewServeMux()}
}

// Register 登记一条标准 net/http 路由。
func (r *Registry) Register(pattern string, handler nethttp.Handler) (err error) {
	if pattern == "" {
		return fmt.Errorf("http: empty pattern")
	}
	if handler == nil {
		return fmt.Errorf("http: register %q with nil handler", pattern)
	}

	defer recoverRegister(pattern, &err)
	r.mux.Handle(pattern, handler)
	return nil
}

// ServeHTTP 把请求交给已登记的路由。
func (r *Registry) ServeHTTP(w nethttp.ResponseWriter, request *nethttp.Request) {
	r.mux.ServeHTTP(w, request)
}

func recoverRegister(pattern string, err *error) {
	recovered := recover()
	if recovered == nil {
		return
	}
	*err = fmt.Errorf("http: register %q: %v", pattern, recovered)
}
