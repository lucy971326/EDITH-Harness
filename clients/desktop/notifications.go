package desktop

import (
	"context"
	"fmt"
	"sync"
	"time"

	"harness/internal/approvals"
	"harness/internal/runner"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

type activitySubscriber func(func([]approvals.Pending, bool), func(runner.RunEvent)) (func(), error)

type notificationItem struct {
	id, sessionID, body string
	testResult          chan error
}

// 数据。系统授权与服务可用性，不保存用户开关。
type NotificationStatus struct {
	Permission string `json:"permission"`
	Message    string `json:"message,omitempty"`
}

// 活对象。Desktop 接入层持有原生通知、事件订阅和有界发送队列。
type Notifications struct {
	window    *application.WebviewWindow
	service   *notifications.NotificationService
	subscribe activitySubscriber

	mu          sync.Mutex
	queue       chan notificationItem
	done        chan struct{}
	unsubscribe func()
	known       map[string]struct{}
	pendingOpen string
	enabled     bool
	ready       bool
	closed      bool
	failed      bool
}

func NewNotifications(window *application.WebviewWindow, subscribe activitySubscriber) *Notifications {
	return &Notifications{window: window, service: notifications.New(), subscribe: subscribe}
}

// ServiceStartup 允许通知服务故障降级，不让 Wails 启动失败。
func (n *Notifications) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	err := n.service.ServiceStartup(ctx, options)
	if err != nil {
		_ = n.service.ServiceShutdown()
		n.mu.Lock()
		n.failed = true
		n.mu.Unlock()
		return nil
	}
	n.service.OnNotificationResponse(n.onResponse)
	n.mu.Lock()
	n.queue = make(chan notificationItem, 32)
	n.done = make(chan struct{})
	n.known = make(map[string]struct{})
	n.mu.Unlock()
	unsubscribe, err := n.subscribe(n.onApprovals, n.onRun)
	if err != nil {
		n.mu.Lock()
		n.failed = true
		n.queue = nil
		n.done = nil
		n.mu.Unlock()
		_ = n.service.ServiceShutdown()
		return nil
	}
	n.mu.Lock()
	n.unsubscribe = unsubscribe
	n.ready = true
	n.mu.Unlock()
	go n.sendLoop()
	return nil
}

func (n *Notifications) ServiceShutdown() error {
	n.mu.Lock()
	n.closed = true
	n.enabled = false
	unsubscribe, queue, done := n.unsubscribe, n.queue, n.done
	n.mu.Unlock()
	if unsubscribe != nil {
		unsubscribe()
	}
	if done == nil {
		return nil // 启动失败时已释放底层服务。
	}
	close(queue)
	select {
	case <-done:
		return n.service.ServiceShutdown()
	case <-time.After(5 * time.Second):
		return fmt.Errorf("desktop: notification sender did not stop")
	}
}

func (n *Notifications) Status() NotificationStatus {
	n.mu.Lock()
	ready := n.ready && !n.failed && !n.closed
	n.mu.Unlock()
	if !ready {
		return NotificationStatus{Permission: "unavailable", Message: "此环境暂不支持系统通知"}
	}
	if !notificationDaemonReady() {
		return NotificationStatus{Permission: "unavailable", Message: "当前桌面环境没有可用的通知服务"}
	}
	authorized, err := n.service.CheckNotificationAuthorization()
	if err != nil {
		return NotificationStatus{Permission: "unavailable", Message: "无法读取系统通知权限"}
	}
	if authorized {
		return NotificationStatus{Permission: "granted"}
	}
	return NotificationStatus{Permission: "not-granted"}
}

// RequestAuthorization 只由用户打开设置开关的动作调用。
func (n *Notifications) RequestAuthorization() (NotificationStatus, error) {
	if status := n.Status(); status.Permission != "not-granted" {
		return status, nil
	}
	authorized, err := n.service.RequestNotificationAuthorization()
	if err != nil {
		return NotificationStatus{Permission: "unavailable", Message: "系统通知授权失败"}, nil
	}
	if !authorized {
		return NotificationStatus{Permission: "not-granted", Message: "请在系统设置中允许 Harness 通知"}, nil
	}
	return NotificationStatus{Permission: "granted"}, nil
}

func (n *Notifications) SetEnabled(enabled bool) bool {
	if enabled && n.Status().Permission != "granted" {
		return false
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.enabled = enabled && n.ready && !n.failed && !n.closed
	return n.enabled
}

// SendTest 经现有发送队列提交测试通知；聚焦设置页时也会显示。
func (n *Notifications) SendTest() error {
	if n.Status().Permission != "granted" {
		return fmt.Errorf("desktop: notification permission is not granted")
	}
	result := make(chan error, 1)
	n.mu.Lock()
	if !n.enabled || n.closed || n.queue == nil {
		n.mu.Unlock()
		return fmt.Errorf("desktop: notifications are disabled")
	}
	select {
	case n.queue <- notificationItem{id: fmt.Sprintf("test-%d", time.Now().UnixNano()), body: "这是一条测试通知", testResult: result}:
	default:
		n.mu.Unlock()
		return fmt.Errorf("desktop: notification queue is full")
	}
	n.mu.Unlock()
	select {
	case err := <-result:
		return err
	case <-time.After(10 * time.Second):
		return fmt.Errorf("desktop: test notification timed out")
	}
}

// TakePendingOpen 补取前端完成挂载前点开的通知。
func (n *Notifications) TakePendingOpen() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	sessionID := n.pendingOpen
	n.pendingOpen = ""
	return sessionID
}

func (n *Notifications) onApprovals(pending []approvals.Pending, initial bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	next := make(map[string]struct{}, len(pending))
	for _, item := range pending {
		next[item.ID] = struct{}{}
		if !initial {
			if _, seen := n.known[item.ID]; !seen {
				n.enqueueLocked(notificationItem{id: "approval-" + item.ID, sessionID: item.SessionID, body: "有一项操作等待确认"})
			}
		}
	}
	n.known = next
}

func (n *Notifications) onRun(event runner.RunEvent) {
	if event.Kind != runner.RunEnded || (event.Status != runner.RunSucceeded && event.Status != runner.RunFailed) {
		return
	}
	body := "任务已完成"
	if event.Status == runner.RunFailed {
		body = "任务执行失败"
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.enqueueLocked(notificationItem{id: "run-" + event.RunID, sessionID: event.SessionID, body: body})
}

// 调用方持 mu；后台事件不能等待系统通知，也不能反压 Runner。
func (n *Notifications) enqueueLocked(item notificationItem) {
	if !n.enabled || n.closed || n.queue == nil {
		return
	}
	select {
	case n.queue <- item:
	default:
	}
}

func (n *Notifications) sendLoop() {
	defer close(n.done)
	for item := range n.queue {
		n.mu.Lock()
		enabled := n.enabled && !n.failed
		n.mu.Unlock()
		if !enabled || (item.testResult == nil && n.window.IsVisible() && !n.window.IsMinimised() && n.window.IsFocused()) {
			if item.testResult != nil {
				item.testResult <- fmt.Errorf("desktop: notifications are disabled")
			}
			continue
		}
		err := n.service.SendNotification(notifications.NotificationOptions{
			ID: item.id, Title: "Harness", Body: item.body,
			Data: map[string]interface{}{"sessionID": item.sessionID},
		})
		if item.testResult != nil {
			item.testResult <- err
		}
		// 系统服务可能临时离线；下一条可自然重试，Run 不受影响。
	}
}

func (n *Notifications) onResponse(result notifications.NotificationResult) {
	if result.Error != nil {
		return
	}
	sessionID, _ := result.Response.UserInfo["sessionID"].(string)
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return
	}
	if sessionID != "" {
		n.pendingOpen = sessionID
	}
	n.mu.Unlock()
	n.window.Show()
	n.window.UnMinimise()
	n.window.Focus()
	if sessionID != "" {
		n.window.EmitEvent("desktop-notification-open", sessionID)
	}
}

var _ application.ServiceStartup = (*Notifications)(nil)
var _ application.ServiceShutdown = (*Notifications)(nil)
