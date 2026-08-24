package compose

import (
	"fmt"
	"path/filepath"

	"harness/agents"
	"harness/core"
	"harness/llm"
	"harness/llmplugins/deepseek"
	"harness/localenv"
	"harness/loop"
	"harness/persistence/jsonl"
	"harness/persistence/profilejson"
	"harness/session"
	filetools "harness/toolplugins/files"
	"harness/tools"
)

// pluginSelection 放本次启动选中的大零件；固定插座不交给配置选择。
type pluginSelection struct {
	profileStore core.Plugin
	journal      core.Plugin
	llmAdapters  []core.Plugin
	environment  core.Plugin
	toolPlugins  []core.Plugin
	runner       core.Plugin
}

func selectPlugins(config Config, home string, workspace string) (pluginSelection, error) {
	var selected pluginSelection

	switch config.Plugins.ProfileStore {
	case "profilejson":
		selected.profileStore = profilejson.Plugin{Root: filepath.Join(home, "agents")}
	default:
		return pluginSelection{}, fmt.Errorf("未知的 plugins.profile_store：%s", config.Plugins.ProfileStore)
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
			provider := config.Providers.DeepSeek
			if provider.APIKey == "" {
				return pluginSelection{}, fmt.Errorf("配置缺少 providers.deepseek.api_key")
			}
			if provider.BaseURL == "" {
				provider.BaseURL = defaultDeepSeekBaseURL
			}
			selected.llmAdapters = append(selected.llmAdapters, deepseek.Plugin{
				APIKey:  provider.APIKey,
				BaseURL: provider.BaseURL,
			})
		default:
			return pluginSelection{}, fmt.Errorf("未知的 plugins.llm_adapters：%s", name)
		}
	}

	switch config.Plugins.Environment {
	case "localenv":
		selected.environment = localenv.Plugin{Root: workspace}
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

	return selected, nil
}

func (p pluginSelection) ordered() []core.Plugin {
	plugins := []core.Plugin{
		p.profileStore,
		p.journal,
		session.Plugin{},
		llm.Plugin{},
	}
	plugins = append(plugins, p.llmAdapters...)
	plugins = append(plugins, tools.Plugin{}, p.environment)
	plugins = append(plugins, p.toolPlugins...)
	plugins = append(plugins, agents.Plugin{}, p.runner)
	return plugins
}
