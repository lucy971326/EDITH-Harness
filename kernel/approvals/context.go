package approvals

import (
	"fmt"

	"harness/kernel/session"
)

// 从未压缩账本取真实输入；不把模型摘要、环境提示或委派文字当作用户授权。
func (s *Service) userRequests(sessionID, runID string, visiting map[string]bool) ([]string, error) {
	result, err := s.collectUserRequests(sessionID, runID, visiting, make(map[string]bool))
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("没有可核实的用户要求")
	}
	return result, nil
}

// 整次追溯共享消息身份集合，避免父会话的旧授权跨轮次重复出现在新限制之后。
func (s *Service) collectUserRequests(sessionID, runID string, visiting, seenMessages map[string]bool) ([]string, error) {
	if s.sessions == nil || sessionID == "" || runID == "" || visiting[sessionID] || len(visiting) >= 16 {
		return nil, fmt.Errorf("无法确认用户授权来源")
	}
	visiting[sessionID] = true
	defer delete(visiting, sessionID)
	sess, err := s.sessions.Get(sessionID)
	if err != nil {
		return nil, fmt.Errorf("授权来源会话不可用")
	}
	entries := sess.Entries()
	end := -1
	for i, entry := range entries {
		if entry.Message.RunID == runID {
			end = i
		}
	}
	if end < 0 {
		return nil, fmt.Errorf("授权来源不在当前分支")
	}
	var result []string
	seen := make(map[string]bool)
	for _, entry := range entries[:end+1] {
		message := entry.Message
		if message.Role != session.RoleUser {
			continue
		}
		if message.SourceSessionID != "" {
			key := message.SourceSessionID + ":" + message.SourceRunID
			if seen[key] {
				continue
			}
			seen[key] = true
			parent, err := s.collectUserRequests(message.SourceSessionID, message.SourceRunID, visiting, seenMessages)
			if err != nil {
				return nil, err
			}
			result = append(result, parent...)
			continue
		}
		if !message.UserAuthored {
			return nil, fmt.Errorf("旧消息缺少可信来源，请人工确认")
		}
		// Entry ID 在分叉中保持不变；相同文字的新消息仍应保留。
		if seenMessages[entry.ID] {
			continue
		}
		seenMessages[entry.ID] = true
		for _, block := range message.Blocks {
			if block.Kind != "text" {
				return nil, fmt.Errorf("授权包含非文本内容，请人工确认")
			}
			if block.Text != "" {
				result = append(result, block.Text)
			}
		}
	}
	return result, nil
}
