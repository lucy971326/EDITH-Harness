package llm

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"harness/internal/llm/codex"

	"harness/internal/llm/oauth"

	"harness/internal/llm/xai"
	"harness/internal/persist"
)

// Models 返回当前已配置 Provider 可用的模型、窗口、是否看图和思考档位。
func (c *Client) Models() []ModelChoice {
	state := c.state()
	out := make([]ModelChoice, 0, len(state.models))
	for id, definition := range state.models {
		provider, ok := state.provider(definition.Provider)
		if !ok || !validProtocol(modelProtocol(definition, provider)) || !c.providerReady(definition.Provider, provider) {
			continue
		}
		efforts := make([]string, 0, len(definition.Reasoning))
		for _, level := range definition.Reasoning {
			efforts = append(efforts, level.Effort)
		}
		out = append(out, ModelChoice{
			ID:               id,
			Provider:         definition.Provider,
			ContextWindow:    definition.ContextWindow,
			Vision:           definition.Vision,
			ReasoningEfforts: efforts,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ValidateConfig 检查模型对应的本地 Provider 是否具备可用凭据。
func (c *Client) ValidateConfig(id string) error {
	state := c.state()
	definition, ok := state.models[id]
	if !ok {
		return fmt.Errorf("llm: model provider is not configured")
	}
	providerConfig, configured := state.provider(definition.Provider)
	if !configured || !c.providerReady(definition.Provider, providerConfig) {
		return fmt.Errorf("llm: model provider is not configured")
	}
	return nil
}

// ContextWindow 返回指定模型的窗口大小；未知模型返回 0。
func (c *Client) ContextWindow(id string) int {
	if c == nil {
		return 0
	}
	definition, ok := c.state().models[id]
	if !ok {
		return 0
	}
	return definition.ContextWindow
}

// Vision 返回指定模型是否能看图；未知模型返回 false。
func (c *Client) Vision(id string) bool {
	if c == nil {
		return false
	}
	definition, ok := c.state().models[id]
	return ok && definition.Vision
}

// 活对象。当前目录原子替换；已开始的模型循环通过 Pin 保留旧目录。
type Client struct {
	// current 为完整不可变目录；编辑串行化，已开始的 Run 持有旧目录。
	current atomic.Pointer[modelState]
	editMu  sync.Mutex
	files   *persist.Files
	auth    map[string]modelAuth
}

// 契约。已实现的内置账号通道，凭据与固定端点由各自实现持有。
type modelAuth interface {
	Start() (oauth.View, error)
	Status() oauth.View
	Cancel() oauth.View
	Logout() (oauth.View, error)
	Authenticated() bool
	RequestAuth(context.Context) (oauth.Authorization, error)
	Close()
}

var builtInProviders = []string{"openai-codex", "xai-oauth"}

func builtInProvider(id string) bool {
	for _, item := range builtInProviders {
		if item == id {
			return true
		}
	}
	return false
}

type modelState struct {
	config config
	models map[string]model
}

func (s *modelState) provider(id string) (providerConfig, bool) {
	if builtInProvider(id) {
		return providerConfig{Protocol: id}, true
	}
	entry, ok := s.config.Providers[id]
	return entry, ok
}

func (c *Client) state() *modelState {
	if c == nil {
		return &modelState{}
	}
	if current := c.current.Load(); current != nil {
		return current
	}
	return &modelState{}
}

// Pin 固定一轮运行使用的目录，避免多步工具调用期间切换供应商设置。
func (c *Client) Pin() *Client {
	state := c.state()
	pinned := &Client{auth: c.auth}
	pinned.current.Store(state)
	return pinned
}

// Stream 固定请求参数，再将供应商流翻译为 Harness 事件。
func (c *Client) Stream(ctx context.Context, config RunConfig, input Input) (<-chan StreamChunk, error) {
	state := c.state()
	definition, ok := state.models[config.Model]
	if !ok {
		return nil, fmt.Errorf("llm: unknown model %q", config.Model)
	}
	provider, ok := state.provider(definition.Provider)
	if !ok {
		return nil, fmt.Errorf("llm: provider %q is not configured", definition.Provider)
	}
	protocol := modelProtocol(definition, provider)
	if !validProtocol(protocol) {
		return nil, fmt.Errorf("llm: unsupported protocol %q", protocol)
	}
	messages, err := toProviderMessages(input.History, definition.Vision)
	if err != nil {
		return nil, err
	}
	options, err := reasoningOptions(definition, config.ReasoningEffort)
	if err != nil {
		return nil, err
	}
	request := streamRequest{definition: definition, protocol: protocol, baseURL: modelURL(definition, provider), token: provider.APIKey,
		system: input.System, messages: messages, tools: toProviderTools(input.Tools), toolChoice: input.ToolChoice, options: options}
	if strings.ContainsAny(request.baseURL, "{}") {
		return nil, fmt.Errorf("llm: 请先填写供应商 API 地址中的账号和网关 ID")
	}
	if definition.Provider == "github-copilot" {
		initiator := "user"
		if len(messages) > 0 && messages[len(messages)-1].Role != RoleUser {
			initiator = "agent"
		}
		request.headers = map[string]string{"User-Agent": "GitHubCopilotChat/0.35.0", "Editor-Version": "vscode/1.107.0", "Editor-Plugin-Version": "copilot-chat/0.35.0", "Copilot-Integration-Id": "vscode-chat", "X-Initiator": initiator, "Openai-Intent": "conversation-edits"}
		for _, message := range messages {
			for _, part := range message.Content {
				if part.Type == PartImage {
					request.headers["Copilot-Vision-Request"] = "true"
				}
			}
		}
	}
	if auth := c.auth[definition.Provider]; auth != nil {
		credentials, err := auth.RequestAuth(ctx)
		if err != nil {
			return nil, err
		}
		request.token = credentials.Token
		if credentials.AccountID != "" {
			request.headers = map[string]string{"chatgpt-account-id": credentials.AccountID, "originator": "edith", "OpenAI-Beta": "responses=experimental"}
		}
	}
	if request.token == "" {
		return nil, fmt.Errorf("llm: provider %q has no credentials", definition.Provider)
	}
	return startStream(ctx, request), nil
}

// New 从本机配置创建模型客户端。
func New(files *persist.Files) (*Client, error) {
	cfg, err := loadConfig(files)
	if err != nil {
		return nil, err
	}
	models, _, err := loadModelsFile(files)
	if err != nil {
		return nil, err
	}
	client := &Client{files: files, auth: map[string]modelAuth{}}
	constructors := []struct {
		id  string
		new func(*persist.Files) (modelAuth, error)
	}{
		{"openai-codex", func(f *persist.Files) (modelAuth, error) { return codex.New(f) }},
		{"xai-oauth", func(f *persist.Files) (modelAuth, error) { return xai.New(f) }},
	}
	for _, item := range constructors {
		auth, err := item.new(files)
		if err != nil {
			_ = client.Close()
			return nil, err
		}
		client.auth[item.id] = auth
	}
	client.current.Store(&modelState{config: cfg, models: models})
	return client, nil
}

func (c *Client) providerReady(id string, config providerConfig) bool {
	if auth := c.auth[id]; auth != nil {
		return auth.Authenticated()
	}
	return config.APIKey != ""
}

// Close 取消尚未完成的浏览器授权并释放回调监听器。
func (c *Client) Close() error {
	for _, auth := range c.auth {
		auth.Close()
	}
	return nil
}

func (c *Client) authFor(id string) (modelAuth, error) {
	auth := c.auth[id]
	if auth == nil {
		return nil, fmt.Errorf("%w: 未知账号供应商", ErrInvalidSettings)
	}
	return auth, nil
}

func (c *Client) StartAuth(id string) (oauth.View, error) {
	auth, err := c.authFor(id)
	if err != nil {
		return oauth.View{}, err
	}
	return auth.Start()
}
func (c *Client) AuthStatus(id string) (oauth.View, error) {
	auth, err := c.authFor(id)
	if err != nil {
		return oauth.View{}, err
	}
	return auth.Status(), nil
}
func (c *Client) CancelAuth(id string) (oauth.View, error) {
	auth, err := c.authFor(id)
	if err != nil {
		return oauth.View{}, err
	}
	return auth.Cancel(), nil
}
func (c *Client) LogoutAuth(id string) (oauth.View, error) {
	auth, err := c.authFor(id)
	if err != nil {
		return oauth.View{}, err
	}
	return auth.Logout()
}
