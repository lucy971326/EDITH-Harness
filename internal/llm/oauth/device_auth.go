package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"harness/internal/persist"
)

// 数据。设备码登录返回给用户的授权地址、短码及服务端轮询限制。
type Challenge struct {
	DeviceCode string
	URL        string
	UserCode   string
	Interval   time.Duration
	Expires    time.Duration
}

// 数据。可刷新的模型服务令牌；仅保存在本机私有文件。
type Credential struct {
	Access  string    `json:"access"`
	Refresh string    `json:"refresh"`
	Expires time.Time `json:"expires"`
}

// 契约。设备码服务自己负责端点、响应解析和刷新规则。
type DeviceDriver interface {
	Begin(context.Context) (Challenge, error)
	Poll(context.Context, string) (DeviceResult, error)
	Refresh(context.Context, Credential) (Credential, error)
}

type deviceTask struct {
	view   View
	cancel context.CancelFunc
	ctx    context.Context
}

// 活对象。设备码登录任务与该服务的私有令牌；刷新和退出串行化。
type DeviceAuth struct {
	mu         sync.Mutex
	workers    sync.WaitGroup
	store      *Store
	driver     DeviceDriver
	credential *Credential
	loadError  string
	task       *deviceTask
	closed     bool
}

// NewDeviceAuth 读取一家设备码服务的凭据；损坏凭据可在设置页重新登录。
func NewDeviceAuth(files *persist.Files, name string, driver DeviceDriver) (*DeviceAuth, error) {
	store, err := NewStore(files, name)
	if err != nil {
		return nil, err
	}
	auth := &DeviceAuth{store: store, driver: driver}
	body, err := store.Read()
	if errors.Is(err, os.ErrNotExist) {
		return auth, nil
	}
	if err != nil && !errors.Is(err, ErrUnreadable) {
		return nil, err
	}
	if err == nil {
		var credential Credential
		err = json.Unmarshal(body, &credential)
		clear(body)
		if err == nil && credential.Access != "" && credential.Refresh != "" && !credential.Expires.IsZero() {
			auth.credential = &credential
			return auth, nil
		}
	}
	auth.loadError = "已保存的登录信息无法读取，请重新登录"
	return auth, nil
}

func (a *DeviceAuth) Authenticated() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.credential != nil
}

func (a *DeviceAuth) viewLocked() View {
	if a.task != nil {
		view := a.task.view
		view.Authenticated = a.credential != nil
		return view
	}
	if a.credential != nil {
		return View{Authenticated: true, State: "connected"}
	}
	return View{State: "disconnected", Message: a.loadError}
}

func (a *DeviceAuth) Status() View {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.viewLocked()
}

func (a *DeviceAuth) Start() (View, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return View{}, fmt.Errorf("model OAuth service closed")
	}
	if a.task != nil && a.task.view.State == "waiting" {
		return a.viewLocked(), nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	challenge, err := a.driver.Begin(ctx)
	if err != nil {
		cancel()
		return View{}, err
	}
	if challenge.DeviceCode == "" || challenge.URL == "" || challenge.UserCode == "" || challenge.Expires <= 0 {
		cancel()
		return View{}, fmt.Errorf("设备授权响应不完整")
	}
	task := &deviceTask{view: View{State: "waiting", URL: challenge.URL, UserCode: challenge.UserCode},
		ctx: ctx, cancel: cancel}
	a.task = task
	a.workers.Add(1)
	go func() { defer a.workers.Done(); a.finish(task, challenge) }()
	return a.viewLocked(), nil
}

func (a *DeviceAuth) finish(task *deviceTask, challenge Challenge) {
	defer task.cancel()
	credential, err := PollDevice(task.ctx, challenge.Interval, challenge.Expires,
		func(ctx context.Context) (DeviceResult, error) { return a.driver.Poll(ctx, challenge.DeviceCode) })
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.task != task || task.ctx.Err() != nil {
		return
	}
	if err == nil {
		err = a.saveLocked(*credential)
	}
	if err != nil {
		task.view = View{State: "failed", Message: err.Error()}
		return
	}
	a.credential = credential
	a.loadError = ""
	task.view = View{Authenticated: true, State: "complete"}
}

func (a *DeviceAuth) saveLocked(credential Credential) error {
	body, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	err = a.store.Write(body)
	clear(body)
	return err
}

// Token 刷新临期令牌；新令牌保存成功后才供模型调用使用。
func (a *DeviceAuth) Token(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.credential == nil {
		return "", fmt.Errorf("请先登录账号")
	}
	if time.Until(a.credential.Expires) > 2*time.Minute {
		return a.credential.Access, nil
	}
	updated, err := a.driver.Refresh(ctx, *a.credential)
	if err != nil {
		if errors.Is(err, ErrLoginExpired) {
			if removeErr := a.store.Remove(); removeErr == nil || errors.Is(removeErr, os.ErrNotExist) {
				a.credential = nil
				a.loadError = "登录已失效，请重新登录"
			}
		}
		return "", err
	}
	if updated.Access == "" || updated.Refresh == "" || updated.Expires.IsZero() {
		return "", fmt.Errorf("刷新令牌响应不完整")
	}
	if err := a.saveLocked(updated); err != nil {
		return "", err
	}
	a.credential = &updated
	return updated.Access, nil
}

func (a *DeviceAuth) Cancel() View {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.task != nil && a.task.view.State == "waiting" {
		a.task.view = View{State: "cancelled"}
		a.task.cancel()
	}
	return a.viewLocked()
}

func (a *DeviceAuth) Logout() (View, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.task != nil {
		a.task.cancel()
		a.task = nil
	}
	err := a.store.Remove()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return a.viewLocked(), err
	}
	a.credential = nil
	a.loadError = ""
	return a.viewLocked(), nil
}

func (a *DeviceAuth) Close() {
	a.mu.Lock()
	a.closed = true
	if a.task != nil {
		a.task.cancel()
		a.task = nil
	}
	a.mu.Unlock()
	a.workers.Wait()
}
