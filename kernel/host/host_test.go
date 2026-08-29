package host

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestRegisterResolve(t *testing.T) {
	h := NewHost()
	err := h.RegisterService("n", 7)
	if err != nil {
		t.Fatal(err)
	}

	got, err := Resolve[int](h, "n")
	if err != nil {
		t.Fatal(err)
	}
	if got != 7 {
		t.Fatalf("got %d, want 7", got)
	}
}

func TestRegister_duplicate(t *testing.T) {
	h := NewHost()
	err := h.RegisterService("n", 1)
	if err != nil {
		t.Fatal(err)
	}

	err = h.RegisterService("n", 2)
	if err == nil {
		t.Fatal("want error on duplicate")
	}

	got, err := Resolve[int](h, "n")
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Fatalf("got %d, want the first value", got)
	}
}

func TestRegister_emptyName(t *testing.T) {
	h := NewHost()
	err := h.RegisterService("", 1)
	if err == nil {
		t.Fatal("want error on empty name")
	}
}

func TestRegister_nil(t *testing.T) {
	h := NewHost()
	err := h.RegisterService("n", nil)
	if err == nil {
		t.Fatal("want error on nil value")
	}
}

func TestResolve_missing(t *testing.T) {
	h := NewHost()
	_, err := Resolve[int](h, "n")
	if err == nil {
		t.Fatal("want error when missing")
	}
}

func TestResolve_wrongType(t *testing.T) {
	h := NewHost()
	err := h.RegisterService("n", 7)
	if err != nil {
		t.Fatal(err)
	}

	_, err = Resolve[string](h, "n")
	if err == nil {
		t.Fatal("want error on type mismatch")
	}
}

func TestInstall_thenResolve(t *testing.T) {
	h := NewHost()
	p := &stub{
		name: "alpha",
		hang: pair{"n", 7},
	}

	err := h.Install(p)
	if err != nil {
		t.Fatal(err)
	}

	got, err := Resolve[int](h, "n")
	if err != nil {
		t.Fatal(err)
	}
	if got != 7 {
		t.Fatalf("got %d, want 7", got)
	}
}

func TestInstall_nil(t *testing.T) {
	h := NewHost()
	err := h.Install(nil)
	if err == nil {
		t.Fatal("want error on nil plugin")
	}
}

func TestInstall_failClosesAlreadyStarted(t *testing.T) {
	h := NewHost()
	var order []string
	a := &stub{name: "a", log: &order, hang: pair{"a", 1}}
	b := &stub{name: "b", log: &order, startErr: errors.New("boom")}

	err := h.Install(a)
	if err != nil {
		t.Fatal(err)
	}

	err = h.Install(b)
	if err == nil {
		t.Fatal("want start error")
	}

	want := []string{
		"start a",
		"start b",
		"close b",
		"close a",
	}
	if !slices.Equal(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}

	_, err = Resolve[int](h, "a")
	if err == nil {
		t.Fatal("want empty table after failed install")
	}
}

func TestInstall_failClearsPartialRegister(t *testing.T) {
	h := NewHost()
	p := &stub{
		name:          "bad",
		hang:          pair{"x", 1},
		startErr:      errors.New("boom"),
		failAfterHang: true,
	}

	err := h.Install(p)
	if err == nil {
		t.Fatal("want start error")
	}

	_, err = Resolve[int](h, "x")
	if err == nil {
		t.Fatal("want service cleared after failed start")
	}
}

func TestClose_reverse(t *testing.T) {
	h := NewHost()
	var order []string
	err := h.Install(&stub{name: "a", log: &order})
	if err != nil {
		t.Fatal(err)
	}
	err = h.Install(&stub{name: "b", log: &order})
	if err != nil {
		t.Fatal(err)
	}
	err = h.Install(&stub{name: "c", log: &order})
	if err != nil {
		t.Fatal(err)
	}

	err = h.Close()
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"start a",
		"start b",
		"start c",
		"close c",
		"close b",
		"close a",
	}
	if !slices.Equal(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestClose_collectsErrors(t *testing.T) {
	h := NewHost()
	var order []string
	err := h.Install(&stub{name: "a", log: &order, closeErr: errors.New("a-fail")})
	if err != nil {
		t.Fatal(err)
	}
	err = h.Install(&stub{name: "b", log: &order, closeErr: errors.New("b-fail")})
	if err != nil {
		t.Fatal(err)
	}

	err = h.Close()
	if err == nil {
		t.Fatal("want collected close errors")
	}
	if !strings.Contains(err.Error(), "a-fail") {
		t.Fatalf("missing a-fail: %v", err)
	}
	if !strings.Contains(err.Error(), "b-fail") {
		t.Fatalf("missing b-fail: %v", err)
	}

	want := []string{
		"start a",
		"start b",
		"close b",
		"close a",
	}
	if !slices.Equal(order, want) {
		t.Fatalf("still close all, got %v", order)
	}
}

func TestClose_emptyAndTwice(t *testing.T) {
	h := NewHost()
	err := h.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = h.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestClose_clearsDirectRegister(t *testing.T) {
	h := NewHost()
	err := h.RegisterService("n", 1)
	if err != nil {
		t.Fatal(err)
	}

	err = h.Close()
	if err != nil {
		t.Fatal(err)
	}

	_, err = Resolve[int](h, "n")
	if err == nil {
		t.Fatal("want empty table after close")
	}
}

type pair struct {
	name string
	v    any
}

// stub 是测试用插件。log 为 nil 时不记顺序。
type stub struct {
	name          string
	log           *[]string
	hang          pair
	startErr      error
	closeErr      error
	failAfterHang bool
}

func (p *stub) Name() string { return p.name }

func (p *stub) Start(h *Host) error {
	p.note("start " + p.name)
	if p.startErr != nil && !p.failAfterHang {
		return p.startErr
	}
	if p.hang.name != "" {
		err := h.RegisterService(p.hang.name, p.hang.v)
		if err != nil {
			return err
		}
	}
	if p.startErr != nil {
		return p.startErr
	}
	return nil
}

func (p *stub) Close() error {
	p.note("close " + p.name)
	return p.closeErr
}

func (p *stub) note(step string) {
	if p.log == nil {
		return
	}
	*p.log = append(*p.log, step)
}
