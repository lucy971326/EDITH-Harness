package web

import (
	"fmt"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// 活对象。textHandler 是测试使用的固定响应 Handler。
type textHandler struct {
	text string
}

func (h *textHandler) ServeHTTP(w nethttp.ResponseWriter, _ *nethttp.Request) {
	_, _ = io.WriteString(w, h.text)
}

func TestRegistryProducts(t *testing.T) {
	registry := newRegistry()
	err := registry.RegisterProduct(Product{ID: "movie", Name: "电影", BasePath: "/movie", Order: 20})
	if err != nil {
		t.Fatal(err)
	}
	err = registry.RegisterProduct(Product{ID: "chat", Name: "对话", BasePath: "/chat", Order: 10})
	if err != nil {
		t.Fatal(err)
	}

	products := registry.Products()
	if len(products) != 2 || products[0].ID != "chat" || products[1].ID != "movie" {
		t.Fatalf("Products() = %#v", products)
	}
	products[0].Name = "changed"
	if registry.Products()[0].Name != "对话" {
		t.Fatal("Products returned shared state")
	}
}

func TestRegistryRejectsDuplicateProducts(t *testing.T) {
	registry := newRegistry()
	err := registry.RegisterProduct(Product{ID: "chat", Name: "对话", BasePath: "/chat"})
	if err != nil {
		t.Fatal(err)
	}
	tests := []Product{
		{ID: "chat", Name: "另一个", BasePath: "/other"},
		{ID: "other", Name: "另一个", BasePath: "/chat"},
		{ID: "", Name: "空", BasePath: "/empty"},
		{ID: "bad", Name: "", BasePath: "/bad"},
		{ID: "path", Name: "路径", BasePath: "relative"},
	}
	for _, product := range tests {
		err = registry.RegisterProduct(product)
		if err == nil {
			t.Fatalf("RegisterProduct(%#v) error = nil", product)
		}
	}
}

func TestRegistryRoutes(t *testing.T) {
	registry := newRegistry()
	err := registry.RegisterRoute("", &textHandler{})
	if err == nil {
		t.Fatal("empty route error = nil")
	}
	err = registry.RegisterRoute("GET /nil", nil)
	if err == nil {
		t.Fatal("nil handler error = nil")
	}
	err = registry.RegisterRoute("GET /chat", &textHandler{text: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	err = registry.RegisterRoute("GET /chat", &textHandler{text: "duplicate"})
	if err == nil {
		t.Fatal("duplicate route error = nil")
	}
	err = registry.RegisterRoute("GET /{", &textHandler{})
	if err == nil {
		t.Fatal("invalid route error = nil")
	}

	request := httptest.NewRequest(nethttp.MethodGet, "/chat", nil)
	response := httptest.NewRecorder()
	registry.ServeHTTP(response, request)
	if response.Code != nethttp.StatusOK || response.Body.String() != "chat" {
		t.Fatalf("GET /chat = (%d, %q)", response.Code, response.Body.String())
	}
}

func TestRegistryConcurrentRegistrationAndRequests(t *testing.T) {
	registry := newRegistry()
	err := registry.RegisterRoute("GET /stable", &textHandler{text: "stable"})
	if err != nil {
		t.Fatal(err)
	}

	var wait sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for requestNumber := 0; requestNumber < 100; requestNumber++ {
				request := httptest.NewRequest(nethttp.MethodGet, "/stable", nil)
				response := httptest.NewRecorder()
				registry.ServeHTTP(response, request)
				if response.Code != nethttp.StatusOK {
					t.Errorf("GET /stable status = %d", response.Code)
					return
				}
			}
		}()
	}
	for index := 0; index < 50; index++ {
		product := Product{
			ID:       fmt.Sprintf("product-%d", index),
			Name:     "产品",
			BasePath: fmt.Sprintf("/product/%d", index),
			Order:    index,
		}
		err = registry.RegisterProduct(product)
		if err != nil {
			t.Fatal(err)
		}
	}
	wait.Wait()
}
