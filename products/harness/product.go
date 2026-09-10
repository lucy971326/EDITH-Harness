package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"harness/kernel/agents"
	"harness/kernel/commands"
	"harness/kernel/llm"
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/session/settings"
	"harness/kernel/subagents"
)

// 活对象。聊天业务的统一入口；不拥有账本、Run 或事件登记处。
type Product struct {
	// 会话数据与运行配置。
	sessions *session.Store
	settings settings.SessionSettingsStore
	agents   *agents.Service
	models   *llm.Client

	// 执行与产品操作。
	runner    *runner.Runner
	commands  commands.Commands
	subagents *subagents.Subagents

	// 并发协调：分别保护空会话复用和发送时的忙闲判断。
	createMu sync.Mutex
	sendMu   sync.Mutex
}

// New 组装聊天业务服务。
func New(sessions *session.Store, settingsStore settings.SessionSettingsStore, agentService *agents.Service, modelClient *llm.Client, runService *runner.Runner, commandService commands.Commands, subagentService *subagents.Subagents) (*Product, error) {
	if sessions == nil {
		return nil, fmt.Errorf("harness product: nil sessions")
	}
	if settingsStore == nil {
		return nil, fmt.Errorf("harness product: nil session settings")
	}
	if agentService == nil {
		return nil, fmt.Errorf("harness product: nil agents")
	}
	if modelClient == nil {
		return nil, fmt.Errorf("harness product: nil llm")
	}
	if runService == nil {
		return nil, fmt.Errorf("harness product: nil runner")
	}
	if commandService == nil {
		return nil, fmt.Errorf("harness product: nil commands")
	}
	if subagentService == nil {
		return nil, fmt.Errorf("harness product: nil subagents")
	}
	return &Product{sessions: sessions, settings: settingsStore, agents: agentService, models: modelClient, runner: runService, commands: commandService, subagents: subagentService}, nil
}

// Create 创建或复用指定工作区中的空会话。
func (s *Product) Create(workspace string) (SessionInfo, error) {
	err := checkWorkspace(workspace)
	if err != nil {
		return SessionInfo{}, fmt.Errorf("%w: %w", ErrWorkspace, err)
	}
	s.createMu.Lock()
	defer s.createMu.Unlock()

	infos, err := s.List()
	if err != nil {
		return SessionInfo{}, err
	}
	for _, info := range infos {
		if info.Settings.Workspace != workspace {
			continue
		}
		sess, err := s.sessions.Get(info.Meta.ID)
		if err != nil {
			return SessionInfo{}, fmt.Errorf("harness product: get session: %w", err)
		}
		if len(sess.Entries()) == 0 {
			return info, nil
		}
	}

	id, err := session.NewID()
	if err != nil {
		return SessionInfo{}, err
	}
	_, err = s.sessions.Create(id)
	if err != nil {
		return SessionInfo{}, fmt.Errorf("harness product: create session: %w", err)
	}
	setup := settings.SessionSettings{AgentID: agents.DefaultID, Workspace: workspace}
	err = s.settings.Put(id, setup)
	if err != nil {
		discardErr := s.sessions.DiscardEmpty(id)
		return SessionInfo{}, fmt.Errorf("harness product: save session settings: %w", errors.Join(err, discardErr))
	}
	return s.Session(id)
}

// List 返回全部会话及其运行设置；页面分组和排序由调用方决定。
func (s *Product) List() ([]SessionInfo, error) {
	metas, err := s.sessions.List()
	if err != nil {
		return nil, fmt.Errorf("harness product: list sessions: %w", err)
	}
	out := make([]SessionInfo, 0, len(metas))
	for _, meta := range metas {
		if s.subagents.IsChildSession(meta.ID) {
			continue
		}
		setup, err := s.settings.For(meta.ID)
		if err != nil {
			return nil, fmt.Errorf("%w: settings for %q: %w", ErrSessionSettings, meta.ID, err)
		}
		out = append(out, SessionInfo{Meta: meta, Settings: setup})
	}
	return out, nil
}

// Session 返回一场已存在会话的元数据与运行设置。
func (s *Product) Session(id string) (SessionInfo, error) {
	if s.subagents.IsChildSession(id) {
		return SessionInfo{}, fmt.Errorf("%w: %w: session %q", ErrSessionNotFound, os.ErrNotExist, id)
	}
	metas, err := s.sessions.List()
	if err != nil {
		return SessionInfo{}, err
	}
	for _, meta := range metas {
		if meta.ID == id {
			setup, err := s.settings.For(id)
			if err != nil {
				return SessionInfo{}, fmt.Errorf("%w: settings for %q: %w", ErrSessionSettings, id, err)
			}
			return SessionInfo{Meta: meta, Settings: setup}, nil
		}
	}
	return SessionInfo{}, fmt.Errorf("%w: %w: session %q", ErrSessionNotFound, os.ErrNotExist, id)
}

// Snapshot 返回聊天投影所需的耐久账本与活跃 Run。
func (s *Product) Snapshot(sessionID string) (Snapshot, error) {
	sess, err := s.sessions.Get(sessionID)
	if err != nil {
		return Snapshot{}, err
	}
	out := Snapshot{Runs: []runner.RunState{}}
	// 先读运行身份，再读账本；不把尚未落下首条输入的准备期当成可恢复运行。
	if state, ok := s.runner.State(sessionID); ok && state.AfterEntrySeq != 0 {
		out.Runs = []runner.RunState{state}
	}
	out.Entries = sess.Entries()
	return out, nil
}

// Start 保存下一轮设置并启动 Runner 自己管理的后台 Run。
func (s *Product) Start(ctx context.Context, input RunInput) error {
	if s.subagents.IsChildSession(input.SessionID) {
		return fmt.Errorf("%w: session %q", os.ErrNotExist, input.SessionID)
	}
	_, err := s.sessions.Get(input.SessionID)
	if err != nil {
		return err
	}
	err = checkMessage(input.Message)
	if err != nil {
		return err
	}
	setup, err := s.settings.For(input.SessionID)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSessionSettings, err)
	}
	err = s.selectRunSettings(&setup, input)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidRunSettings, err)
	}
	err = s.ensureVision(setup.Model, input.Message)
	if err != nil {
		return err
	}
	err = s.settings.Put(input.SessionID, setup)
	if err != nil {
		return err
	}
	_, err = s.runner.Start(ctx, input.SessionID, input.Message)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRunStart, err)
	}
	return nil
}

// Steer 将一条输入交给当前 Run；不修改下一轮设置。
func (s *Product) Steer(sessionID string, message session.UserMessage) error {
	if s.subagents.IsChildSession(sessionID) {
		return fmt.Errorf("%w: session %q", os.ErrNotExist, sessionID)
	}
	_, err := s.sessions.Get(sessionID)
	if err != nil {
		return err
	}
	err = checkMessage(message)
	if err != nil {
		return err
	}
	setup, err := s.settings.For(sessionID)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSessionSettings, err)
	}
	err = s.ensureVision(setup.Model, message)
	if err != nil {
		return err
	}
	err = s.runner.Steer(sessionID, message)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRunSteer, err)
	}
	return nil
}

// Stop 取消父会话与它的孩子；父已闲置时也不能漏掉仍在运行的孩子。
func (s *Product) Stop(sessionID string) error {
	if s.subagents.IsChildSession(sessionID) {
		return fmt.Errorf("%w: session %q", os.ErrNotExist, sessionID)
	}
	_, err := s.sessions.Get(sessionID)
	if err != nil {
		return err
	}
	return s.subagents.StopFamily(context.Background(), sessionID)
}

// Fork 复制账本中一段已结束助手回答之前的历史与会话设置。
func (s *Product) Fork(input ForkInput) (string, error) {
	if input.SessionID == "" || input.RunID == "" || input.BoundaryEntryID == "" {
		return "", fmt.Errorf("harness product: fork has empty required field")
	}
	if state, running := s.runner.State(input.SessionID); running && state.RunID == input.RunID {
		return "", fmt.Errorf("harness product: assistant response is still running")
	}
	sess, err := s.sessions.Get(input.SessionID)
	if err != nil {
		return "", err
	}
	through, err := assistantSegmentEnd(sess.Entries(), input.RunID, input.BoundaryEntryID)
	if err != nil {
		return "", err
	}
	setup, err := s.settings.For(input.SessionID)
	if err != nil {
		return "", fmt.Errorf("harness product: load session settings: %w", err)
	}
	info, err := s.Session(input.SessionID)
	if err != nil {
		return "", err
	}
	destinationID, err := session.NewID()
	if err != nil {
		return "", err
	}
	err = s.settings.Put(destinationID, setup)
	if err != nil {
		return "", fmt.Errorf("harness product: copy session settings: %w", err)
	}
	_, err = s.sessions.Fork(input.SessionID, destinationID, through, info.Meta.Title+" · 分叉")
	if err != nil {
		return "", fmt.Errorf("harness product: copy session: %w", err)
	}
	return destinationID, nil
}

// CallCommand 执行一条平台命令。
func (s *Product) CallCommand(ctx context.Context, name, sessionID string) error {
	_, err := s.sessions.Get(sessionID)
	if err != nil {
		return err
	}
	return s.commands.Call(ctx, name, sessionID)
}

func (s *Product) selectRunSettings(setup *settings.SessionSettings, input RunInput) error {
	if strings.TrimSpace(input.Model) == "" || strings.TrimSpace(input.ReasoningEffort) == "" {
		return fmt.Errorf("请先选择模型和思考档位")
	}
	agentID := strings.TrimSpace(input.AgentID)
	if agentID == "" {
		agentID = setup.AgentID
	}
	_, err := s.agents.Get(agentID)
	if err != nil {
		return fmt.Errorf("Agent 不可用：%w", err)
	}
	for _, choice := range s.models.Models() {
		if choice.ID != input.Model {
			continue
		}
		for _, effort := range choice.ReasoningEfforts {
			if effort == input.ReasoningEffort {
				setup.AgentID = agentID
				setup.Model = input.Model
				setup.ReasoningEffort = input.ReasoningEffort
				return nil
			}
		}
	}
	return fmt.Errorf("模型或思考档位不可用")
}

func (s *Product) ensureVision(model string, message session.UserMessage) error {
	for _, block := range message.Blocks {
		if block.Kind == "image" && !s.models.Vision(model) {
			return fmt.Errorf("当前模型无法识别图片")
		}
	}
	return nil
}

func checkWorkspace(workspace string) error {
	workspace = strings.TrimSpace(workspace)
	if !filepath.IsAbs(workspace) {
		return fmt.Errorf("工作区目录必须是绝对路径")
	}
	info, err := os.Stat(workspace)
	if err != nil {
		return fmt.Errorf("工作区目录不可用：%w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("工作区路径不是目录")
	}
	return nil
}

func checkMessage(message session.UserMessage) error {
	if len(message.Blocks) == 0 {
		return fmt.Errorf("请输入文字或图片")
	}
	for _, block := range message.Blocks {
		switch block.Kind {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				return nil
			}
		case "image":
			if block.Media != nil && block.Media.MIME != "" && block.Media.Data != "" {
				return nil
			}
		}
	}
	return fmt.Errorf("请输入文字或图片")
}

func assistantSegmentEnd(entries []session.Entry, runID, boundaryEntryID string) (string, error) {
	start := -1
	for index, entry := range entries {
		if entry.ID == boundaryEntryID && entry.Message.RunID == runID && entry.Message.Role == session.RoleUser {
			start = index
			break
		}
	}
	if start < 0 {
		return "", fmt.Errorf("harness product: assistant segment not found")
	}
	end := ""
	for _, entry := range entries[start+1:] {
		if entry.Message.RunID != runID {
			continue
		}
		if entry.Message.Role == session.RoleUser {
			break
		}
		end = entry.ID
	}
	if end == "" {
		return "", fmt.Errorf("harness product: assistant segment has no durable result")
	}
	return end, nil
}
