package runner

import (
	"fmt"

	"harness/kernel/session"
)

// Receive 接收带来源的协作消息；false 表示当前不接收，应由发送者保留待投递。
// true 以当前分叉账本实际存在消息 ID 为准，重试不会再次落账或进入检查点。
func (r *Runner) Receive(sessionID string, message session.Message) (bool, error) {
	if message.Role != session.RoleCollaboration || message.MessageID == "" || message.SourceSessionID == "" {
		return false, fmt.Errorf("runner: invalid collaboration identity")
	}
	sess, err := r.sessions.Get(sessionID)
	if err != nil {
		return false, err
	}
	if existing, found := collaborationEntry(sess.Entries(), message.MessageID); found {
		return collaborationMatches(existing.Message, message)
	}

	current, err := r.current(sessionID)
	if err != nil {
		return false, nil
	}
	current.handoff.Lock()
	// Checkpoint 可能在第一次读取后完成落账；在同一交接边界内重新确认。
	if existing, found := collaborationEntry(sess.Entries(), message.MessageID); found {
		current.handoff.Unlock()
		return collaborationMatches(existing.Message, message)
	}
	current.mu.Lock()
	for _, input := range current.pendingInputs {
		if input.message.MessageID != message.MessageID {
			continue
		}
		matches := input.message.Role == message.Role && input.message.SourceSessionID == message.SourceSessionID && input.message.SourceRunID == message.SourceRunID
		current.mu.Unlock()
		current.handoff.Unlock()
		if !matches {
			return false, fmt.Errorf("runner: collaboration ID conflicts with pending message")
		}
		return false, nil
	}
	if current.steeringState != steeringOpen {
		current.mu.Unlock()
		current.handoff.Unlock()
		return false, nil
	}
	runID := current.runID
	message.RunID = runID
	message.Blocks = cloneBlocks(message.Blocks)
	current.pendingInputs = append(current.pendingInputs, pendingInput{message: message})
	current.signalInputLocked()
	current.mu.Unlock()
	current.handoff.Unlock()
	return false, nil
}

func collaborationEntry(entries []session.Entry, messageID string) (session.Entry, bool) {
	for _, entry := range entries {
		if entry.Message.MessageID == messageID {
			return entry, true
		}
	}
	return session.Entry{}, false
}

func collaborationMatches(existing, incoming session.Message) (bool, error) {
	if existing.Role != incoming.Role || existing.SourceSessionID != incoming.SourceSessionID || existing.SourceRunID != incoming.SourceRunID {
		return false, fmt.Errorf("runner: collaboration ID conflicts with existing message")
	}
	return true, nil
}
