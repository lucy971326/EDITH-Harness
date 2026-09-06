package subagents

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"harness/kernel/agents"
	"harness/kernel/events"
	"harness/kernel/llm"
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/session/settings"
)

// 活对象。任务级别协调器，保证单个任务的操作与状态变迁串行化，防止并发与竞态。
type taskCoord struct {
	mu           sync.Mutex
	task         Task
	activeHandle *runner.RunHandle
	persistErr   error
	deliveryErr  error
	finalizingCh chan struct{}
}

// 活对象。挂在 Host 的 subagents 键上的子会话委派服务。
type Subagents struct {
	sessions *session.Store
	settings settings.SessionSettingsStore
	agents   *agents.Service
	models   *llm.Client
	runner   *runner.Runner
	store    *taskStore

	ctx    context.Context
	cancel context.CancelFunc

	mu            sync.RWMutex
	closed        bool
	closeOnce     sync.Once
	closeDone     chan struct{}
	closeErr      error
	inFlight      sync.WaitGroup
	childSessions map[string]string   // childSessionID -> taskID
	parentTasks   map[string][]string // parentSessionID -> []taskID
	coords        map[string]*taskCoord

	wg          sync.WaitGroup
	deliveryMu  sync.Mutex
	changed     chan struct{}
	unsubscribe func()
}

// NewSubagents 组装子会话委派服务并恢复已有状态。subagentsDir 为 ~/.harness/subagents 目录。
func NewSubagents(
	sessions *session.Store,
	settingsStore settings.SessionSettingsStore,
	agentService *agents.Service,
	modelClient *llm.Client,
	runnerService *runner.Runner,
	eventRegistry *events.Registry,
	subagentsDir string,
) (*Subagents, error) {
	store, err := newTaskStore(subagentsDir)
	if err != nil {
		return nil, fmt.Errorf("subagents: init store: %w", err)
	}
	return newSubagentsWithStore(sessions, settingsStore, agentService, modelClient, runnerService, eventRegistry, store)
}

func newSubagentsWithStore(
	sessions *session.Store,
	settingsStore settings.SessionSettingsStore,
	agentService *agents.Service,
	modelClient *llm.Client,
	runnerService *runner.Runner,
	eventRegistry *events.Registry,
	store *taskStore,
) (*Subagents, error) {
	if sessions == nil {
		return nil, fmt.Errorf("subagents: nil sessions")
	}
	if settingsStore == nil {
		return nil, fmt.Errorf("subagents: nil session settings")
	}
	if agentService == nil {
		return nil, fmt.Errorf("subagents: nil agents")
	}
	if modelClient == nil {
		return nil, fmt.Errorf("subagents: nil llm")
	}
	if runnerService == nil {
		return nil, fmt.Errorf("subagents: nil runner")
	}
	if store == nil {
		return nil, fmt.Errorf("subagents: nil store")
	}
	if eventRegistry == nil {
		return nil, fmt.Errorf("subagents: nil events")
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &Subagents{
		sessions:      sessions,
		settings:      settingsStore,
		agents:        agentService,
		models:        modelClient,
		runner:        runnerService,
		store:         store,
		ctx:           ctx,
		cancel:        cancel,
		closeDone:     make(chan struct{}),
		childSessions: make(map[string]string),
		parentTasks:   make(map[string][]string),
		coords:        make(map[string]*taskCoord),
		changed:       make(chan struct{}),
	}

	// 启动恢复：加载所有持久化任务，未完成运行标记中断
	taskList, err := store.listTasks()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("subagents: recover tasks: %w", err)
	}

	for _, task := range taskList {
		// 校验重复 task ID
		if _, exists := s.coords[task.ID]; exists {
			cancel()
			return nil, fmt.Errorf("subagents: duplicate task ID %q during recovery", task.ID)
		}
		// 校验重复 childSessionID
		if prevID, exists := s.childSessions[task.ChildSessionID]; exists {
			cancel()
			return nil, fmt.Errorf("subagents: duplicate child session ID %q in tasks %q and %q during recovery", task.ChildSessionID, prevID, task.ID)
		}

		recovered := false
		if task.Status == StatusPending || task.Status == StatusRunning {
			recovered = true
			task.Status = StatusInterrupted
			task.Error = "interrupted by restart"
			now := time.Now().UTC()
			task.UpdatedAt = now
			if len(task.Turns) > 0 {
				task.Turns[len(task.Turns)-1].Status = StatusInterrupted
				task.Turns[len(task.Turns)-1].Error = task.Error
				task.Turns[len(task.Turns)-1].UpdatedAt = now
			}
		}
		// 兼容较早记录：中断等终态可能还没有生成通知，按同一轮稳定 ID 补齐。
		for _, rec := range task.Turns {
			found := false
			for _, n := range task.Notifications {
				if n.Turn == rec.Turn {
					found = true
					break
				}
			}
			if !found {
				task.Notifications = append(task.Notifications, Notification{NotificationID: notificationID(task.ID, rec.Turn), TaskID: task.ID, ChildSessionID: task.ChildSessionID, RunID: rec.RunID, Turn: rec.Turn, Status: rec.Status, ResultEntryID: rec.ResultEntryID, Error: rec.Error, CreatedAt: rec.UpdatedAt})
				recovered = true
			}
		}
		if recovered {
			err = store.saveTask(task)
			if err != nil {
				cancel()
				return nil, fmt.Errorf("subagents: mark interrupted task %q: %w", task.ID, err)
			}
		}
		s.childSessions[task.ChildSessionID] = task.ID
		s.parentTasks[task.ParentSessionID] = append(s.parentTasks[task.ParentSessionID], task.ID)

		closedCh := make(chan struct{})
		close(closedCh)
		s.coords[task.ID] = &taskCoord{
			task:         task,
			finalizingCh: closedCh,
		}
	}

	s.unsubscribe, err = events.Subscribe(eventRegistry, func(ctx context.Context, event runner.RunEvent) error {
		if event.Kind != runner.RunStarted {
			return nil
		}
		return s.deliver(event.SessionID)
	})
	if err != nil {
		cancel()
		return nil, err
	}
	s.wg.Add(1)
	go s.deliverLoop()
	return s, nil
}

// IsChildSession 判断指定会话是否为子会话（用于 Chat 列表过滤与空会话复用隔离）。
func (s *Subagents) IsChildSession(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.childSessions[sessionID]
	return ok
}

// Options 返回当前可供子任务使用的 Agent 清单与模型列表。
func (s *Subagents) Options(ctx context.Context) (OptionsResult, error) {
	err := ctx.Err()
	if err != nil {
		return OptionsResult{}, err
	}
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return OptionsResult{}, ErrClosed
	}
	s.inFlight.Add(1)
	s.mu.RUnlock()
	defer s.inFlight.Done()

	agentList, err := s.agents.List()
	if err != nil {
		return OptionsResult{}, fmt.Errorf("subagents options: list agents: %w", err)
	}
	modelsList := s.models.Models()
	return OptionsResult{
		Agents: agentList,
		Models: modelsList,
	}, nil
}

// Close 关闭服务：拒绝新请求，取消服务上下文，唤醒所有等待，停止所有孩子，等待 inFlight 与后台退出。
func (s *Subagents) Close() error {
	s.closeOnce.Do(func() {
		// 1. 取消服务生命周期 context，立即使所有待运行/进行中启动操作感知关闭
		s.cancel()
		s.unsubscribe()

		s.mu.Lock()
		s.closed = true
		var coordsToWake []*taskCoord
		for _, coord := range s.coords {
			coordsToWake = append(coordsToWake, coord)
		}
		s.mu.Unlock()

		// Wait / Send 通过服务 context 唤醒；子 Run 也继承同一 context。
		// 不提前关闭完成通道：完成信号只能由真正落盘的收尾发布。

		// 4. 等待所有正在执行中的公开入口调用完成
		s.inFlight.Wait()
		// 5. 等待所有运行追踪 goroutine 退出
		s.wg.Wait()

		// 6. 收集未落盘隐患并保存为全局 closeErr
		var persistErrs []error
		for _, coord := range coordsToWake {
			coord.mu.Lock()
			if coord.persistErr != nil {
				persistErrs = append(persistErrs, fmt.Errorf("task %s persist error: %w", coord.task.ID, coord.persistErr))
			}
			if coord.deliveryErr != nil {
				persistErrs = append(persistErrs, coord.deliveryErr)
			}
			coord.mu.Unlock()
		}

		if len(persistErrs) > 0 {
			s.closeErr = errors.Join(persistErrs...)
		}
		close(s.closeDone)
	})

	<-s.closeDone
	return s.closeErr
}

func (s *Subagents) trackRun(taskID string, turn int, handle *runner.RunHandle, coord *taskCoord) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		res := handle.Wait()
		s.onRunFinished(taskID, turn, res, coord)
	}()
}

func (s *Subagents) onRunFinished(taskID string, turn int, res runner.RunResult, coord *taskCoord) {
	coord.mu.Lock()
	if coord.task.Turn != turn || (coord.task.CurrentRunID != "" && coord.task.CurrentRunID != res.RunID) {
		coord.mu.Unlock()
		return
	}

	turnIdx := turn - 1
	var turnRec TurnRecord
	if turnIdx >= 0 && turnIdx < len(coord.task.Turns) {
		turnRec = coord.task.Turns[turnIdx]
	} else {
		turnRec = TurnRecord{Turn: turn, CreatedAt: time.Now().UTC()}
	}
	turnRec.RunID = res.RunID

	switch res.Status {
	case runner.RunSucceeded:
		coord.task.Status = StatusCompleted
		coord.task.Error = ""
		entryID, err := s.findFinalAssistantEntry(coord.task.ChildSessionID, res.RunID)
		if err != nil {
			// 真实账本读取错误：保留错误
			coord.task.Status = StatusFailed
			coord.task.ResultEntryID = ""
			coord.task.Error = err.Error()
			turnRec.Error = err.Error()
		} else {
			coord.task.ResultEntryID = entryID
			turnRec.ResultEntryID = entryID
		}
		turnRec.Status = coord.task.Status
	case runner.RunCancelled:
		coord.task.Status = StatusCancelled
		coord.task.Error = "cancelled"
		coord.task.ResultEntryID = ""
		turnRec.Status = StatusCancelled
		turnRec.Error = "cancelled"
	case runner.RunFailed:
		coord.task.Status = StatusFailed
		if res.Err != nil {
			coord.task.Error = res.Err.Error()
		} else {
			coord.task.Error = "run failed"
		}
		coord.task.ResultEntryID = ""
		turnRec.Status = StatusFailed
		turnRec.Error = coord.task.Error
	}

	now := time.Now().UTC()
	coord.task.UpdatedAt = now
	turnRec.UpdatedAt = now

	if turnIdx >= 0 && turnIdx < len(coord.task.Turns) {
		coord.task.Turns[turnIdx] = turnRec
	} else {
		coord.task.Turns = append(coord.task.Turns, turnRec)
	}

	notif := Notification{
		NotificationID: notificationID(coord.task.ID, turn),
		TaskID:         coord.task.ID,
		ChildSessionID: coord.task.ChildSessionID,
		RunID:          res.RunID,
		Turn:           turn,
		Status:         coord.task.Status,
		ResultEntryID:  coord.task.ResultEntryID,
		Error:          coord.task.Error,
		CreatedAt:      now,
	}
	coord.task.Notifications = append(coord.task.Notifications, notif)

	// 同一任务的状态更新与落盘串行，查询只能看到已完成的落盘尝试。
	saveErr := s.store.saveTask(coord.task)
	defer func() {
		coord.mu.Unlock()
		s.signalChange()
	}()
	if saveErr != nil {
		coord.persistErr = errors.Join(coord.persistErr, saveErr)
		coord.task.Error = fmt.Sprintf("persist completed task: %v", saveErr)
	}

	// 解除 finalizingCh 通道，通知 Send 等待方旧轮次已完全落盘收尾
	if coord.finalizingCh != nil {
		select {
		case <-coord.finalizingCh:
		default:
			close(coord.finalizingCh)
		}
	}
	coord.activeHandle = nil
}

func (s *Subagents) findFinalAssistantEntry(childSessionID, runID string) (string, error) {
	sess, err := s.sessions.Get(childSessionID)
	if err != nil {
		return "", fmt.Errorf("subagents: get child session %q: %w", childSessionID, err)
	}
	entries := sess.Entries()
	var lastAssistantEntry *session.Entry
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if entry.Message.RunID != runID {
			continue
		}
		if entry.Message.Role == session.RoleAssistant {
			lastAssistantEntry = &entry
			break
		}
	}
	if lastAssistantEntry == nil {
		return "", nil
	}

	hasToolCall := false
	hasNonEmptyText := false
	for _, b := range lastAssistantEntry.Message.Blocks {
		if b.Kind == "tool-call" || b.Tool != nil {
			hasToolCall = true
			break
		}
		if b.Kind == "text" && strings.TrimSpace(b.Text) != "" {
			hasNonEmptyText = true
		}
	}
	if hasToolCall || !hasNonEmptyText {
		return "", nil
	}
	return lastAssistantEntry.ID, nil
}

func deepCopyTask(t Task) Task {
	cp := t
	if len(t.Turns) > 0 {
		cp.Turns = make([]TurnRecord, len(t.Turns))
		copy(cp.Turns, t.Turns)
	}
	if len(t.Notifications) > 0 {
		cp.Notifications = make([]Notification, len(t.Notifications))
		copy(cp.Notifications, t.Notifications)
	}
	return cp
}

func newTaskID() (string, error) {
	var b [16]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return "", fmt.Errorf("subagents: new task id: %w", err)
	}
	return "task-" + hex.EncodeToString(b[:]), nil
}
