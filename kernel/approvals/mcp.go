package approvals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"harness/kernel/permissions"
)

const mcpTrustFile = "mcp-trust.json"

// ErrMCPConfigDenied 表示用户本轮不启用项目 MCP；全局 MCP 仍可使用。
var ErrMCPConfigDenied = errors.New("approvals: project MCP configuration denied")

// ConfirmMCPConfig 在连接项目 Server 前要求用户确认当前配置版本。
func (s *Service) ConfirmMCPConfig(ctx context.Context, identity Identity, digest string, request MCPRequest) error {
	if digest == "" || request.Kind != "config" || identity.SessionID == "" || identity.RunID == "" {
		return fmt.Errorf("approvals: invalid MCP configuration approval")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return context.Canceled
	}
	trusted, err := s.readMCPTrust()
	if err != nil {
		s.mu.Unlock()
		return err
	}
	if trusted[request.Workspace] == digest {
		s.mu.Unlock()
		return nil
	}
	s.work.Add(1)
	s.mu.Unlock()
	defer s.work.Done()

	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	defer cancel()
	identity.ToolCallID = "mcp-config"
	request.Digest = digest
	decision, err := s.waitForHuman(ctx, Pending{Identity: identity, MCP: &request})
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.ctx.Err() != nil {
		return context.Canceled
	}
	if !decision.Approved {
		return ErrMCPConfigDenied
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return context.Canceled
	}
	trusted, err = s.readMCPTrust()
	if err != nil {
		return err
	}
	trusted[request.Workspace] = digest
	body, err := json.Marshal(trusted)
	if err != nil {
		return err
	}
	return s.files.Write(mcpTrustFile, body)
}

// AuthorizeMCPCall 按本轮模式逐次审核 MCP 操作；批准不改变沙箱或会话设置。
func (s *Service) AuthorizeMCPCall(ctx context.Context, identity Identity, kind permissions.ReviewerKind, request MCPRequest) error {
	if request.Kind != "call" || identity.SessionID == "" || identity.RunID == "" || identity.ToolCallID == "" {
		return fmt.Errorf("approvals: invalid MCP tool approval")
	}
	if kind != permissions.HumanReviewer && kind != permissions.ModelReviewer {
		return fmt.Errorf("approvals: MCP reviewer %q is unavailable", kind)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return context.Canceled
	}
	settings := s.settings
	s.work.Add(1)
	s.mu.Unlock()
	defer s.work.Done()
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	defer cancel()

	var decision permissions.Decision
	var err error
	if kind == permissions.ModelReviewer {
		var result reviewResult
		result, err = s.reviewOperation(ctx, identity, settings, request)
		if err == nil && result.Decision != "ask" {
			decision = permissions.Decision{Approved: result.Decision == "allow", Reason: result.Reason}
		} else {
			reason := result.Reason
			if err != nil {
				reason = "智能审核未完成，转人工审批：" + err.Error()
			}
			decision, err = s.waitForHuman(ctx, Pending{Identity: identity, MCP: &request, ReviewReason: reason})
		}
	} else {
		decision, err = s.waitForHuman(ctx, Pending{Identity: identity, MCP: &request})
	}
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.ctx.Err() != nil {
		return context.Canceled
	}
	if !decision.Approved {
		return fmt.Errorf("approvals: MCP tool call denied: %s", decision.Reason)
	}
	return nil
}

func (s *Service) readMCPTrust() (map[string]string, error) {
	if s.files == nil {
		return nil, fmt.Errorf("approvals: MCP trust storage unavailable")
	}
	body, err := s.files.Read(mcpTrustFile)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, err
	}
	var trusted map[string]string
	if err := json.Unmarshal(body, &trusted); err != nil {
		return nil, fmt.Errorf("approvals: decode MCP trust: %w", err)
	}
	if trusted == nil {
		trusted = make(map[string]string)
	}
	return trusted, nil
}
