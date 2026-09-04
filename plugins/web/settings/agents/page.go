package agents

import (
	"errors"
	"fmt"
	nethttp "net/http"
	"os"
	"strings"

	"github.com/a-h/templ"
	"harness/kernel/agents"
	"harness/kernel/loops"
	"harness/kernel/skills"
	"harness/kernel/tools"
	"harness/surface/web"
)

// 活对象。page 渲染 Agent 设置栏目并处理其表单。
type page struct {
	agents *agents.Service
}

// 数据。agentSettingsView 是 Agent 设置页面的渲染数据。
type agentSettingsView struct {
	Agents        []agentView
	Selected      agents.Agent
	Choices       agents.Choices
	IsNew         bool
	Error         string
	SelectedID    string
	SelectedInUse bool
}

// 数据。agentView 是左侧列表的一条 Agent 摘要。
type agentView struct {
	ID       string
	Name     string
	Selected bool
	InUse    bool
}

func newPage(agentService *agents.Service) *page {
	return &page{agents: agentService}
}

func (p *page) Definition() web.SettingsSectionDefinition {
	return web.SettingsSectionDefinition{ID: "agents", Title: "Agent 设置", Order: 10}
}

func (p *page) Render() (templ.Component, error) {
	view, err := p.view(agents.DefaultID, agents.Agent{}, false, "")
	if err != nil {
		return nil, err
	}
	return AgentSettings(view), nil
}

func (p *page) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	switch {
	case r.Method == nethttp.MethodGet:
		p.renderSelected(w, r, r.PathValue("agentID"))
	case r.Method == nethttp.MethodPost && strings.HasSuffix(r.URL.Path, "/delete"):
		p.delete(w, r)
	case r.Method == nethttp.MethodPost:
		p.save(w, r)
	default:
		nethttp.NotFound(w, r)
	}
}

func (p *page) renderSelected(w nethttp.ResponseWriter, r *nethttp.Request, id string) {
	if id == "new" {
		view, err := p.view("new", p.newAgent(), true, "")
		if err != nil {
			nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
			return
		}
		p.render(w, r, view)
		return
	}
	view, err := p.view(id, agents.Agent{}, false, "")
	if err != nil {
		status := nethttp.StatusInternalServerError
		if errors.Is(err, os.ErrNotExist) {
			status = nethttp.StatusNotFound
		}
		nethttp.Error(w, err.Error(), status)
		return
	}
	p.render(w, r, view)
}

func (p *page) save(w nethttp.ResponseWriter, r *nethttp.Request) {
	if err := r.ParseForm(); err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusBadRequest)
		return
	}
	id := r.PathValue("agentID")
	agent := agents.Agent{
		ID:           id,
		Name:         strings.TrimSpace(r.FormValue("name")),
		Kind:         strings.TrimSpace(r.FormValue("kind")),
		SystemPrompt: r.FormValue("systemPrompt"),
		Tools:        r.Form["tools"],
		Skills:       r.Form["skills"],
	}
	saved, err := p.agents.Save(agent)
	if err != nil {
		view, viewErr := p.view(id, agent, id == "", err.Error())
		if viewErr != nil {
			nethttp.Error(w, viewErr.Error(), nethttp.StatusInternalServerError)
			return
		}
		w.WriteHeader(nethttp.StatusBadRequest)
		p.render(w, r, view)
		return
	}
	view, err := p.view(saved.ID, agents.Agent{}, false, "")
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
		return
	}
	p.render(w, r, view)
}

func (p *page) delete(w nethttp.ResponseWriter, r *nethttp.Request) {
	id := r.PathValue("agentID")
	err := p.agents.Delete(id)
	if err != nil {
		view, viewErr := p.view(id, agents.Agent{}, false, err.Error())
		if viewErr != nil {
			nethttp.Error(w, viewErr.Error(), nethttp.StatusInternalServerError)
			return
		}
		w.WriteHeader(nethttp.StatusBadRequest)
		p.render(w, r, view)
		return
	}
	view, err := p.view(agents.DefaultID, agents.Agent{}, false, "")
	if err != nil {
		nethttp.Error(w, err.Error(), nethttp.StatusInternalServerError)
		return
	}
	p.render(w, r, view)
}

func (p *page) view(id string, draft agents.Agent, isNew bool, message string) (agentSettingsView, error) {
	all, err := p.agents.List()
	if err != nil {
		return agentSettingsView{}, err
	}
	if id == "" {
		id = agents.DefaultID
	}
	selected := draft
	if !isNew && draft.ID == "" {
		selected, err = p.agents.Get(id)
		if err != nil {
			return agentSettingsView{}, err
		}
	}
	views := make([]agentView, 0, len(all))
	selectedInUse := false
	for _, agent := range all {
		inUse, err := p.agents.InUse(agent.ID)
		if err != nil {
			return agentSettingsView{}, err
		}
		if agent.ID == id && !isNew {
			selectedInUse = inUse
		}
		views = append(views, agentView{ID: agent.ID, Name: agent.Name, Selected: agent.ID == id && !isNew, InUse: inUse})
	}
	return agentSettingsView{
		Agents: views, Selected: selected, Choices: p.agents.Choices(), IsNew: isNew, Error: message, SelectedID: id, SelectedInUse: selectedInUse,
	}, nil
}

func (p *page) newAgent() agents.Agent {
	choices := p.agents.Choices()
	if len(choices.Loops) == 0 {
		return agents.Agent{}
	}
	return agents.Agent{Kind: choices.Loops[0].Kind}
}

func (p *page) render(w nethttp.ResponseWriter, r *nethttp.Request, view agentSettingsView) {
	templ.Handler(AgentSettings(view)).ServeHTTP(w, r)
}

func selectedName(names []string, name string) bool {
	for _, current := range names {
		if current == name {
			return true
		}
	}
	return false
}

func loopDescription(definition loops.Definition) string { return definition.Description }

func toolDescription(definition tools.Definition) string { return definition.Description }

func skillSummary(skill skills.Skill) string { return skill.Summary }

func agentTitle(agent agents.Agent, isNew bool) string {
	if isNew {
		return "新建 Agent"
	}
	return fmt.Sprintf("编辑 %s", agent.Name)
}
