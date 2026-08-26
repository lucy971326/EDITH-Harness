package loop

import "context"

// State 返回 "idle"（闲）、"starting"（准备中）或 "busy"（运行中）。
func (c *Conversation) State() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.busy {
		return "busy"
	}
	if c.working {
		return "starting"
	}
	return "idle"
}

// WaitIdle 一直等到这段会话把手上的活干完：不在干活、且待办队列空。
// 只看"忙"不够——消息刚投进来还没开跑的那一瞬也是"不忙"，等了等于没等。
func (c *Conversation) WaitIdle() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for c.working || c.busy || c.inbox.pending() {
		c.cond.Wait()
	}
}

// Cancel 取消当前正在跑的步（模型请求、工具执行都会收到取消信号）。
// 已吐的字和工具的善后按账本规矩走，绝不装作没发生。
func (c *Conversation) Cancel() {
	c.mu.Lock()
	stop := c.stepStop
	if stop == nil && (c.working || c.busy) {
		c.cancelRequested = true
	}
	c.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// stepContext 拿当前步的取消口；每步一个，Cancel 掐的就是它。
func (c *Conversation) stepContext() context.Context {
	ctx, stop := context.WithCancel(context.Background())
	c.mu.Lock()
	cancelBeforeStart := c.cancelRequested
	c.cancelRequested = false
	c.stepCtx = ctx
	c.stepStop = stop
	c.mu.Unlock()
	if cancelBeforeStart {
		stop()
	}
	return ctx
}

func (c *Conversation) clearStepContext() {
	c.mu.Lock()
	c.stepCtx = nil
	c.stepStop = nil
	c.mu.Unlock()
}

// markWorking 先占住待办，防止队列刚领空时 WaitIdle 误判已经干完。
func (c *Conversation) markWorking() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.working = true
}

// markBusy 报告一轮已经正式开跑。
func (c *Conversation) markBusy() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.busy = true
}

// markTurnDone 报告这一轮结束；搬运工可能还要接着办下一轮。
func (c *Conversation) markTurnDone() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.busy = false
}

// markIdle 报闲并广播：所有等闲的人一起醒。
func (c *Conversation) markIdle() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.working = false
	c.busy = false
	c.cancelRequested = false
	c.cond.Broadcast()
}
