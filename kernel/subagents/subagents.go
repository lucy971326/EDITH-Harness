package subagents

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"harness/kernel/agents"
	"harness/kernel/events"
	"harness/kernel/llm"
	"harness/kernel/persist"
	"harness/kernel/runner"
	"harness/kernel/session"
	"harness/kernel/session/settings"
)

// 活对象。任务级别协调器，串行同一孩子的操作并保留进程内停止边界。
type taskCoord struct {
	// 启动许可与停止代次由 Subagents.mu 保护。
	admission      admission
	stopRequested  bool
	stopGeneration uint64

	mu          sync.Mutex
	record      TaskRecord
	deliveryErr error
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
	families      map[string]familyState

	wg          sync.WaitGroup
	deliveryMu  sync.Mutex
	confirmed   map[string]struct{} // 父账本已确认存在的通知；仅为可丢弃的进程内缓存。
	pending     map[string]struct{} // 尚需定时重试投递的 taskID。
	changed     chan struct{}
	unsubscribe func()
}

// NewSubagents 组装子会话委派服务并从统一持久化服务恢复关系。
func NewSubagents(
	sessions *session.Store,
	settingsStore settings.SessionSettingsStore,
	agentService *agents.Service,
	modelClient *llm.Client,
	runnerService *runner.Runner,
	eventRegistry *events.Registry,
	files *persist.Files,
) (*Subagents, error) {
	taskFiles, err := files.Scope("tasks")
	if err != nil {
		return nil, fmt.Errorf("subagents: scope store: %w", err)
	}
	store, err := newTaskStoreFiles(taskFiles)
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
		families:      make(map[string]familyState),
		confirmed:     make(map[string]struct{}),
		pending:       make(map[string]struct{}),
		changed:       make(chan struct{}),
	}

	// 启动只恢复父子关系。运行中断与结果继续由 Runner 和子账本恢复。
	records, err := store.listTasks()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("subagents: recover tasks: %w", err)
	}
	for _, record := range records {
		if _, exists := s.coords[record.ID]; exists {
			cancel()
			return nil, fmt.Errorf("subagents: duplicate task ID %q during recovery", record.ID)
		}
		if previousID, exists := s.childSessions[record.ChildSessionID]; exists {
			cancel()
			return nil, fmt.Errorf("subagents: duplicate child session ID %q in tasks %q and %q during recovery", record.ChildSessionID, previousID, record.ID)
		}
		s.childSessions[record.ChildSessionID] = record.ID
		s.parentTasks[record.ParentSessionID] = append(s.parentTasks[record.ParentSessionID], record.ID)
		s.coords[record.ID] = &taskCoord{
			admission: admission{parentSessionID: record.ParentSessionID},
			record:    record,
		}
	}

	s.unsubscribe, err = events.Subscribe(eventRegistry, func(ctx context.Context, event runner.RunEvent) error {
		if event.Kind != runner.RunStarted {
			return nil
		}
		return s.onRunStarted(event)
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
	return OptionsResult{Agents: agentList, Models: s.models.Models()}, nil
}

// Close 拒绝新请求、取消孩子，并等待公开调用和后台工作退出。
func (s *Subagents) Close() error {
	s.closeOnce.Do(func() {
		s.cancel()
		s.unsubscribe()

		s.mu.Lock()
		s.closed = true
		coords := make([]*taskCoord, 0, len(s.coords))
		for _, coord := range s.coords {
			coords = append(coords, coord)
		}
		s.mu.Unlock()

		s.inFlight.Wait()
		s.wg.Wait()

		var deliveryErrs []error
		for _, coord := range coords {
			coord.mu.Lock()
			if coord.deliveryErr != nil {
				deliveryErrs = append(deliveryErrs, coord.deliveryErr)
			}
			coord.mu.Unlock()
		}
		if len(deliveryErrs) > 0 {
			s.closeErr = errors.Join(deliveryErrs...)
		}
		close(s.closeDone)
	})

	<-s.closeDone
	return s.closeErr
}

func (s *Subagents) trackRun(handle *runner.RunHandle) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		handle.Wait()
		s.signalChange()
	}()
}

func newTaskID() (string, error) {
	var b [16]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return "", fmt.Errorf("subagents: new task id: %w", err)
	}
	return "task-" + hex.EncodeToString(b[:]), nil
}
