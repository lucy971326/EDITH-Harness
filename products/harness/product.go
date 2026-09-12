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

// Send 闲时启动、忙时插话；已接受的运行由 Runner 管理生命周期。
func (p *Product) Send(ctx context.Context, input RunInput) (string, error) {
	p.sendMu.Lock()
	defer p.sendMu.Unlock()
	err := ctx.Err()
	if err != nil {
		return "", err
	}
	_, err = p.Session(input.SessionID)
	if err != nil {
		return "", err
	}
	err = checkMessage(input.Message)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidMessage, err)
	}
	if _, running := p.runner.State(input.SessionID); running || input.ExpectedRunID != "" {
		err = p.steer(input.SessionID, input.ExpectedRunID, input.Message)
		return "steered", err
	}
	// 接受之后由 Runner 的 Stop / Close 管生命周期，不继承连接取消。
	err = p.start(context.Background(), input)
	if err != nil {
		return "", err
	}
	return "started", nil
}

// UpdateSettings 在会话空闲时保存下一轮使用的 Agent、模型和思考档位。
func (p *Product) UpdateSettings(ctx context.Context, sessionID string, next settings.SessionSettings) (SessionInfo, error) {
	p.sendMu.Lock()
	defer p.sendMu.Unlock()

	err := ctx.Err()
	if err != nil {
		return SessionInfo{}, err
	}
	info, err := p.Session(sessionID)
	if err != nil {
		return SessionInfo{}, err
	}
	if _, running := p.runner.State(sessionID); running {
		return SessionInfo{}, ErrRunActive
	}

	next.Workspace = info.Settings.Workspace
	err = p.validateRunSettings(next, false)
	if err != nil {
		return SessionInfo{}, fmt.Errorf("%w: %w", ErrInvalidRunSettings, err)
	}
	err = p.agents.SaveSessionSettings(sessionID, next)
	if err != nil {
		if errors.Is(err, agents.ErrInvalid) {
			return SessionInfo{}, fmt.Errorf("%w: %w", ErrInvalidRunSettings, err)
		}
		return SessionInfo{}, fmt.Errorf("%w: %w", ErrSessionSettings, err)
	}
	info.Settings = next
	return info, nil
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

// Snapshot 返回聊天投影所需的耐久账本、运行状态、草稿和更新边界。
func (s *Product) Snapshot(sessionID string) (Snapshot, error) {
	view, err := s.runner.SessionView(sessionID)
	if err != nil {
		return Snapshot{}, err
	}
	if view.Runs == nil {
		view.Runs = []runner.RunState{}
	}
	return Snapshot{Entries: view.Entries, Runs: view.Runs, UpdateSeq: view.UpdateSeq, SeqEpoch: view.SeqEpoch}, nil
}

// Start 保存下一轮设置并启动 Runner 自己管理的后台 Run。
func (s *Product) Start(ctx context.Context, input RunInput) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return s.start(ctx, input)
}

func (s *Product) start(ctx context.Context, input RunInput) error {
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
		return fmt.Errorf("%w: %w", ErrInvalidRunSettings, err)
	}
	err = s.agents.SaveSessionSettings(input.SessionID, setup)
	if err != nil {
		if errors.Is(err, agents.ErrInvalid) {
			return fmt.Errorf("%w: %w", ErrInvalidRunSettings, err)
		}
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
	return s.steer(sessionID, "", message)
}

func (s *Product) steer(sessionID, expectedRunID string, message session.UserMessage) error {
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
		return fmt.Errorf("%w: %w", ErrInvalidRunSettings, err)
	}
	if expectedRunID != "" {
		err = s.runner.SteerRun(sessionID, expectedRunID, message)
	} else {
		err = s.runner.Steer(sessionID, message)
	}
	if errors.Is(err, runner.ErrRunChanged) {
		return fmt.Errorf("%w: %w", ErrRunChanged, err)
	}
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
	sourceEntries := sess.Entries()
	_, err = s.sessions.Fork(input.SessionID, destinationID, through, info.Meta.Title+" · 分叉")
	if err != nil {
		return "", fmt.Errorf("harness product: copy session: %w", err)
	}
	dest, err := s.sessions.Get(destinationID)
	if err != nil {
		return "", fmt.Errorf("harness product: load forked session: %w", err)
	}
	seqMap := map[uint64]uint64{}
	for index, entry := range dest.Entries() {
		seqMap[sourceEntries[index].Seq] = entry.Seq
	}
	err = s.runner.CopyRecordsForFork(input.SessionID, destinationID, seqMap)
	if err != nil {
		return "", fmt.Errorf("harness product: copy run records: %w", err)
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
	if agentID := strings.TrimSpace(input.AgentID); agentID != "" {
		setup.AgentID = agentID
	}
	if model := strings.TrimSpace(input.Model); model != "" {
		setup.Model = model
	}
	if effort := strings.TrimSpace(input.ReasoningEffort); effort != "" {
		setup.ReasoningEffort = effort
	}
	return s.validateRunSettings(*setup, true)
}

func (s *Product) validateRunSettings(setup settings.SessionSettings, requireModel bool) error {
	if strings.TrimSpace(setup.AgentID) == "" {
		return fmt.Errorf("请先选择 Agent")
	}
	_, err := s.agents.Get(setup.AgentID)
	if err != nil {
		return fmt.Errorf("Agent 不可用：%w", err)
	}

	model := strings.TrimSpace(setup.Model)
	effort := strings.TrimSpace(setup.ReasoningEffort)
	if model == "" && effort == "" && !requireModel {
		return nil
	}
	if model == "" || effort == "" {
		return fmt.Errorf("请同时选择模型和思考档位")
	}
	for _, choice := range s.models.Models() {
		if choice.ID != model {
			continue
		}
		for _, available := range choice.ReasoningEfforts {
			if available == effort {
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
