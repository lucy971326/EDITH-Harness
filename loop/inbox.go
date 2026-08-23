// Package loop 是搬运工：不知疲倦地重复"领消息 → 问模型 → 调工具 → 再问"，直到没活。
// 它自己没有主见——拿什么、调什么、准不准，全是别人决定的。
package loop

import (
	"fmt"
	"strconv"
	"sync"

	"harness/session"
)

// 投递目标：给谁的。
const (
	TargetNextTurn = "next-turn" // 开新轮（正常的新问题）
	TargetNextStep = "next-step" // 进当前轮的下一步（中途捎话）
	TargetMemo     = "context"   // 塞小抄：不吵醒、不占待办，下次问模型时拼进上下文
)

// delivery 是收件箱里的一份投递。
type delivery struct {
	ID   string
	Text string
}

// inbox 是收件箱：三条待办队列 + 一个唤醒铃。
// 每一份投递都先落账再入队，崩溃不丢；领出时再落一笔领出账——两条账分开，
// 恢复时只重放"投递了没领出"的，绝不把用户消息弄成两份。
type inbox struct {
	mu       sync.Mutex
	nextTurn []delivery
	nextStep []delivery
	memos    []delivery
	wake     chan struct{} // 唤醒铃：有活可干时响一声
	nextID   int
}

func newInbox() *inbox {
	return &inbox{wake: make(chan struct{}, 1)}
}

// deliver 投一份：先落账，再入队，再按需响铃。
func (b *inbox) deliver(book *session.Session, target string, text string, ring bool) error {
	b.mu.Lock()
	b.nextID++
	id := fmt.Sprintf("d%d", b.nextID)
	b.mu.Unlock()

	_, err := book.RecordDeliver(id, text, target)
	if err != nil {
		return err
	}

	item := delivery{ID: id, Text: text}
	b.mu.Lock()
	switch target {
	case TargetNextTurn:
		b.nextTurn = append(b.nextTurn, item)
	case TargetNextStep:
		b.nextStep = append(b.nextStep, item)
	case TargetMemo:
		b.memos = append(b.memos, item)
	default:
		b.mu.Unlock()
		return fmt.Errorf("投递目标不认识：%s", target)
	}
	b.mu.Unlock()

	if ring {
		b.ring()
	}
	return nil
}

// takeNextTurn 领走一条开新轮的消息；没有就返回 false。
// 闲时的中途话也走这（没轮可插，就是新轮的开头）。
func (b *inbox) takeNextTurn() (delivery, bool) {
	return b.takeFrom(&b.nextTurn)
}

// takeNextStep 领走全部中途话（当前轮下一步生效）。
func (b *inbox) takeNextStep() []delivery {
	b.mu.Lock()
	defer b.mu.Unlock()

	taken := b.nextStep
	b.nextStep = nil
	return taken
}

// takeMemos 领走全部小抄。
func (b *inbox) takeMemos() []delivery {
	b.mu.Lock()
	defer b.mu.Unlock()

	taken := b.memos
	b.memos = nil
	return taken
}

// restore 恢复时把一份旧投递放回队列（账上有 deliver 无 claim 的那些）。
func (b *inbox) restore(item delivery, target string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch target {
	case TargetNextTurn:
		b.nextTurn = append(b.nextTurn, item)
	case TargetNextStep:
		b.nextStep = append(b.nextStep, item)
	case TargetMemo:
		b.memos = append(b.memos, item)
	}
}

// rememberDeliveryID 读一个旧投递编号，让新编号接着旧账往后数。
func (b *inbox) rememberDeliveryID(id string) {
	if len(id) < 2 || id[0] != 'd' {
		return
	}
	number, err := strconv.Atoi(id[1:])
	if err != nil {
		return
	}
	if number < 1 {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if number > b.nextID {
		b.nextID = number
	}
}

// pending 有没有活可干（开新轮的活）。
func (b *inbox) pending() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	return len(b.nextTurn) > 0 || len(b.nextStep) > 0
}

// ring 响铃：铃里已有一声就不重复响。
func (b *inbox) ring() {
	select {
	case b.wake <- struct{}{}:
	default:
	}
}

// takeFrom 从指定队列领走队首。
func (b *inbox) takeFrom(queue *[]delivery) (delivery, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	items := *queue
	if len(items) == 0 {
		return delivery{}, false
	}
	first := items[0]
	*queue = items[1:]
	return first, true
}
