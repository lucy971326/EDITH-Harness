package core

import "testing"

func TestForAgentInheritsParentServices(t *testing.T) {
	root := New()
	t.Cleanup(root.Close)
	root.RegisterService("tools", &fakeRegistry{})

	agent := root.ForAgent("小红")

	_, err := Resolve[*fakeRegistry](agent, "tools")
	if err != nil {
		t.Fatalf("子作用域应继承父的服务：%v", err)
	}
}

func TestChildServiceShadowsParent(t *testing.T) {
	root := New()
	t.Cleanup(root.Close)
	root.RegisterService("tools", &fakeRegistry{})

	agent := root.ForAgent("小红")
	sandbox := &fakeRegistry{}
	agent.RegisterService("tools", sandbox)

	got, err := Resolve[*fakeRegistry](agent, "tools")
	if err != nil {
		t.Fatalf("遮蔽后取用不该报错：%v", err)
	}
	if got != sandbox {
		t.Fatal("子注册的同名应遮蔽父的")
	}

	parentGot, err := Resolve[*fakeRegistry](root, "tools")
	if err != nil {
		t.Fatalf("父的取用不该受影响：%v", err)
	}
	if parentGot == sandbox {
		t.Fatal("父取到的必须是父自己的那份")
	}
}

func TestRestrictBlocksInheritedButNotOwn(t *testing.T) {
	root := New()
	t.Cleanup(root.Close)
	root.RegisterService("tools", &fakeRegistry{})
	root.RegisterService("llm", &fakeRegistry{})

	agent := root.ForAgent("小红")
	agent.Restrict("tools")

	_, err := Resolve[*fakeRegistry](agent, "tools")
	if err == nil {
		t.Fatal("被裁的能力不该再查到")
	}
	_, err = Resolve[*fakeRegistry](agent, "llm")
	if err != nil {
		t.Fatalf("没裁的应照常继承：%v", err)
	}

	// 裁剪挡不住自己注册的：显式给一份沙箱版，就能用。
	sandbox := &fakeRegistry{}
	agent.RegisterService("tools", sandbox)
	got, err := Resolve[*fakeRegistry](agent, "tools")
	if err != nil || got != sandbox {
		t.Fatalf("自己注册的应绕过裁剪：err=%v", err)
	}

	_, err = Resolve[*fakeRegistry](root, "tools")
	if err != nil {
		t.Fatalf("子的裁剪不该影响父：%v", err)
	}
}

func TestRestrictDoesNotAffectSiblingAgents(t *testing.T) {
	root := New()
	t.Cleanup(root.Close)
	root.RegisterService("tools", &fakeRegistry{})

	小红 := root.ForAgent("小红")
	小红.Restrict("tools")
	小刚 := root.ForAgent("小刚")

	_, err := Resolve[*fakeRegistry](小刚, "tools")
	if err != nil {
		t.Fatalf("兄弟作用域不该被牵连：%v", err)
	}
}

func TestGrandchildInheritsThroughChain(t *testing.T) {
	root := New()
	t.Cleanup(root.Close)
	root.RegisterService("llm", &fakeRegistry{})

	agent := root.ForAgent("小红")
	session := agent.ForAgent("小红的会话")

	_, err := Resolve[*fakeRegistry](session, "llm")
	if err != nil {
		t.Fatalf("孙作用域应沿链继承到根：%v", err)
	}
}

func TestBroadcastBubblesUpToParentListeners(t *testing.T) {
	root := New()
	t.Cleanup(root.Close)
	agent := root.ForAgent("小红")

	var order []string
	agent.Subscribe("记账", func(payload any) { order = append(order, "子的观察者") })
	root.Subscribe("记账", func(payload any) { order = append(order, "父的观察者") })

	agent.Broadcast("记账", nil)

	if len(order) != 2 || order[0] != "子的观察者" || order[1] != "父的观察者" {
		t.Fatalf("子广播应先通知自己再上浮到父：got %v", order)
	}
}

func TestParentBroadcastDoesNotReachChildListeners(t *testing.T) {
	root := New()
	t.Cleanup(root.Close)
	agent := root.ForAgent("小红")

	childHeard := false
	agent.Subscribe("记账", func(payload any) { childHeard = true })

	root.Broadcast("记账", nil)

	if childHeard {
		t.Fatal("父广播不该下沉到子")
	}
}

func TestChildCloseDropsItsServicesAndListeners(t *testing.T) {
	root := New()
	t.Cleanup(root.Close)
	root.RegisterService("tools", &fakeRegistry{})

	agent := root.ForAgent("小红")
	agent.RegisterService("私有能力", &fakeRegistry{})

	childHeard := false
	parentHeard := false
	agent.Subscribe("记账", func(payload any) { childHeard = true })
	root.Subscribe("记账", func(payload any) { parentHeard = true })

	agent.Close()

	_, err := Resolve[*fakeRegistry](agent, "私有能力")
	if err == nil {
		t.Fatal("子关闭后它注册的服务应消失")
	}
	_, err = Resolve[*fakeRegistry](root, "tools")
	if err != nil {
		t.Fatalf("父应毫发无损：%v", err)
	}

	root.Broadcast("记账", nil)
	if childHeard {
		t.Fatal("子关闭后它的观察者应整批消失")
	}
	if !parentHeard {
		t.Fatal("父的观察者应照常工作")
	}
}

func TestChildCloseRunsItsOwnCleanups(t *testing.T) {
	root := New()
	t.Cleanup(root.Close)
	agent := root.ForAgent("小红")

	var order []string
	agent.OnCleanup(func() { order = append(order, "子的收摊") })
	root.OnCleanup(func() { order = append(order, "父的收摊") })

	agent.Close()

	if len(order) != 1 || order[0] != "子的收摊" {
		t.Fatalf("子 Close 只收自己的摊：got %v", order)
	}
}
