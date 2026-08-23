package core

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRunChainRunsBodyDirectlyWhenNoMiddleware(t *testing.T) {
	app := New()
	t.Cleanup(app.Close)

	got := RunChain[string, int](app, "无中间件的链", "abc", func(p string) int {
		return len(p)
	})

	if got != 3 {
		t.Fatalf("无中间件时应直达 body：got %d", got)
	}
}

func TestRunChainLayersRunOutsideIn(t *testing.T) {
	app := New()
	t.Cleanup(app.Close)

	var order []string
	Intercept(app, "写入文件", func(p string, next func(string) string) string {
		order = append(order, "外层：守卫")
		return next(p)
	})
	Intercept(app, "写入文件", func(p string, next func(string) string) string {
		order = append(order, "内层：记账")
		return next(p)
	})

	RunChain(app, "写入文件", "a.txt", func(p string) string {
		order = append(order, "body：本体")
		return p
	})

	want := []string{"外层：守卫", "内层：记账", "body：本体"}
	if len(order) != len(want) {
		t.Fatalf("层数不对：got %v", order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("应先注册的在外层：got %v, want %v", order, want)
		}
	}
}

func TestRunChainMiddlewareCanRewritePayload(t *testing.T) {
	app := New()
	t.Cleanup(app.Close)

	Intercept(app, "打招呼", func(p string, next func(string) string) string {
		return next(p + "，注意礼貌")
	})

	var received string
	RunChain(app, "打招呼", "在吗", func(p string) string {
		received = p
		return p
	})

	if received != "在吗，注意礼貌" {
		t.Fatalf("改写应传到 body：got %q", received)
	}
}

func TestRunChainMiddlewareCanShortCircuit(t *testing.T) {
	app := New()
	t.Cleanup(app.Close)

	Intercept(app, "危险操作", func(p string, next func(string) string) string {
		if p == "rm -rf /" {
			return "拒绝：太危险"
		}
		return next(p)
	})

	bodyRan := false
	got := RunChain(app, "危险操作", "rm -rf /", func(p string) string {
		bodyRan = true
		return "执行了"
	})

	if bodyRan {
		t.Fatal("拦截时 body 不该执行")
	}
	if got != "拒绝：太危险" {
		t.Fatalf("应返回拦截者的结果：got %q", got)
	}
}

func TestRunChainPropagatesPanic(t *testing.T) {
	app := New()
	t.Cleanup(app.Close)

	Intercept(app, "会炸的链", func(p string, next func(string) string) string {
		panic("中间件崩了")
	})

	defer func() {
		r := recover()
		if r != "中间件崩了" {
			t.Fatalf("panic 应原样上抛，got %v", r)
		}
	}()
	RunChain(app, "会炸的链", "x", func(p string) string { return p })
	t.Fatal("走到这里说明 panic 被吞了")
}

func TestRunChainPanicsOnMiddlewareTypeMismatch(t *testing.T) {
	app := New()
	t.Cleanup(app.Close)

	Intercept[string, string](app, "类型混用", func(p string, next func(string) string) string {
		return next(p)
	})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("同链类型不符应 panic")
		}
	}()
	RunChain[int, int](app, "类型混用", 1, func(p int) int { return p })
	t.Fatal("走到这里说明 panic 没发生")
}

func TestRunSequentiallyRunsAllInOrder(t *testing.T) {
	app := New()
	t.Cleanup(app.Close)

	var order []string
	app.RegisterTask("压缩前", func(payload any) error {
		order = append(order, "清缓存")
		return nil
	})
	app.RegisterTask("压缩前", func(payload any) error {
		order = append(order, "存快照")
		return nil
	})

	err := RunSequentially(app, "压缩前", nil)
	if err != nil {
		t.Fatalf("全部成功不该报错：%v", err)
	}
	if len(order) != 2 || order[0] != "清缓存" || order[1] != "存快照" {
		t.Fatalf("应按注册顺序执行：got %v", order)
	}
}

func TestRunSequentiallyStopsAtFirstError(t *testing.T) {
	app := New()
	t.Cleanup(app.Close)

	boom := errors.New("磁盘满了")
	var laterRan bool
	app.RegisterTask("收尾", func(payload any) error { return nil })
	app.RegisterTask("收尾", func(payload any) error { return boom })
	app.RegisterTask("收尾", func(payload any) error {
		laterRan = true
		return nil
	})

	err := RunSequentially(app, "收尾", nil)
	if !errors.Is(err, boom) {
		t.Fatalf("应返回失败任务的错误：got %v", err)
	}
	if laterRan {
		t.Fatal("失败后不该继续执行后面的任务")
	}
	if !strings.Contains(err.Error(), "收尾") {
		t.Fatalf("报错应点名事件：%v", err)
	}
}

func TestRunConcurrentlyRunsAllAndWaits(t *testing.T) {
	app := New()
	t.Cleanup(app.Close)

	// 每个任务慢一拍再报到 finished：返回后全部就位，才证明"等全部完成"。
	finished := make(chan int, 3)
	for i := 0; i < 3; i++ {
		app.RegisterTask("并行核对", func(payload any) error {
			time.Sleep(5 * time.Millisecond)
			finished <- 1
			return nil
		})
	}

	err := RunConcurrently(app, "并行核对", nil)
	if err != nil {
		t.Fatalf("全部成功不该报错：%v", err)
	}
	if len(finished) != 3 {
		t.Fatalf("返回时应等全部任务完成：got %d/3", len(finished))
	}
}

func TestRunConcurrentlyAggregatesAllFailures(t *testing.T) {
	app := New()
	t.Cleanup(app.Close)

	app.RegisterTask("并行核对", func(payload any) error {
		return errors.New("第一个失败")
	})
	app.RegisterTask("并行核对", func(payload any) error { return nil })
	app.RegisterTask("并行核对", func(payload any) error {
		return errors.New("第二个失败")
	})

	err := RunConcurrently(app, "并行核对", nil)
	if err == nil {
		t.Fatal("有失败就该返回 error")
	}
	if !strings.Contains(err.Error(), "第一个失败") || !strings.Contains(err.Error(), "第二个失败") {
		t.Fatalf("聚合错误应包含全部失败：%v", err)
	}
}

func TestRunConcurrentlyTurnsPanicIntoFailure(t *testing.T) {
	app := New()
	t.Cleanup(app.Close)

	app.RegisterTask("并行核对", func(payload any) error {
		panic("任务炸了")
	})
	app.RegisterTask("并行核对", func(payload any) error {
		return errors.New("正常失败")
	})

	// 走到断言就证明 panic 没炸进程。
	err := RunConcurrently(app, "并行核对", nil)
	if err == nil {
		t.Fatal("崩溃加失败都该聚合成 error")
	}
	if !strings.Contains(err.Error(), "任务炸了") || !strings.Contains(err.Error(), "正常失败") {
		t.Fatalf("崩溃应转为失败收进聚合：%v", err)
	}
}
