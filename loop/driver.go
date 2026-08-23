package loop

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"harness/chat"
	"harness/llm"
	"harness/session"
	"harness/tools"
)

// Checkpoint 是步前检查站的链名：插件可以拒绝这批消息进上下文。
// 被拒的也留痕——关一个空轮记账，绝不静默吞掉。
const Checkpoint = "loop/pre-step"

// driver 是门面背后的搬运工：一个 goroutine，闲了等铃，铃响干活，干完再等。
type driver struct {
	agent *Agent
	stop  chan struct{}
	done  chan struct{}
}

func newDriver(agent *Agent) *driver {
	return &driver{
		agent: agent,
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
}

// start 把搬运工放上线。
func (d *driver) start() {
	go d.run()
}

// stopAndJoin 让搬运工下线并等他收工。
func (d *driver) stopAndJoin() {
	close(d.stop)
	<-d.done
}

// run 主循环：等铃 → 把活干到没有 → 报闲 → 再等。
func (d *driver) run() {
	defer close(d.done)

	for {
		select {
		case <-d.stop:
			return
		case <-d.agent.inbox.wake:
		}

		d.workOffQueue()

		// 铃响到报闲之间又来了活，接着干；确认没活才报闲。
		for d.agent.inbox.pending() {
			d.workOffQueue()
		}
		d.agent.markIdle()
	}
}

// workOffQueue 把待办队列里的轮全跑完（一条 followup 一轮）。
func (d *driver) workOffQueue() {
	for {
		first, ok := d.agent.inbox.takeNextTurn()
		if !ok {
			// 没排队的 followup，但可能有闲时塞进来的中途话——它就是新轮的开头。
			steerings := d.agent.inbox.takeNextStep()
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
	a := d.agent

	err := a.claimAsUserMessage(first)
	if err != nil {
		log.Printf("loop: 领出 %s 失败，这轮不跑：%v", first.ID, err)
		return
	}
	for _, steering := range extraSteerings {
		_ = a.claimAsUserMessage(steering)
	}

	_, err = a.book.RecordTurnStart()
	if err != nil {
		log.Printf("loop: 开轮记账失败，这轮不跑：%v", err)
		return
	}

	a.markBusy()
	defer func() {
		_, _ = a.book.RecordTurnEnd()
		a.markIdle()
	}()

	for {
		more, err := d.runStep()
		if err != nil {
			log.Printf("loop: 一步失败，收轮：%v", err)
			return
		}
		if !more {
			return
		}
	}
}

// runStep 跑一步：领中途话和小抄 → 拍快照 → 清仓 → 问模型 → 定稿 → 调工具。
// 返回"还要不要下一步"。任何出口都把这一步收口（step/end），
// 账上不留悬空的步——恢复时少猜一件事。
func (d *driver) runStep() (bool, error) {
	a := d.agent

	_, err := a.book.RecordStepStart()
	if err != nil {
		return false, err
	}
	stepOpen := true
	defer func() {
		if stepOpen {
			_, _ = a.book.RecordStepEnd()
		}
	}()

	for _, steering := range a.inbox.takeNextStep() {
		_ = a.claimAsUserMessage(steering)
	}
	memos := a.inbox.takeMemos()

	// 组装最终请求：系统提示 + 小抄 + 账本投影出的历史。
	request := llm.Request{
		Model:    a.config.Model,
		Messages: buildMessages(a.config.SystemPrompt, memos, a.book.ModelHistory()),
	}
	snapshot, err := json.Marshal(request)
	if err != nil {
		return false, err
	}
	_, err = a.book.RecordSnapshot(snapshot)
	if err != nil {
		return false, err
	}

	// 危险边界：问模型前把攒着的账全部写完——万一这之后崩了，账知道断在哪。
	err = a.book.Flush()
	if err != nil {
		return false, err
	}

	// 边吐边记：UI 的逐字显示是白送的。
	var chunkSeqs []int
	chunkText := &strings.Builder{}
	ctx := a.stepContext()
	reply, err := a.llmSvc.Stream(ctx, request, func(delta chat.Delta) {
		event, chunkErr := a.book.RecordChunk(delta.Text)
		if chunkErr != nil {
			log.Printf("loop: 记流式片段失败（账本异常，继续收字）：%v", chunkErr)
			return
		}
		chunkSeqs = append(chunkSeqs, event.Seq)
		chunkText.WriteString(delta.Text)
	})
	if err != nil {
		// 被取消：已收到的半句固化成被打断的定稿——只落一条，绝不丢字。
		if ctx.Err() != nil && len(chunkSeqs) > 0 {
			_, _ = a.book.RecordAssistantFinal(session.AssistantFinalData{
				Text:        chunkText.String(),
				Interrupted: true,
			}, chunkSeqs)
		}
		return false, fmt.Errorf("问模型失败：%w", err)
	}

	_, err = a.book.RecordAssistantFinal(session.AssistantFinalData{
		Text:  reply.Text,
		Usage: reply.Usage,
	}, chunkSeqs)
	if err != nil {
		return false, err
	}

	if len(reply.Calls) == 0 {
		stepOpen = false
		_, err = a.book.RecordStepEnd()
		return false, err
	}

	// 模型要调工具：逐个过 M3 流水线，结果都入账后才走下一步。
	for _, call := range reply.Calls {
		result := a.toolsReg.ExecuteCall(ctx, a.scope, a.book, tools.Call{
			ID:       call.ID,
			Name:     call.Name,
			Argument: call.Argument,
			Agent:    a.id,
		})
		if result.Status == tools.ResultUnknown {
			return false, fmt.Errorf("工具 %s 已开跑但结果不明，收轮待恢复", call.Name)
		}
	}

	stepOpen = false
	_, err = a.book.RecordStepEnd()
	if err != nil {
		return false, err
	}
	return true, nil
}

// buildMessages 拼最终发给模型的消息：系统提示在前、小抄其次、历史照账本。
func buildMessages(systemPrompt string, memos []delivery, history []chat.Message) []chat.Message {
	var head []chat.Message
	if systemPrompt != "" {
		head = append(head, chat.Message{Role: "system", Text: systemPrompt})
	}
	for _, memo := range memos {
		head = append(head, chat.Message{Role: "system", Text: memo.Text})
	}
	return append(head, history...)
}
