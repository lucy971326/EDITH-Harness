package approvals

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"sync"
	"time"

	"harness/internal/llm"
	"harness/internal/permissions"
	"harness/internal/persist"
	"harness/internal/session"
)

// ErrExpired 表示请求已回答、已取消或不存在。
var ErrExpired = errors.New("approvals: request is no longer pending")

type pendingRequest struct {
	view   Pending
	ctx    context.Context
	answer chan permissions.Decision
}

// 活对象。拥有待审批请求与订阅；等待沿用工具的 Context。
type Service struct {
	// 审核依赖；密钥只在启动读取。
	models   *llm.Client
	sessions *session.Store
	files    *persist.Files
	jevKey   string
	http     *http.Client

	// 服务关闭时取消审核与人工等待，并等待在途申请退出。
	ctx    context.Context
	cancel context.CancelFunc
	work   sync.WaitGroup

	// 全局设置、待审批与订阅状态。
	mu        sync.Mutex
	settings  Settings
	pending   map[string]*pendingRequest
	listeners map[chan []Pending]struct{}
	closed    bool
}

// New 创建审批服务，不启动后台任务。
func New() *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		pending:   make(map[string]*pendingRequest),
		listeners: make(map[chan []Pending]struct{}),
		settings:  Settings{Engine: "llm"},
		http:      &http.Client{Timeout: 30 * time.Second},
		ctx:       ctx, cancel: cancel,
	}
}

// Authorize 检查申请，必要时等待审核，返回本次独立权限。
func (s *Service) Authorize(ctx context.Context, identity Identity, kind permissions.ReviewerKind, request permissions.ApprovalRequest) (permissions.Policy, error) {
	err := ctx.Err()
	if err != nil {
		return permissions.Policy{}, err
	}
	request = cloneRequest(request)
	requirement, err := permissions.Evaluate(request.Current, request.Requested)
	if err != nil {
		return permissions.Policy{}, err
	}
	if requirement == permissions.Allow {
		return request.Current, nil
	}
	if kind != permissions.HumanReviewer && kind != permissions.ModelReviewer {
		return permissions.Policy{}, fmt.Errorf("approvals: reviewer %q is unavailable; additional permissions were not granted", kind)
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return permissions.Policy{}, context.Canceled
	}
	settings := s.settings
	s.work.Add(1)
	s.mu.Unlock()
	defer s.work.Done()
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	defer cancel()
	var reviewer permissions.Reviewer = humanReviewer{service: s, identity: identity}
	if kind == permissions.ModelReviewer {
		reviewer = modelReviewer{service: s, identity: identity, settings: settings}
	}
	decision, err := reviewer.Review(ctx, request)
	if err != nil {
		return permissions.Policy{}, err
	}
	err = ctx.Err()
	if err != nil {
		return permissions.Policy{}, err
	}
	if s.ctx.Err() != nil {
		return permissions.Policy{}, context.Canceled
	}
	return permissions.ApplyDecision(request, decision)
}

type humanReviewer struct {
	service  *Service
	identity Identity
	reason   string
}

var _ permissions.Reviewer = humanReviewer{}

func (r humanReviewer) Review(ctx context.Context, request permissions.ApprovalRequest) (permissions.Decision, error) {
	return r.service.waitForHuman(ctx, Pending{Identity: r.identity, Request: cloneRequest(request), ReviewReason: r.reason})
}

func (s *Service) waitForHuman(ctx context.Context, view Pending) (permissions.Decision, error) {
	if view.SessionID == "" || view.RunID == "" || view.ToolCallID == "" {
		return permissions.Decision{}, fmt.Errorf("approvals: missing trusted call identity")
	}
	view.ID = rand.Text()
	item := &pendingRequest{view: view, ctx: ctx, answer: make(chan permissions.Decision, 1)}
	s.mu.Lock()
	if s.closed || ctx.Err() != nil {
		s.mu.Unlock()
		return permissions.Decision{}, context.Canceled
	}
	s.pending[item.view.ID] = item
	s.publishLocked()
	s.mu.Unlock()

	select {
	case decision := <-item.answer:
		return decision, ctx.Err()
	case <-ctx.Done():
		s.mu.Lock()
		delete(s.pending, item.view.ID)
		s.publishLocked()
		s.mu.Unlock()
		return permissions.Decision{}, ctx.Err()
	}
}

// Respond 原子取走请求；重复或取消后的回答不会再次交付权限。
func (s *Service) Respond(id string, decision permissions.Decision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := s.pending[id]
	if item == nil {
		return ErrExpired
	}
	delete(s.pending, id)
	s.publishLocked()
	if item.ctx.Err() != nil {
		return ErrExpired
	}
	item.answer <- decision
	return nil
}

// Subscribe 原子返回快照和后续快照通道；慢订阅只保留最新状态。
func (s *Service) Subscribe() ([]Pending, <-chan []Pending, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	updates := make(chan []Pending, 1)
	if s.closed {
		close(updates)
		return []Pending{}, updates, func() {}
	}
	s.listeners[updates] = struct{}{}
	var once sync.Once
	return s.snapshotLocked(), updates, func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if _, ok := s.listeners[updates]; ok {
				delete(s.listeners, updates)
				close(updates)
			}
		})
	}
}

// Close 拒绝待审批申请并关闭订阅，不保留跨重启授权。
func (s *Service) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		s.work.Wait()
		return nil
	}
	s.closed = true
	s.cancel()
	for id, item := range s.pending {
		delete(s.pending, id)
		item.answer <- permissions.Decision{Reason: "approval service closed"}
	}
	for listener := range s.listeners {
		close(listener)
		delete(s.listeners, listener)
	}
	s.mu.Unlock()
	s.work.Wait()
	s.http.CloseIdleConnections()
	return nil
}

// 调用方持锁；全量快照可以合并，不能阻塞工具或积累事件队列。
func (s *Service) publishLocked() {
	for listener := range s.listeners {
		select {
		case <-listener:
		default:
		}
		listener <- s.snapshotLocked()
	}
}

func (s *Service) snapshotLocked() []Pending {
	result := make([]Pending, 0, len(s.pending))
	for _, item := range s.pending {
		view := item.view
		view.Request = cloneRequest(view.Request)
		if view.MCP != nil {
			copied := *view.MCP
			copied.Servers = slices.Clone(copied.Servers)
			copied.Arguments = slices.Clone(copied.Arguments)
			view.MCP = &copied
		}
		result = append(result, view)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func cloneRequest(request permissions.ApprovalRequest) permissions.ApprovalRequest {
	request.Arguments = slices.Clone(request.Arguments)
	request.Current.WriteRoots = append([]string{}, request.Current.WriteRoots...)
	request.Requested.WriteRoots = slices.Clone(request.Requested.WriteRoots)
	return request
}

// Open 创建审批服务并读取持久化设置；失败时释放已创建的取消资源。
func Open(files *persist.Files, models *llm.Client, sessions *session.Store) (*Service, error) {
	service := New()
	service.models = models
	service.sessions = sessions
	err := service.loadSettings(files)
	if err != nil {
		service.Close()
		return nil, err
	}
	return service, nil
}
