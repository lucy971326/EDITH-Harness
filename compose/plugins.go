package compose

import (
	"fmt"
	"path/filepath"

	"harness/agents"
	"harness/commands"
	cfg "harness/config"
	"harness/core"
	"harness/llm"
	"harness/llmplugins/deepseek"
	"harness/localenv"
	"harness/loop"
	"harness/persistence/jsonl"
	"harness/persistence/presetjson"
	"harness/persistence/projectjson"
	"harness/presets"
	"harness/projects"
	"harness/session"
	filetools "harness/toolplugins/files"
	"harness/tools"
	terminalui "harness/uiplugins/terminal"
	webui "harness/uiplugins/web"
)

// pluginSelection 放本次启动选中的大零件；固定插座不交给配置选择。
type pluginSelection struct {
	home         string
	projectStore core.Plugin
	presetStore  core.Plugin
	journal      core.Plugin
	llmAdapters  []core.Plugin
	environment  core.Plugin
	toolPlugins  []core.Plugin
	runner       core.Plugin
	ui           core.Plugin
}

func selectPlugins(config Config, home string) (pluginSelection, error) {
	selected := pluginSelection{home: home}

	switch config.Plugins.ProjectStore {
	case "projectjson":
		selected.projectStore = projectjson.Plugin{Root: filepath.Join(home, "projects")}
	default:
		return pluginSelection{}, fmt.Errorf("未知的 plugins.project_store：%s", config.Plugins.ProjectStore)
	}

	switch config.Plugins.PresetStore {
	case "presetjson":
		selected.presetStore = presetjson.Plugin{Root: filepath.Join(home, "presets")}
	default:
		return pluginSelection{}, fmt.Errorf("未知的 plugins.preset_store：%s", config.Plugins.PresetStore)
	}

	switch config.Plugins.Journal {
	case "jsonl":
		selected.journal = jsonl.Plugin{Root: filepath.Join(home, "sessions")}
	default:
		return pluginSelection{}, fmt.Errorf("未知的 plugins.journal：%s", config.Plugins.Journal)
	}

	for _, name := range config.Plugins.LLMAdapters {
		switch name {
		case "deepseek":
			selected.llmAdapters = append(selected.llmAdapters, deepseek.Plugin{})
		default:
			return pluginSelection{}, fmt.Errorf("未知的 plugins.llm_adapters：%s", name)
		}
	}

	switch config.Plugins.Environment {
	case "localenv":
		selected.environment = localenv.Plugin{}
	default:
		return pluginSelection{}, fmt.Errorf("未知的 plugins.environment：%s", config.Plugins.Environment)
	}

	for _, name := range config.Plugins.ToolPlugins {
		switch name {
		case "files":
			selected.toolPlugins = append(selected.toolPlugins, filetools.Plugin{})
		default:
			return pluginSelection{}, fmt.Errorf("未知的 plugins.tool_plugins：%s", name)
		}
	}

	switch config.Plugins.Runner {
	case "loop":
		selected.runner = loop.Plugin{}
	default:
		return pluginSelection{}, fmt.Errorf("未知的 plugins.runner：%s", config.Plugins.Runner)
	}

	switch config.Plugins.UI {
	case "terminal":
		selected.ui = terminalui.Plugin{}
	case "web":
		selected.ui = webui.Plugin{}
	default:
		return pluginSelection{}, fmt.Errorf("未知的 plugins.ui：%s", config.Plugins.UI)
	}

	return selected, nil
}

func (p pluginSelection) ordered() []core.Plugin {
	plugins := []core.Plugin{
		cfg.Plugin{Home: p.home},
		p.projectStore,
		p.presetStore,
		p.journal,
		session.Plugin{},
		llm.Plugin{},
	}
	plugins = append(plugins, p.llmAdapters...)
	plugins = append(plugins, tools.Plugin{}, commands.Plugin{}, p.environment)
	plugins = append(plugins, p.toolPlugins...)
	plugins = append(plugins, projects.Plugin{}, presets.Plugin{}, agents.Plugin{}, p.runner, p.ui)
	return plugins
}
