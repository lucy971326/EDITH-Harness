package loop

import (
	"fmt"

	"harness/llm"
	"harness/session"
)

// SubmitFollowup 投一条开新轮的消息：忙时排队，本轮正常结束后才开新轮。
func (c *Conversation) SubmitFollowup(text string) error {
	c.mu.Lock()
	ready := c.config.Provider != "" && c.config.Model != ""
	c.mu.Unlock()
	if !ready {
		return fmt.Errorf("请先选择模型")
	}
	return c.inbox.deliver(c.book, TargetNextTurn, text, true)
}

// SelectModel 在会话空闲时切换下一轮使用的模型，并把选择同步记进账本。
func (c *Conversation) SelectModel(selection llm.Selection) error {
	err := c.llmSvc.Validate(selection)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.working {
		return fmt.Errorf("当前回答尚未结束，请稍后再切换模型")
	}
	_, err = c.book.RecordModelSelection(session.ModelSelectedData{Provider: selection.Provider, Model: selection.Model, Thinking: selection.Thinking})
	if err != nil {
		return err
	}
	c.config.Provider = selection.Provider
	c.config.Model = selection.Model
	c.config.Thinking = selection.Thinking
	return nil
}

// Steer 中途捎话：忙时进当前轮的下一步；闲时没人可打扰，就当新轮的开头。
func (c *Conversation) Steer(text string) error {
	return c.inbox.deliver(c.book, TargetNextStep, text, true)
}

// claimAsUserMessage 领出一份投递并落成用户的话——领出才进模型，两条账在这合上。
func (c *Conversation) claimAsUserMessage(item delivery) error {
	_, err := c.book.RecordClaim(item.ID)
	if err != nil {
		return err
	}
	_, err = c.book.RecordUserMessage(item.Text)
	return err
}
