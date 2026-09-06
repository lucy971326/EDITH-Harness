package runner

import (
	"context"
	"errors"
	"fmt"

	"harness/kernel/session"
)

// Receive 接收带来源的协作消息；false 表示当前不接收，应由发送者保留待投递。
// true 以当前分叉账本实际存在消息 ID 为准，重试不会再次落账或进入检查点。
func (r *Runner) Receive(sessionID string, message session.Message) (bool, error) {
	if message.Role != session.RoleCollaboration || message.MessageID == "" || message.SourceSessionID == "" {
		return false, fmt.Errorf("runner: invalid collaboration identity")
	}
	current, err := r.current(sessionID)
	if err != nil {
		return false, nil
	}
	current.mu.Lock()
	sess, err := r.sessions.Get(sessionID)
	if err != nil {
		current.mu.Unlock()
		return false, err
	}
	for _, entry := range sess.Entries() {
		if entry.Message.MessageID == message.MessageID {
			current.mu.Unlock()
			if entry.Message.Role != message.Role || entry.Message.SourceSessionID != message.SourceSessionID || entry.Message.SourceRunID != message.SourceRunID {
				return false, fmt.Errorf("runner: collaboration ID conflicts with existing message")
			}
			return true, nil
		}
	}
	if current.steeringState != steeringOpen {
		current.mu.Unlock()
		return false, nil
	}
	message.RunID = current.runID
	message.Blocks = cloneBlocks(message.Blocks)
	entry, err := sess.Append(message)
	if err != nil {
		current.inputErr = errors.Join(current.inputErr, err)
		current.mu.Unlock()
		current.stop()
		return false, err
	}
	current.steers = append(current.steers, message)
	current.signalInputLocked()
	current.inputPublications.Add(1)
	defer current.inputPublications.Done()
	after := current.afterEntrySeq
	current.mu.Unlock()
	err = r.publish(context.Background(), RunEvent{SessionID: sessionID, RunID: message.RunID, Kind: Message, AfterEntrySeq: after, Entry: &entry})
	if err != nil {
		current.mu.Lock()
		current.inputErr = errors.Join(current.inputErr, err)
		current.mu.Unlock()
		current.stop()
	}
	return true, err
}
