package terminal

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"

	"harness/agents"
	"harness/core"
	"harness/llm"
	"harness/session"
	"harness/tools"
)

var errChangeAgent = errors.New("切换 agent")
var errChangeSession = errors.New("切换会话")
var errNewSession = errors.New("新建会话")

// service 组合终端的输入输出、可用能力和当前正在看的会话。
type service struct {
	app      *core.App
	roster   agents.Service
	models   *llm.Service
	tools    *tools.Registry
	rawInput io.Reader
	input    *bufio.Reader
	renderer *renderer
	queue    *eventQueue

	mu      sync.Mutex
	view    activeView
	stopped chan struct{}

	renderMu   sync.Mutex
	renderCond *sync.Cond
	rendered   map[string]int
}

type activeView struct {
	sessionID  string
	generation int
	ready      bool
	watermark  int
	pending    []queuedEvent
}

type queuedEvent struct {
	generation int
	appended   *session.Appended
	problem    string
}

func newService(app *core.App, roster agents.Service, models *llm.Service, toolsReg *tools.Registry, input io.Reader, output io.Writer) *service {
	s := &service{
		app:      app,
		roster:   roster,
		models:   models,
		tools:    toolsReg,
		rawInput: input,
		input:    bufio.NewReader(input),
		renderer: newRenderer(output),
		queue:    newEventQueue(),
		stopped:  make(chan struct{}),
		rendered: make(map[string]int),
	}
	s.renderCond = sync.NewCond(&s.renderMu)
	app.Subscribe(session.EventAppended, s.noticeAppend)
	app.Subscribe(agents.EventConversationError, s.noticeProblem)
	return s
}

// Run 依次选择 Agent、会话和聊天；返回时不再读取终端。
func (s *service) Run(ctx context.Context) error {
	go s.renderEvents()
	defer func() {
		s.queue.close()
		<-s.stopped
	}()

	interrupts, stopSignals := interruptChannel(s.rawInput)
	defer stopSignals()

	for {
		profile, err := s.chooseAgent(ctx, interrupts)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		for {
			conversation, err := s.chooseSession(ctx, interrupts, profile)
			if errors.Is(err, errChangeAgent) {
				break
			}
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
			s.openView(conversation)
			err = s.chat(ctx, interrupts, conversation)
			closeErr := conversation.Close()
			if closeErr != nil {
				return closeErr
			}
			if errors.Is(err, errChangeAgent) {
				break
			}
			if errors.Is(err, errChangeSession) || errors.Is(err, errNewSession) {
				continue
			}
			if errors.Is(err, io.EOF) || err == nil {
				return nil
			}
			return err
		}
	}
}

func (s *service) chooseAgent(ctx context.Context, interrupts <-chan os.Signal) (agents.AgentProfile, error) {
	for {
		profiles, err := s.roster.ListAgents()
		if err != nil {
			return agents.AgentProfile{}, err
		}
		s.renderer.line("\n小红：")
		for i, profile := range profiles {
			state := ""
			if profile.Archived {
				state = "（已归档，只能打开旧会话）"
			}
			s.renderer.line("%d. %s %s", i+1, profile.ID, state)
		}
		s.renderer.line("n. 新建小红")
		line, err := s.readLine(ctx, interrupts, "请选择：")
		if err != nil {
			return agents.AgentProfile{}, err
		}
		if line == "n" {
			profile, err := s.createAgent(ctx, interrupts)
			if err != nil {
				return agents.AgentProfile{}, err
			}
			return profile, nil
		}
		index, ok := parseChoice(line, len(profiles))
		if !ok {
			s.renderer.line("请输入编号或 n。")
			continue
		}
		return profiles[index], nil
	}
}

func (s *service) createAgent(ctx context.Context, interrupts <-chan os.Signal) (agents.AgentProfile, error) {
	id, err := s.readRequired(ctx, interrupts, "小红名字：")
	if err != nil {
		return agents.AgentProfile{}, err
	}
	providers := s.models.Providers()
	if len(providers) == 0 {
		return agents.AgentProfile{}, errors.New("没有已安装的模型服务商")
	}
	s.renderer.line("模型服务商：")
	for i, provider := range providers {
		s.renderer.line("%d. %s", i+1, provider.Name)
	}
	providerIndex, err := s.readChoice(ctx, interrupts, len(providers), "请选择服务商：")
	if err != nil {
		return agents.AgentProfile{}, err
	}
	provider := providers[providerIndex]
	model, err := s.readRequired(ctx, interrupts, "模型名：")
	if err != nil {
		return agents.AgentProfile{}, err
	}
	s.renderer.line("思考档位：")
	for i, level := range provider.ThinkingLevels {
		s.renderer.line("%d. %s", i+1, level)
	}
	thinkingIndex, err := s.readChoice(ctx, interrupts, len(provider.ThinkingLevels), "请选择思考档位：")
	if err != nil {
		return agents.AgentProfile{}, err
	}
	prompt, err := s.readLine(ctx, interrupts, "人设（可空）：")
	if err != nil {
		return agents.AgentProfile{}, err
	}
	tools := s.tools.Names()
	s.renderer.line("可用工具：")
	for i, name := range tools {
		s.renderer.line("%d. %s", i+1, name)
	}
	toolLine, err := s.readLine(ctx, interrupts, "工具编号（可空，多个用逗号分隔）：")
	if err != nil {
		return agents.AgentProfile{}, err
	}
	selected, err := chooseTools(toolLine, tools)
	if err != nil {
		return agents.AgentProfile{}, err
	}
	profile := agents.AgentProfile{
		ID:           id,
		Provider:     provider.Name,
		Model:        model,
		Thinking:     provider.ThinkingLevels[thinkingIndex],
		SystemPrompt: prompt,
		Tools:        selected,
	}
	err = s.roster.CreateAgent(profile)
	if err != nil {
		return agents.AgentProfile{}, err
	}
	s.renderer.line("已创建小红：%s", id)
	return s.roster.GetAgent(id)
}

func (s *service) chooseSession(ctx context.Context, interrupts <-chan os.Signal, profile agents.AgentProfile) (agents.Conversation, error) {
	for {
		sessions, err := s.roster.ListSessions(profile.ID)
		if err != nil {
			return nil, err
		}
		s.renderer.line("\n%s 的会话：", profile.ID)
		for i, item := range sessions {
			state := ""
			if item.Open {
				state = "（已打开）"
			}
			s.renderer.line("%d. %s %s", i+1, item.ID, state)
		}
		if !profile.Archived {
			s.renderer.line("n. 新建会话")
		}
		s.renderer.line("a. 换小红")
		line, err := s.readLine(ctx, interrupts, "请选择：")
		if err != nil {
			return nil, err
		}
		if line == "a" {
			return nil, errChangeAgent
		}
		if line == "n" && !profile.Archived {
			return s.startSession(ctx, interrupts, profile.ID)
		}
		index, ok := parseChoice(line, len(sessions))
		if !ok {
			s.renderer.line("请输入编号、n 或 a。")
			continue
		}
		item := sessions[index]
		if item.Open {
			return s.roster.GetSession(item.ID)
		}
		return s.roster.ResumeSession(item.ID)
	}
}

func (s *service) startSession(ctx context.Context, interrupts <-chan os.Signal, agentID string) (agents.Conversation, error) {
	for {
		id, err := s.readRequired(ctx, interrupts, "新会话名字（全局唯一）：")
		if err != nil {
			return nil, err
		}
		conversation, err := s.roster.StartSession(agentID, id)
		if err == nil {
			return conversation, nil
		}
		s.renderer.line("新建失败：%v", err)
	}
}

func (s *service) chat(ctx context.Context, interrupts <-chan os.Signal, conversation agents.Conversation) error {
	s.renderer.line("\n进入会话 %s。:history 重看历史，:sessions 换会话，:agents 换小红，:cancel 取消，:exit 退出。", conversation.SessionID())
	for {
		line, err := s.readLine(ctx, interrupts, "你：")
		if err != nil {
			return err
		}
		switch line {
		case "":
			continue
		case ":history":
			s.openView(conversation)
			continue
		case ":sessions":
			return errChangeSession
		case ":agents":
			return errChangeAgent
		case ":new":
			return errNewSession
		case ":cancel":
			conversation.Cancel()
			s.renderer.line("已请求取消当前步骤。")
			continue
		case ":exit":
			return nil
		}
		err = conversation.SubmitFollowup(line)
		if err != nil {
			s.renderer.line("发送失败：%v", err)
			continue
		}
		err = waitIdle(ctx, interrupts, conversation)
		if errors.Is(err, context.Canceled) {
			return err
		}
		if err != nil {
			return err
		}
		s.waitRendered(conversation.SessionID(), len(conversation.Book().Events()))
	}
}

func (s *service) openView(conversation agents.Conversation) {
	s.mu.Lock()
	s.view.sessionID = conversation.SessionID()
	s.view.generation++
	s.view.ready = false
	s.view.watermark = 0
	s.view.pending = nil
	generation := s.view.generation
	s.mu.Unlock()

	events := conversation.Book().Events()
	s.renderer.history(events)

	s.mu.Lock()
	if s.view.generation != generation {
		s.mu.Unlock()
		return
	}
	s.view.watermark = len(events)
	s.view.ready = true
	pending := s.view.pending
	s.view.pending = nil
	s.mu.Unlock()
	s.markRendered(conversation.SessionID(), len(events))
	for _, item := range pending {
		s.queue.push(item)
	}
}

func (s *service) noticeAppend(payload any) {
	appended, ok := payload.(session.Appended)
	if !ok {
		return
	}
	s.mu.Lock()
	if s.view.sessionID != appended.SessionID {
		s.mu.Unlock()
		return
	}
	generation := s.view.generation
	s.mu.Unlock()
	s.queue.push(queuedEvent{generation: generation, appended: &appended})
}

func (s *service) noticeProblem(payload any) {
	problem, ok := payload.(agents.ConversationError)
	if !ok {
		return
	}
	s.mu.Lock()
	if s.view.sessionID != problem.SessionID {
		s.mu.Unlock()
		return
	}
	generation := s.view.generation
	s.mu.Unlock()
	s.queue.push(queuedEvent{generation: generation, problem: problem.Message})
}

func (s *service) renderEvents() {
	defer close(s.stopped)
	for {
		item, ok := s.queue.pop()
		if !ok {
			return
		}
		s.mu.Lock()
		if item.generation != s.view.generation {
			s.mu.Unlock()
			continue
		}
		if !s.view.ready {
			s.view.pending = append(s.view.pending, item)
			s.mu.Unlock()
			continue
		}
		watermark := s.view.watermark
		s.mu.Unlock()
		if item.appended != nil {
			if item.appended.Event.Seq <= watermark {
				continue
			}
			s.renderer.live(item.appended.Event)
			s.markRendered(item.appended.SessionID, item.appended.Event.Seq)
			continue
		}
		if item.problem != "" {
			s.renderer.line("运行失败：%s", safeText(item.problem))
		}
	}
}

func (s *service) waitRendered(sessionID string, sequence int) {
	s.renderMu.Lock()
	defer s.renderMu.Unlock()
	for s.rendered[sessionID] < sequence {
		s.renderCond.Wait()
	}
}

func (s *service) markRendered(sessionID string, sequence int) {
	s.renderMu.Lock()
	if sequence > s.rendered[sessionID] {
		s.rendered[sessionID] = sequence
		s.renderCond.Broadcast()
	}
	s.renderMu.Unlock()
}

func (s *service) readRequired(ctx context.Context, interrupts <-chan os.Signal, prompt string) (string, error) {
	for {
		line, err := s.readLine(ctx, interrupts, prompt)
		if err != nil {
			return "", err
		}
		if line != "" {
			return line, nil
		}
		s.renderer.line("这一项不能为空。")
	}
}

func (s *service) readChoice(ctx context.Context, interrupts <-chan os.Signal, count int, prompt string) (int, error) {
	for {
		line, err := s.readLine(ctx, interrupts, prompt)
		if err != nil {
			return 0, err
		}
		index, ok := parseChoice(line, count)
		if ok {
			return index, nil
		}
		s.renderer.line("请输入有效编号。")
	}
}

func (s *service) readLine(ctx context.Context, interrupts <-chan os.Signal, prompt string) (string, error) {
	s.renderer.prompt(prompt)
	type result struct {
		line string
		err  error
	}
	read := make(chan result, 1)
	go func() {
		line, err := s.input.ReadString('\n')
		read <- result{line: line, err: err}
	}()
	select {
	case result := <-read:
		line := strings.TrimSpace(result.line)
		if result.err != nil && !errors.Is(result.err, io.EOF) {
			return "", result.err
		}
		if errors.Is(result.err, io.EOF) && line == "" {
			return "", io.EOF
		}
		return line, nil
	case <-ctx.Done():
		return "", ctx.Err()
	case <-interrupts:
		return "", context.Canceled
	}
}

func waitIdle(ctx context.Context, interrupts <-chan os.Signal, conversation agents.Conversation) error {
	done := make(chan struct{})
	go func() {
		conversation.WaitIdle()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		conversation.Cancel()
		<-done
		return ctx.Err()
	case <-interrupts:
		conversation.Cancel()
		<-done
		return nil
	}
}

func parseChoice(text string, count int) (int, bool) {
	number, err := strconv.Atoi(text)
	if err != nil || number < 1 || number > count {
		return 0, false
	}
	return number - 1, true
}

func chooseTools(text string, names []string) ([]string, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	seen := make(map[int]bool)
	selected := make([]string, 0)
	for _, part := range strings.Split(text, ",") {
		index, ok := parseChoice(strings.TrimSpace(part), len(names))
		if !ok {
			return nil, fmt.Errorf("工具编号 %q 不存在", strings.TrimSpace(part))
		}
		if seen[index] {
			continue
		}
		seen[index] = true
		selected = append(selected, names[index])
	}
	return selected, nil
}

func interruptChannel(input io.Reader) (<-chan os.Signal, func()) {
	file, ok := input.(*os.File)
	if !ok || file != os.Stdin {
		return nil, func() {}
	}
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	return interrupts, func() {
		signal.Stop(interrupts)
	}
}

type eventQueue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	items  []queuedEvent
	closed bool
}

func newEventQueue() *eventQueue {
	queue := &eventQueue{}
	queue.cond = sync.NewCond(&queue.mu)
	return queue
}

func (q *eventQueue) push(item queuedEvent) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.items = append(q.items, item)
	q.cond.Signal()
}

func (q *eventQueue) pop() (queuedEvent, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.items) == 0 && !q.closed {
		q.cond.Wait()
	}
	if len(q.items) == 0 {
		return queuedEvent{}, false
	}
	item := q.items[0]
	q.items = q.items[1:]
	return item, true
}

func (q *eventQueue) close() {
	q.mu.Lock()
	q.closed = true
	q.cond.Broadcast()
	q.mu.Unlock()
}

// renderer 将账本投影成终端上的聊天和工具过程；它从不显示隐藏思考。
type renderer struct {
	mu        sync.Mutex
	output    io.Writer
	calls     map[string]string
	streaming bool
}

func newRenderer(output io.Writer) *renderer {
	return &renderer{output: output, calls: make(map[string]string)}
}

func (r *renderer) history(events []session.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = make(map[string]string)
	r.streaming = false
	fmt.Fprintln(r.output, "\n--- 历史 ---")
	for _, event := range visibleEvents(events) {
		r.render(event, false)
	}
	fmt.Fprintln(r.output, "--- 现在 ---")
}

func (r *renderer) live(event session.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.render(event, true)
}

func (r *renderer) line(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.streaming {
		fmt.Fprintln(r.output)
		r.streaming = false
	}
	fmt.Fprintf(r.output, format+"\n", args...)
}

func (r *renderer) prompt(prompt string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.streaming {
		fmt.Fprintln(r.output)
		r.streaming = false
	}
	fmt.Fprint(r.output, prompt)
}

func (r *renderer) render(event session.Event, live bool) {
	switch event.Kind {
	case session.KindUserMessage:
		// 本地终端已经回显用户刚输入的这一行；实时再画会重复。
		if live {
			return
		}
		var data session.UserMessageData
		if json.Unmarshal(event.Data, &data) == nil {
			r.breakStream()
			fmt.Fprintf(r.output, "你：%s\n", safeText(data.Text))
		}
	case session.KindChunk:
		if !live {
			return
		}
		var data session.ChunkData
		if json.Unmarshal(event.Data, &data) == nil {
			if !r.streaming {
				fmt.Fprint(r.output, "小红：")
				r.streaming = true
			}
			fmt.Fprint(r.output, safeText(data.Delta))
		}
	case session.KindAssistantFinal:
		var data session.AssistantFinalData
		if json.Unmarshal(event.Data, &data) != nil {
			return
		}
		if live && len(event.Replaces) > 0 && r.streaming {
			fmt.Fprintln(r.output)
			r.streaming = false
			return
		}
		r.breakStream()
		if data.Text != "" {
			fmt.Fprintf(r.output, "小红：%s\n", safeText(data.Text))
		}
	case session.KindToolCall:
		var data session.ToolCallData
		if json.Unmarshal(event.Data, &data) == nil {
			r.calls[data.ID] = data.Name
			r.breakStream()
			fmt.Fprintf(r.output, "工具：%s\n", safeText(data.Name))
		}
	case session.KindToolStart:
		var data session.ToolStartData
		if json.Unmarshal(event.Data, &data) == nil {
			r.breakStream()
			fmt.Fprintf(r.output, "工具执行中：%s\n", safeText(r.toolName(data.CallID)))
		}
	case session.KindToolResult:
		var data session.ToolResultData
		if json.Unmarshal(event.Data, &data) == nil {
			r.breakStream()
			fmt.Fprintf(r.output, "工具完成：%s（%s）%s\n", safeText(r.toolName(data.CallID)), data.Status, preview(data.Output))
		}
	}
}

func (r *renderer) breakStream() {
	if !r.streaming {
		return
	}
	fmt.Fprintln(r.output)
	r.streaming = false
}

func (r *renderer) toolName(callID string) string {
	name := r.calls[callID]
	if name == "" {
		return callID
	}
	return name
}

func visibleEvents(events []session.Event) []session.Event {
	replaced := make(map[int]bool)
	for _, event := range events {
		for _, seq := range event.Replaces {
			replaced[seq] = true
		}
	}
	visible := make([]session.Event, 0, len(events))
	for _, event := range events {
		if !replaced[event.Seq] {
			visible = append(visible, event)
		}
	}
	return visible
}

func preview(text string) string {
	clean := safeText(text)
	if clean == "" {
		return ""
	}
	const limit = 240
	runes := []rune(clean)
	if len(runes) > limit {
		clean = string(runes[:limit]) + "…"
	}
	return "：" + clean
}

func safeText(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r >= 0x20 {
			return r
		}
		return '�'
	}, text)
}
