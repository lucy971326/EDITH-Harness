package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"sync"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"harness/agents"
	"harness/core"
	"harness/llm"
	"harness/session"
	"harness/tools"
)

type tuiService struct {
	app    *core.App
	roster agents.Service
	models *llm.Service
	tools  *tools.Registry
	input  io.Reader
	output io.Writer
	events *eventBuffer
}

func newTUIService(app *core.App, roster agents.Service, models *llm.Service, toolsReg *tools.Registry, input io.Reader, output io.Writer) *tuiService {
	events := newEventBuffer()
	app.Subscribe(session.EventAppended, func(payload any) {
		appended, ok := payload.(session.Appended)
		if !ok {
			return
		}
		events.push(appendedMessage{value: appended})
	})
	app.Subscribe(agents.EventConversationError, func(payload any) {
		problem, ok := payload.(agents.ConversationError)
		if !ok {
			return
		}
		events.push(conversationErrorMessage{value: problem})
	})
	return &tuiService{
		app:    app,
		roster: roster,
		models: models,
		tools:  toolsReg,
		input:  input,
		output: output,
		events: events,
	}
}

// Run 启动全屏终端界面；界面本身只驱动 agents 和 session 事件。
func (s *tuiService) Run(ctx context.Context) error {
	model := newTUIModel(s.roster, s.models, s.tools, s.events)
	program := tea.NewProgram(
		model,
		tea.WithContext(ctx),
		tea.WithInput(s.input),
		tea.WithOutput(s.output),
	)
	_, err := program.Run()
	s.events.close()
	closeErr := model.closeConversation()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	return closeErr
}

type eventBuffer struct {
	mu     sync.Mutex
	items  []tea.Msg
	wake   chan struct{}
	closed bool
}

func newEventBuffer() *eventBuffer {
	return &eventBuffer{wake: make(chan struct{}, 1)}
}

func (b *eventBuffer) push(msg tea.Msg) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.items = append(b.items, msg)
	b.mu.Unlock()

	select {
	case b.wake <- struct{}{}:
	default:
	}
}

func (b *eventBuffer) pop() []tea.Msg {
	b.mu.Lock()
	defer b.mu.Unlock()
	items := b.items
	b.items = nil
	return items
}

func (b *eventBuffer) close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	b.mu.Unlock()
	select {
	case b.wake <- struct{}{}:
	default:
	}
}

func waitEventBatch(buffer *eventBuffer) tea.Cmd {
	return func() tea.Msg {
		<-buffer.wake
		items := buffer.pop()
		if len(items) == 0 {
			return nil
		}
		return eventBatchMessage{items: items}
	}
}

type appendedMessage struct {
	value session.Appended
}

type conversationErrorMessage struct {
	value agents.ConversationError
}

type eventBatchMessage struct {
	items []tea.Msg
}

type profilesLoadedMessage struct {
	profiles []agents.AgentProfile
	err      error
}

type sessionsLoadedMessage struct {
	agentID  string
	sessions []agents.SessionSummary
	err      error
}

type conversationOpenedMessage struct {
	agentID      string
	sessionID    string
	conversation agents.Conversation
	err          error
}

type agentCreatedMessage struct {
	profile agents.AgentProfile
	err     error
}

type followupSubmittedMessage struct {
	err error
}

type focusArea int

const (
	focusAgents focusArea = iota
	focusSessions
	focusChat
)

type tuiModel struct {
	roster agents.Service
	models *llm.Service
	tools  *tools.Registry
	events *eventBuffer

	profiles     []agents.AgentProfile
	sessions     []agents.SessionSummary
	agentIndex   int
	sessionIndex int
	focus        focusArea
	loading      bool
	status       string
	problem      string

	conversation agents.Conversation
	transcript   []session.Event
	lastSeq      int
	viewport     viewport.Model
	textarea     textarea.Model
	help         help.Model
	keys         tuiKeyMap
	width        int
	height       int
	form         *agentForm
	sessionInput textinput.Model
	sessionForm  bool
}

func newTUIModel(roster agents.Service, models *llm.Service, toolsReg *tools.Registry, events *eventBuffer) *tuiModel {
	area := textarea.New()
	area.Placeholder = "输入消息……"
	area.Prompt = "┃ "
	area.SetVirtualCursor(false)
	area.ShowLineNumbers = false
	area.SetHeight(3)
	area.SetStyles(chatTextareaStyles())
	area.Blur()

	viewportModel := viewport.New()
	viewportModel.SoftWrap = true
	viewportModel.MouseWheelEnabled = true

	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = "输入全局唯一的会话名"
	input.SetStyles(formInputStyles())
	input.SetWidth(42)

	return &tuiModel{
		roster:       roster,
		models:       models,
		tools:        toolsReg,
		events:       events,
		focus:        focusAgents,
		viewport:     viewportModel,
		textarea:     area,
		help:         help.New(),
		keys:         newTUIKeyMap(),
		sessionInput: input,
		width:        80,
		height:       24,
	}
}

func (m *tuiModel) Init() tea.Cmd {
	return tea.Batch(waitEventBatch(m.events), loadProfiles(m.roster))
}

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch value := msg.(type) {
	case eventBatchMessage:
		for _, item := range value.items {
			m.applyEvent(item)
		}
		return m, waitEventBatch(m.events)
	case profilesLoadedMessage:
		m.loading = false
		if value.err != nil {
			m.problem = value.err.Error()
			return m, nil
		}
		m.profiles = value.profiles
		m.agentIndex = clampIndex(m.agentIndex, m.agentCount())
		return m, nil
	case sessionsLoadedMessage:
		m.loading = false
		if value.err != nil {
			m.problem = value.err.Error()
			return m, nil
		}
		if m.selectedAgentID() != value.agentID {
			return m, nil
		}
		m.sessions = value.sessions
		m.sessionIndex = clampIndex(m.sessionIndex, m.sessionCount())
		return m, nil
	case conversationOpenedMessage:
		m.loading = false
		if value.err != nil {
			m.conversation = nil
			m.problem = value.err.Error()
			return m, nil
		}
		m.conversation = value.conversation
		m.transcript = m.conversation.Book().Events()
		m.lastSeq = lastEventSequence(m.transcript)
		m.refreshTranscript(true)
		m.focus = focusChat
		m.problem = ""
		m.status = "已打开会话 " + value.sessionID
		return m, m.textarea.Focus()
	case agentCreatedMessage:
		m.loading = false
		if value.err != nil {
			m.problem = value.err.Error()
			return m, nil
		}
		m.form = nil
		m.profiles = append(m.profiles, value.profile)
		sort.Slice(m.profiles, func(i int, j int) bool {
			return m.profiles[i].ID < m.profiles[j].ID
		})
		m.agentIndex = indexOfAgent(m.profiles, value.profile.ID)
		m.focus = focusSessions
		m.problem = ""
		m.status = "已创建 Agent：" + value.profile.ID
		m.sessions = nil
		m.sessionIndex = 0
		return m, loadSessions(m.roster, value.profile.ID)
	case followupSubmittedMessage:
		m.loading = false
		if value.err != nil {
			m.problem = value.err.Error()
			return m, nil
		}
		m.status = "消息已送入会话"
		return m, nil
	case tea.WindowSizeMsg:
		m.resize(value.Width, value.Height)
		return m, nil
	case tea.KeyPressMsg:
		return m.updateKey(value)
	}

	if m.form != nil {
		return m, m.form.update(msg)
	}
	if m.sessionForm {
		var cmd tea.Cmd
		m.sessionInput, cmd = m.sessionInput.Update(msg)
		return m, cmd
	}
	if m.focus == focusChat {
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *tuiModel) updateKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.form != nil {
		return m.updateAgentFormKey(msg)
	}
	if m.sessionForm {
		return m.updateSessionForm(msg)
	}

	keystroke := msg.Keystroke()
	if keystroke == "ctrl+c" {
		return m, tea.Quit
	}
	if keystroke == "?" && m.focus != focusChat {
		m.help.ShowAll = true
		m.focus = focusChat
		return m, nil
	}
	if keystroke == "esc" && m.help.ShowAll {
		m.help.ShowAll = false
		return m, nil
	}
	if keystroke == "tab" {
		m.nextFocus()
		return m, m.focusInput()
	}

	if m.focus == focusChat {
		return m.updateChatKey(msg)
	}
	if m.focus == focusAgents {
		return m.updateAgentKey(msg)
	}
	return m.updateSessionKey(msg)
}

func (m *tuiModel) updateAgentFormKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	form := m.form
	switch msg.String() {
	case "esc":
		m.form = nil
		m.problem = ""
		return m, nil
	case "tab":
		form.field = (form.field + 1) % agentFormFieldCount
		return m, form.focus()
	case "enter":
		if form.field == formTools {
			profile, err := form.profile()
			if err != nil {
				form.problem = err.Error()
				return m, nil
			}
			if profile.ID == "" || profile.Model == "" {
				form.problem = "Agent 名字和模型名不能为空"
				return m, nil
			}
			m.form = nil
			m.loading = true
			m.status = "正在保存 Agent……"
			return m, createAgent(m.roster, profile)
		}
		form.field++
		return m, form.focus()
	case "left":
		if form.field == formProvider || form.field == formThinking {
			form.moveChoice(-1)
			return m, nil
		}
	case "right":
		if form.field == formProvider || form.field == formThinking {
			form.moveChoice(1)
			return m, nil
		}
	}
	index := form.inputIndex()
	if index < 0 {
		return m, nil
	}
	var cmd tea.Cmd
	form.inputs[index], cmd = form.inputs[index].Update(msg)
	return m, cmd
}

func (m *tuiModel) updateAgentKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		m.agentIndex = clampIndex(m.agentIndex-1, m.agentCount())
	case "down":
		m.agentIndex = clampIndex(m.agentIndex+1, m.agentCount())
	case "enter":
		if m.agentIndex == len(m.profiles) {
			form, err := newAgentForm(m.models, m.tools)
			if err != nil {
				m.problem = err.Error()
				return m, nil
			}
			m.form = form
			return m, form.focus()
		}
		m.sessions = nil
		m.sessionIndex = 0
		m.focus = focusSessions
		return m, loadSessions(m.roster, m.selectedAgentID())
	case "esc":
		return m, tea.Quit
	}
	return m, nil
}

func (m *tuiModel) updateSessionKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		m.sessionIndex = clampIndex(m.sessionIndex-1, m.sessionCount())
	case "down":
		m.sessionIndex = clampIndex(m.sessionIndex+1, m.sessionCount())
	case "enter":
		if m.sessionIndex == len(m.sessions) && m.canCreateSession() {
			m.sessionInput.Reset()
			m.sessionForm = true
			m.problem = ""
			return m, m.sessionInput.Focus()
		}
		if m.sessionIndex >= len(m.sessions) {
			m.problem = "归档 Agent 不能新建会话"
			return m, nil
		}
		selected := m.sessions[m.sessionIndex]
		m.loading = true
		m.status = "正在打开会话……"
		return m, openExistingSession(m.roster, m.conversation, selected)
	case "esc":
		m.focus = focusAgents
		return m, nil
	}
	return m, nil
}

func (m *tuiModel) updateSessionForm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		id := strings.TrimSpace(m.sessionInput.Value())
		if id == "" {
			m.problem = "会话名不能为空"
			return m, nil
		}
		m.loading = true
		m.status = "正在创建会话……"
		m.sessionForm = false
		m.sessionInput.Blur()
		return m, openStartedSession(m.roster, m.conversation, m.selectedAgentID(), id)
	case "esc":
		m.sessionForm = false
		m.sessionInput.Blur()
		m.problem = ""
		return m, nil
	}
	var cmd tea.Cmd
	m.sessionInput, cmd = m.sessionInput.Update(msg)
	return m, cmd
}

func (m *tuiModel) updateChatKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.help.ShowAll {
		return m, nil
	}
	if msg.String() == "pgup" {
		m.viewport.PageUp()
		return m, nil
	}
	if msg.String() == "pgdown" {
		m.viewport.PageDown()
		return m, nil
	}
	if msg.String() == "esc" {
		m.focus = focusSessions
		m.textarea.Blur()
		return m, nil
	}
	if m.conversation == nil {
		m.problem = "请先打开或新建一个会话"
		return m, nil
	}
	if msg.Key().Code == tea.KeyEnter && !msg.Mod.Contains(tea.ModShift) {
		text := strings.TrimSpace(m.textarea.Value())
		if text == "" {
			return m, nil
		}
		m.textarea.Reset()
		if text == "/exit" || text == ":exit" {
			return m, tea.Quit
		}
		if m.handleLocalCommand(text) {
			return m, nil
		}
		if m.conversation == nil {
			m.problem = "请先打开一个会话"
			return m, nil
		}
		m.loading = true
		m.status = "消息发送中……"
		return m, submitFollowup(m.conversation, text)
	}
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

func (m *tuiModel) handleLocalCommand(text string) bool {
	switch text {
	case "/help", ":help":
		m.help.ShowAll = true
	case "/cancel", ":cancel":
		if m.conversation != nil {
			m.conversation.Cancel()
			m.status = "已请求取消当前步骤"
		}
	case "/history", ":history":
		if m.conversation != nil {
			m.transcript = m.conversation.Book().Events()
			m.lastSeq = lastEventSequence(m.transcript)
			m.refreshTranscript(true)
		}
	case "/agents", ":agents":
		m.focus = focusAgents
		m.textarea.Blur()
	case "/sessions", ":sessions":
		m.focus = focusSessions
		m.textarea.Blur()
	default:
		return false
	}
	return true
}

func (m *tuiModel) applyEvent(msg tea.Msg) {
	switch value := msg.(type) {
	case appendedMessage:
		if m.conversation == nil || value.value.SessionID != m.conversation.SessionID() {
			return
		}
		if value.value.Event.Seq <= m.lastSeq {
			return
		}
		m.transcript = append(m.transcript, value.value.Event)
		m.lastSeq = value.value.Event.Seq
		m.refreshTranscript(false)
	case conversationErrorMessage:
		if m.conversation == nil || value.value.SessionID != m.conversation.SessionID() {
			return
		}
		m.problem = value.value.Message
	}
}

func (m *tuiModel) refreshTranscript(forceBottom bool) {
	atBottom := forceBottom || m.viewport.AtBottom()
	m.viewport.SetContent(renderTranscript(m.transcript))
	if atBottom {
		m.viewport.GotoBottom()
	}
}

func (m *tuiModel) resize(width int, height int) {
	if width < 1 {
		width = 80
	}
	if height < 1 {
		height = 24
	}
	m.width = width
	m.height = height
	sidebarWidth := width / 4
	if sidebarWidth < 24 {
		sidebarWidth = 24
	}
	if sidebarWidth > 34 {
		sidebarWidth = 34
	}
	mainWidth := width - sidebarWidth - 1
	if mainWidth < 20 {
		mainWidth = 20
	}
	m.viewport.SetWidth(mainWidth)
	m.viewport.SetHeight(height - 8)
	if m.viewport.Height() < 5 {
		m.viewport.SetHeight(5)
	}
	m.textarea.SetWidth(mainWidth - 2)
	m.help.SetWidth(width - 4)
	if m.form != nil {
		m.form.setWidth(mainWidth - 6)
	}
}

func (m *tuiModel) nextFocus() {
	switch m.focus {
	case focusAgents:
		m.focus = focusSessions
	case focusSessions:
		if m.conversation != nil {
			m.focus = focusChat
		} else {
			m.focus = focusAgents
		}
	default:
		m.focus = focusAgents
	}
}

func (m *tuiModel) focusInput() tea.Cmd {
	if m.focus == focusChat && m.conversation != nil {
		return m.textarea.Focus()
	}
	m.textarea.Blur()
	return nil
}

func (m *tuiModel) selectedAgentID() string {
	if m.agentIndex < 0 || m.agentIndex >= len(m.profiles) {
		return ""
	}
	return m.profiles[m.agentIndex].ID
}

func (m *tuiModel) agentCount() int {
	return len(m.profiles) + 1
}

func (m *tuiModel) sessionCount() int {
	if m.selectedAgentID() == "" {
		return 0
	}
	if m.profiles[m.agentIndex].Archived {
		return len(m.sessions)
	}
	return len(m.sessions) + 1
}

func (m *tuiModel) canCreateSession() bool {
	return m.selectedAgentID() != "" && !m.profiles[m.agentIndex].Archived
}

func (m *tuiModel) closeConversation() error {
	if m.conversation == nil {
		return nil
	}
	err := m.conversation.Close()
	m.conversation = nil
	return err
}

func (m *tuiModel) View() tea.View {
	if m.help.ShowAll {
		view := tea.NewView(renderHelp(m))
		view.AltScreen = true
		return view
	}
	if m.form != nil {
		view := tea.NewView(renderForm(m))
		view.AltScreen = true
		return view
	}
	if m.sessionForm {
		view := tea.NewView(renderSessionForm(m))
		view.AltScreen = true
		return view
	}

	sidebar := renderSidebar(m)
	main := renderMain(m)
	content := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, main)
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "edith-harness"
	if m.focus == focusChat && m.conversation != nil {
		cursor := m.textarea.Cursor()
		if cursor != nil {
			cursor.X += lipgloss.Width(sidebar)
			cursor.Y += m.height - m.textarea.Height() - 2
			view.Cursor = cursor
		}
	}
	return view
}

func renderSidebar(m *tuiModel) string {
	width := m.width / 4
	if width < 24 {
		width = 24
	}
	if width > 34 {
		width = 34
	}
	var b strings.Builder
	b.WriteString(sidebarTitle.Render("AGENTS"))
	for i, profile := range m.profiles {
		prefix := "  "
		if m.focus == focusAgents && i == m.agentIndex {
			prefix = "› "
		}
		name := profile.ID
		if profile.Archived {
			name += " (归档)"
		}
		b.WriteString(sidebarItem(m.focus == focusAgents && i == m.agentIndex, prefix+name))
	}
	newAgentIndex := len(m.profiles)
	b.WriteString(sidebarItem(m.focus == focusAgents && m.agentIndex == newAgentIndex, "  + 新建 Agent"))
	b.WriteString("\n")
	b.WriteString(sidebarTitle.Render("SESSIONS"))
	for i, item := range m.sessions {
		prefix := "  "
		if m.focus == focusSessions && i == m.sessionIndex {
			prefix = "› "
		}
		name := item.ID
		if item.Open {
			name += " (打开)"
		}
		b.WriteString(sidebarItem(m.focus == focusSessions && i == m.sessionIndex, prefix+name))
	}
	if m.selectedAgentID() != "" && !m.profiles[m.agentIndex].Archived {
		newSessionIndex := len(m.sessions)
		b.WriteString(sidebarItem(m.focus == focusSessions && m.sessionIndex == newSessionIndex, "  + 新建会话"))
	}
	return sidebarStyle.Width(width).Height(m.height).Render(b.String())
}

func sidebarItem(active bool, text string) string {
	style := sidebarItemStyle
	if active {
		style = sidebarActiveStyle
	}
	return style.Render(text) + "\n"
}

func renderMain(m *tuiModel) string {
	header := "未选择会话"
	status := m.status
	if m.conversation != nil {
		header = m.conversation.AgentID() + " / " + m.conversation.SessionID()
		if m.conversation.State() == "busy" {
			status = "运行中"
		}
	}
	if status == "" && m.loading {
		status = "处理中……"
	}
	headerView := headerStyle.Render(header)
	if status != "" {
		headerView += "  " + statusStyle.Render(status)
	}
	body := m.viewport.View()
	composer := m.textarea.View()
	if m.conversation == nil {
		body = emptyStateStyle.Height(m.viewport.Height()).AlignVertical(lipgloss.Center).Render("先在左侧选择 Agent，再打开或新建会话")
		composer = disabledInputStyle.Render("打开会话后，这里可以输入消息")
	}
	footer := ""
	if m.problem != "" {
		footer = errorStyle.Render(m.problem)
	} else if m.status != "" {
		footer = mutedStyle.Render(m.status)
	}
	mainWidth := m.width - lipgloss.Width(renderSidebar(m)) - 1
	if mainWidth < 20 {
		mainWidth = 20
	}
	return mainStyle.Width(mainWidth).Height(m.height).Render(
		lipgloss.JoinVertical(lipgloss.Left, headerView, body, composer, footer),
	)
}

func renderHelp(m *tuiModel) string {
	title := titleStyle.Render("edith-harness · 帮助")
	body := m.help.FullHelpView(m.keys.FullHelp())
	return helpStyle.Width(m.width - 4).Height(m.height - 2).Render(
		lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", mutedStyle.Render("Esc 关闭帮助")),
	)
}

func loadProfiles(roster agents.Service) tea.Cmd {
	return func() tea.Msg {
		profiles, err := roster.ListAgents()
		return profilesLoadedMessage{profiles: profiles, err: err}
	}
}

func loadSessions(roster agents.Service, agentID string) tea.Cmd {
	return func() tea.Msg {
		sessions, err := roster.ListSessions(agentID)
		return sessionsLoadedMessage{agentID: agentID, sessions: sessions, err: err}
	}
}

func createAgent(roster agents.Service, profile agents.AgentProfile) tea.Cmd {
	return func() tea.Msg {
		err := roster.CreateAgent(profile)
		return agentCreatedMessage{profile: profile, err: err}
	}
}

func openExistingSession(roster agents.Service, old agents.Conversation, summary agents.SessionSummary) tea.Cmd {
	return func() tea.Msg {
		if old != nil && old.SessionID() == summary.ID {
			return conversationOpenedMessage{agentID: summary.AgentID, sessionID: summary.ID, conversation: old}
		}
		if old != nil {
			err := old.Close()
			if err != nil {
				return conversationOpenedMessage{err: err}
			}
		}
		var conversation agents.Conversation
		var err error
		if summary.Open {
			conversation, err = roster.GetSession(summary.ID)
		} else {
			conversation, err = roster.ResumeSession(summary.ID)
		}
		return conversationOpenedMessage{
			agentID:      summary.AgentID,
			sessionID:    summary.ID,
			conversation: conversation,
			err:          err,
		}
	}
}

func openStartedSession(roster agents.Service, old agents.Conversation, agentID string, sessionID string) tea.Cmd {
	return func() tea.Msg {
		if old != nil {
			err := old.Close()
			if err != nil {
				return conversationOpenedMessage{err: err}
			}
		}
		conversation, err := roster.StartSession(agentID, sessionID)
		return conversationOpenedMessage{
			agentID:      agentID,
			sessionID:    sessionID,
			conversation: conversation,
			err:          err,
		}
	}
}

func submitFollowup(conversation agents.Conversation, text string) tea.Cmd {
	return func() tea.Msg {
		err := conversation.SubmitFollowup(text)
		return followupSubmittedMessage{err: err}
	}
}

type agentForm struct {
	inputs    []textinput.Model
	field     int
	providers []llm.ProviderInfo
	provider  int
	thinking  int
	toolNames []string
	problem   string
}

const (
	formID = iota
	formProvider
	formModel
	formThinking
	formPrompt
	formTools
	agentFormFieldCount
)

func newAgentForm(models *llm.Service, registry *tools.Registry) (*agentForm, error) {
	providers := models.Providers()
	if len(providers) == 0 {
		return nil, errors.New("没有已安装的模型服务商")
	}
	inputs := []textinput.Model{
		newFormInput("例如：小红"),
		newFormInput("例如：deepseek-chat"),
		newFormInput("例如：傲娇小猫娘"),
		newFormInput("名称或编号，多个用逗号分隔"),
	}
	return &agentForm{
		inputs:    inputs,
		providers: providers,
		toolNames: registry.Names(),
	}, nil
}

func newFormInput(placeholder string) textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = placeholder
	input.SetStyles(formInputStyles())
	input.SetWidth(42)
	return input
}

func (f *agentForm) focus() tea.Cmd {
	for i := range f.inputs {
		f.inputs[i].Blur()
	}
	index := f.inputIndex()
	if index >= 0 {
		return f.inputs[index].Focus()
	}
	return nil
}

func (f *agentForm) setWidth(width int) {
	inputWidth := width - 18
	if inputWidth < 20 {
		inputWidth = 20
	}
	if inputWidth > 42 {
		inputWidth = 42
	}
	for i := range f.inputs {
		f.inputs[i].SetWidth(inputWidth)
	}
}

func (f *agentForm) update(msg tea.Msg) tea.Cmd {
	index := f.inputIndex()
	if index < 0 {
		return nil
	}
	var cmd tea.Cmd
	f.inputs[index], cmd = f.inputs[index].Update(msg)
	return cmd
}

func (f *agentForm) inputIndex() int {
	switch f.field {
	case formID:
		return 0
	case formModel:
		return 1
	case formPrompt:
		return 2
	case formTools:
		return 3
	default:
		return -1
	}
}

func (f *agentForm) moveChoice(step int) {
	switch f.field {
	case formProvider:
		f.provider = (f.provider + step + len(f.providers)) % len(f.providers)
		f.thinking = 0
	case formThinking:
		levels := f.thinkingLevels()
		f.thinking = (f.thinking + step + len(levels)) % len(levels)
	}
}

func (f *agentForm) thinkingLevels() []string {
	levels := f.providers[f.provider].ThinkingLevels
	if len(levels) == 0 {
		return []string{"off"}
	}
	return levels
}

func (f *agentForm) profile() (agents.AgentProfile, error) {
	tools, err := chooseTools(f.inputs[3].Value(), f.toolNames)
	if err != nil {
		return agents.AgentProfile{}, err
	}
	return agents.AgentProfile{
		ID:           strings.TrimSpace(f.inputs[0].Value()),
		Provider:     f.providers[f.provider].Name,
		Model:        strings.TrimSpace(f.inputs[1].Value()),
		Thinking:     f.thinkingLevels()[f.thinking],
		SystemPrompt: strings.TrimSpace(f.inputs[2].Value()),
		Tools:        tools,
	}, nil
}

func renderForm(m *tuiModel) string {
	f := m.form
	provider := f.providers[f.provider].Name
	levels := f.thinkingLevels()
	var b strings.Builder
	b.WriteString(titleStyle.Render("新建 Agent"))
	b.WriteString("\n\n")
	b.WriteString(formFieldView(f, formID, "Agent 名字", f.inputs[0].View()))
	b.WriteString(formFieldView(f, formProvider, "模型服务商", provider+"（←/→ 切换）"))
	b.WriteString(formFieldView(f, formModel, "模型名", f.inputs[1].View()))
	b.WriteString(formFieldView(f, formThinking, "思考档位", levels[f.thinking]+"（←/→ 切换）"))
	b.WriteString(formFieldView(f, formPrompt, "人设", f.inputs[2].View()))
	b.WriteString(formFieldView(f, formTools, "工具白名单", f.inputs[3].View()))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("Enter 下一项 · Esc 返回"))
	if f.problem != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(f.problem))
	}
	return formStyle.Width(formPanelWidth(m.width)).Render(b.String())
}

func formFieldView(form *agentForm, field int, label string, value string) string {
	prefix := "  "
	style := mutedStyle
	if form.field == field {
		prefix = "› "
		style = focusedFormStyle
	}
	return style.Render(prefix+padLabel(label, 12)) + value + "\n"
}

func renderSessionForm(m *tuiModel) string {
	body := strings.Join([]string{
		titleStyle.Render("新建会话"),
		"",
		focusedFormStyle.Render("› "+padLabel("会话名", 12)) + m.sessionInput.View(),
		"",
		mutedStyle.Render("Enter 创建 · Esc 返回"),
	}, "\n")
	return formStyle.Width(formPanelWidth(m.width)).Render(body)
}

func formPanelWidth(width int) int {
	panelWidth := width - 6
	if panelWidth > 72 {
		return 72
	}
	return panelWidth
}

func padLabel(label string, width int) string {
	padding := width - lipgloss.Width(label)
	if padding < 1 {
		padding = 1
	}
	return label + strings.Repeat(" ", padding)
}

func renderTranscript(events []session.Event) string {
	var b strings.Builder
	calls := make(map[string]string)
	streaming := false
	for _, event := range visibleEvents(events) {
		switch event.Kind {
		case session.KindUserMessage:
			breakStream(&b, &streaming)
			var data session.UserMessageData
			if json.Unmarshal(event.Data, &data) == nil {
				writeMessage(&b, "你", data.Text)
			}
		case session.KindChunk:
			var data session.ChunkData
			if json.Unmarshal(event.Data, &data) == nil {
				if !streaming {
					b.WriteString(assistantStyle.Render("小红"))
					b.WriteString("：")
					streaming = true
				}
				b.WriteString(safeText(data.Delta))
			}
		case session.KindAssistantFinal:
			var data session.AssistantFinalData
			if json.Unmarshal(event.Data, &data) == nil {
				breakStream(&b, &streaming)
				if data.Text != "" {
					writeMessage(&b, "小红", data.Text)
				}
			}
		case session.KindToolCall:
			var data session.ToolCallData
			if json.Unmarshal(event.Data, &data) == nil {
				calls[data.ID] = data.Name
				breakStream(&b, &streaming)
				b.WriteString(toolStyle.Render("TOOL"))
				b.WriteString(" ")
				b.WriteString(safeText(data.Name))
				b.WriteString("\n")
			}
		case session.KindToolStart:
			var data session.ToolStartData
			if json.Unmarshal(event.Data, &data) == nil {
				breakStream(&b, &streaming)
				b.WriteString(mutedStyle.Render("工具执行中：" + toolName(calls, data.CallID)))
				b.WriteString("\n")
			}
		case session.KindToolResult:
			var data session.ToolResultData
			if json.Unmarshal(event.Data, &data) == nil {
				breakStream(&b, &streaming)
				line := "工具完成：" + toolName(calls, data.CallID) + "（" + data.Status + "）"
				if data.Output != "" {
					line += " " + preview(data.Output)
				}
				b.WriteString(toolStyle.Render(safeText(line)))
				b.WriteString("\n")
			}
		}
	}
	breakStream(&b, &streaming)
	if b.Len() == 0 {
		return mutedStyle.Render("还没有消息。选择一个会话，或者在左侧新建。")
	}
	return b.String()
}

func writeMessage(builder *strings.Builder, who string, text string) {
	if who == "小红" {
		builder.WriteString(assistantStyle.Render(who))
	} else {
		builder.WriteString(userStyle.Render(who))
	}
	builder.WriteString("：")
	builder.WriteString(safeText(text))
	builder.WriteString("\n\n")
}

func breakStream(builder *strings.Builder, streaming *bool) {
	if !*streaming {
		return
	}
	builder.WriteString("\n\n")
	*streaming = false
}

func toolName(calls map[string]string, callID string) string {
	name := calls[callID]
	if name == "" {
		return callID
	}
	return name
}

func lastEventSequence(events []session.Event) int {
	if len(events) == 0 {
		return 0
	}
	return events[len(events)-1].Seq
}

func clampIndex(index int, count int) int {
	if count <= 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index >= count {
		return count - 1
	}
	return index
}

func indexOfAgent(profiles []agents.AgentProfile, id string) int {
	for i, profile := range profiles {
		if profile.ID == id {
			return i
		}
	}
	return 0
}

type tuiKeyMap struct {
	Up      key.Binding
	Down    key.Binding
	Confirm key.Binding
	Next    key.Binding
	Help    key.Binding
	Quit    key.Binding
	Cancel  key.Binding
}

func newTUIKeyMap() tuiKeyMap {
	return tuiKeyMap{
		Up:      key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "上移")),
		Down:    key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "下移")),
		Confirm: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "确认")),
		Next:    key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "切换区域")),
		Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "帮助")),
		Quit:    key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "退出")),
		Cancel:  key.NewBinding(key.WithKeys("/cancel"), key.WithHelp("/cancel", "取消")),
	}
}

func (k tuiKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Confirm, k.Next, k.Help, k.Quit}
}

func (k tuiKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Up, k.Down, k.Confirm, k.Next}, {k.Help, k.Cancel, k.Quit}}
}

var (
	sidebarStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#17212b")).Padding(1, 2).BorderRight(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("#d0d7de"))
	sidebarTitle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#6e7781")).Bold(true)
	sidebarItemStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#24292f"))
	sidebarActiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#0969da")).Bold(true)
	mainStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("#24292f")).Padding(1, 2)
	headerStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#17212b")).Bold(true)
	statusStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#0f766e"))
	mutedStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#64748b"))
	errorStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#b42318"))
	userStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("#2563eb")).Bold(true)
	assistantStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#0f766e")).Bold(true)
	toolStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("#9a6700"))
	titleStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#1769aa")).Bold(true)
	helpStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("#24292f")).Padding(2, 3)
	formStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("#24292f")).Padding(2, 3)
	focusedFormStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#1769aa")).Bold(true)
	emptyStateStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#64748b")).AlignHorizontal(lipgloss.Center)
	disabledInputStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#8c959f"))
)

func chatTextareaStyles() textarea.Styles {
	styles := textarea.DefaultLightStyles()
	base := lipgloss.NewStyle().Foreground(lipgloss.Color("#24292f"))
	placeholder := lipgloss.NewStyle().Foreground(lipgloss.Color("#94a3b8"))
	prompt := lipgloss.NewStyle().Foreground(lipgloss.Color("#1769aa"))
	styles.Focused.Base = base
	styles.Focused.CursorLine = base
	styles.Focused.Placeholder = placeholder
	styles.Focused.Prompt = prompt
	styles.Focused.Text = base
	styles.Blurred.Base = base
	styles.Blurred.CursorLine = base
	styles.Blurred.Placeholder = placeholder
	styles.Blurred.Prompt = prompt
	styles.Blurred.Text = base
	styles.Cursor.Shape = tea.CursorBar
	styles.Cursor.Color = lipgloss.Color("#1769aa")
	return styles
}

func formInputStyles() textinput.Styles {
	styles := textinput.DefaultLightStyles()
	styles.Focused.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("#17212b"))
	styles.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("#1769aa"))
	styles.Focused.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("#94a3b8"))
	styles.Blurred.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("#17212b"))
	styles.Blurred.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("#64748b"))
	styles.Blurred.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("#94a3b8"))
	styles.Cursor.Shape = tea.CursorBar
	styles.Cursor.Color = lipgloss.Color("#1769aa")
	return styles
}
