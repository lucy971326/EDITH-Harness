package loop

import (
	"log"
	"sync"
)

// driver 是门面背后的搬运工：一个 goroutine，闲了等铃，铃响干活，干完再等。
type driver struct {
	conversation *Conversation
	stop         chan struct{}
	done         chan struct{}
	startOnce    sync.Once
	stopOnce     sync.Once
}

func newDriver(conversation *Conversation) *driver {
	return &driver{
		conversation: conversation,
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
	}
}

// start 把搬运工放上线。
func (d *driver) start() {
	d.startOnce.Do(func() {
		go d.run()
	})
}

// stopAndJoin 让搬运工下线并等他收工。
func (d *driver) stopAndJoin() {
	d.start()
	d.stopOnce.Do(func() {
		close(d.stop)
	})
	<-d.done
}

// run 主循环：等铃 → 把活干到没有 → 报闲 → 再等。
func (d *driver) run() {
	defer close(d.done)

	for {
		select {
		case <-d.stop:
			return
		case <-d.conversation.inbox.wake:
		}

		// 先占住待办再领活：WaitIdle 不会在“队列刚领空、活还没开跑”的缝里早退。
		d.conversation.markWorking()
		d.workOffQueue()

		// 铃响到报闲之间又来了活，接着干；确认没活才报闲。
		for d.conversation.inbox.pending() {
			d.workOffQueue()
		}
		d.conversation.markIdle()
	}
}

// workOffQueue 把待办队列里的轮全跑完（一条 followup 一轮）。
func (d *driver) workOffQueue() {
	for {
		first, ok := d.conversation.inbox.takeNextTurn()
		if !ok {
			// 没排队的 followup，但可能有闲时塞进来的中途话——它就是新轮的开头。
			steerings := d.conversation.inbox.takeNextStep()
			if len(steerings) == 0 {
				return
			}
			d.runTurn(steerings[0], steerings[1:])
			continue
		}
		d.runTurn(first, nil)
	}
}

// runTurn 跑一轮：领出落账、开轮、一步一步走到模型不再要工具。
func (d *driver) runTurn(first delivery, extraSteerings []delivery) {
	c := d.conversation

	err := c.claimAsUserMessage(first)
	if err != nil {
		log.Printf("loop: 领出 %s 失败，这轮不跑：%v", first.ID, err)
		c.report(err)
		return
	}
	for _, steering := range extraSteerings {
		_ = c.claimAsUserMessage(steering)
	}

	c.markBusy()
	_, err = c.book.RecordTurnStart()
	if err != nil {
		c.markTurnDone()
		log.Printf("loop: 开轮记账失败，这轮不跑：%v", err)
		c.report(err)
		return
	}

	defer func() {
		c.markTurnDone()
		_, _ = c.book.RecordTurnEnd()
	}()

	for {
		more, err := d.runStep()
		if err != nil {
			log.Printf("loop: 一步失败，收轮：%v", err)
			c.report(err)
			return
		}
		if !more {
			return
		}
	}
}
