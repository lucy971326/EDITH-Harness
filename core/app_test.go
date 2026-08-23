package core

import (
	"errors"
	"strings"
	"testing"
)

// silentWarn 把告警收进切片，既静音测试输出，又留给人断言。
func silentWarn(a *App) *[]string {
	warnings := &[]string{}
	a.warn = func(message string) {
		*warnings = append(*warnings, message)
	}
	return warnings
}

// fakePlugin 是测试用的插件：可选地干点活，可选地启动失败。
type fakePlugin struct {
	name     string
	startErr error
	onStart  func(app *App)
}

func (p *fakePlugin) Name() string {
	return p.name
}

func (p *fakePlugin) Start(app *App) error {
	if p.onStart != nil {
		p.onStart(app)
	}
	return p.startErr
}

func TestResolveReturnsRegisteredService(t *testing.T) {
	app := New()
	t.Cleanup(app.Close)

	reg := &fakeRegistry{}
	app.RegisterService("tools", reg)

	got, err := Resolve[*fakeRegistry](app, "tools")
	if err != nil {
		t.Fatalf("取已注册的服务不该报错：%v", err)
	}
	if got != reg {
		t.Fatalf("取回的不是同一个对象：got %p, want %p", got, reg)
	}
}

func TestResolveErrorsOnMissingService(t *testing.T) {
	app := New()
	t.Cleanup(app.Close)

	_, err := Resolve[*fakeRegistry](app, "tools")
	if err == nil {
		t.Fatal("取不存在的服务应该报 error")
	}
	if !strings.Contains(err.Error(), "tools") {
		t.Fatalf("报错信息应提到缺的能力名：%v", err)
	}
}

func TestResolvePanicsOnWrongType(t *testing.T) {
	app := New()
	t.Cleanup(app.Close)

	app.RegisterService("tools", &fakeRegistry{})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("类型不匹配应该 panic，这是框架编程错误")
		}
	}()
	_, _ = Resolve[string](app, "tools")
	t.Fatal("走到这里说明 panic 没发生")
}

func TestRegisterServicePanicsOnDuplicate(t *testing.T) {
	app := New()
	t.Cleanup(app.Close)

	app.RegisterService("tools", &fakeRegistry{})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("重复注册同名能力应该 panic，这是组装错误")
		}
	}()
	app.RegisterService("tools", &fakeRegistry{})
	t.Fatal("走到这里说明 panic 没发生")
}

func TestCloseRunsCleanupsInReverseOrder(t *testing.T) {
	app := New()

	var order []string
	app.OnCleanup(func() { order = append(order, "第一个挂的") })
	app.OnCleanup(func() { order = append(order, "第二个挂的") })
	app.OnCleanup(func() { order = append(order, "第三个挂的") })

	app.Close()

	want := []string{"第三个挂的", "第二个挂的", "第一个挂的"}
	if len(order) != len(want) {
		t.Fatalf("收摊执行数不对：got %v", order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("收摊应逆序执行：got %v, want %v", order, want)
		}
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	app := New()

	calls := 0
	app.OnCleanup(func() { calls++ })

	app.Close()
	app.Close()

	if calls != 1 {
		t.Fatalf("重复 Close 不该重复收摊：got %d 次", calls)
	}
}

func TestCloseIsolatesPanickingCleanup(t *testing.T) {
	app := New()
	warnings := silentWarn(app)

	var laterRan bool
	app.OnCleanup(func() { panic("第一个收摊炸了") })
	app.OnCleanup(func() { laterRan = true })

	app.Close()

	if !laterRan {
		t.Fatal("一个收摊函数崩溃，不该挡住它前面的收摊")
	}
	if len(*warnings) == 0 {
		t.Fatal("收摊崩溃应记一条告警")
	}
}

func TestInstallRollsBackStartedPluginsOnFailure(t *testing.T) {
	app := New()
	warnings := silentWarn(app)
	t.Cleanup(app.Close)

	var order []string
	okay := &fakePlugin{
		name: "好的插件",
		onStart: func(app *App) {
			app.OnCleanup(func() { order = append(order, "好的插件收摊") })
		},
	}
	broken := &fakePlugin{
		name:     "坏插件",
		startErr: errors.New("端口被占"),
	}
	never := &fakePlugin{name: "轮不到的插件"}

	err := app.Install(okay, broken, never)
	if err == nil {
		t.Fatal("中途失败应该返回 error")
	}
	if !strings.Contains(err.Error(), "坏插件") {
		t.Fatalf("报错应指明是哪个插件失败：%v", err)
	}
	if len(order) != 1 || order[0] != "好的插件收摊" {
		t.Fatalf("失败时应逆序回滚已启动插件：got %v", order)
	}
	if len(*warnings) != 0 {
		t.Fatalf("回滚本身不该产生告警：%v", *warnings)
	}
}

func TestInstallSucceedsWithoutTouchingRollback(t *testing.T) {
	app := New()
	t.Cleanup(app.Close)

	first := &fakePlugin{
		name: "第一个",
		onStart: func(app *App) {
			app.OnCleanup(func() {})
		},
	}
	second := &fakePlugin{name: "第二个"}

	err := app.Install(first, second)
	if err != nil {
		t.Fatalf("全部启动成功不该报错：%v", err)
	}
}

// fakeRegistry 是服务表测试用的占位能力对象；tag 让实例指针各不相同
// （空结构体的地址全相同，没法用指针分辨两个实例）。
type fakeRegistry struct {
	tag string
}
