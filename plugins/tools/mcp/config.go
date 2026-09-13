package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	transportStdio = "stdio"
	transportHTTP  = "http"
)

var environmentPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-([^}]*))?\}`)
var serverNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// 数据。一个 Claude Code 风格 MCP 配置文件。
type configFile struct {
	MCPServers map[string]serverConfig `json:"mcpServers"`
}

// 数据。一条 MCP Server 的静态配置。
type serverConfig struct {
	Type         string            `json:"type"`
	Command      string            `json:"command,omitempty"`
	Args         []string          `json:"args,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
	CWD          string            `json:"cwd,omitempty"`
	URL          string            `json:"url,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	IncludeTools *[]string         `json:"includeTools,omitempty"`
	ExcludeTools []string          `json:"excludeTools,omitempty"`
}

// 数据。一条已确定来源目录、变量和 transport 的 Server 配置。
type serverSpec struct {
	Name         string
	Transport    string
	Command      string
	Args         []string
	Env          map[string]string
	CWD          string
	URL          string
	Headers      map[string]string
	IncludeTools *[]string
	ExcludeTools []string
}

func readConfig(path string) (configFile, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return configFile{}, false, nil
	}
	if err != nil {
		return configFile{}, false, fmt.Errorf("read %s: %w", path, err)
	}
	var config configFile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&config)
	if err != nil {
		return configFile{}, false, fmt.Errorf("parse %s: %w", path, err)
	}
	var trailing any
	err = decoder.Decode(&trailing)
	if !errors.Is(err, io.EOF) {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return configFile{}, false, fmt.Errorf("parse %s: %w", path, err)
	}
	if config.MCPServers == nil {
		config.MCPServers = make(map[string]serverConfig)
	}
	return config, true, nil
}

func normalizeServers(config configFile, baseDir string) ([]serverSpec, error) {
	names := make([]string, 0, len(config.MCPServers))
	for name := range config.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]serverSpec, 0, len(names))
	for _, name := range names {
		spec, err := normalizeServer(name, config.MCPServers[name], baseDir)
		if err != nil {
			return nil, err
		}
		out = append(out, spec)
	}
	return out, nil
}

func normalizeServer(name string, config serverConfig, baseDir string) (serverSpec, error) {
	if !serverNamePattern.MatchString(name) {
		return serverSpec{}, fmt.Errorf("mcp: server name %q may contain only letters, numbers, underscore, and hyphen", name)
	}
	transport := config.Type
	if transport == "streamable-http" {
		transport = transportHTTP
	}
	if transport != transportStdio && transport != transportHTTP {
		return serverSpec{}, fmt.Errorf("mcp: server %q has unsupported type %q", name, config.Type)
	}
	expand := func(field string, value string) (string, error) {
		value, err := expandEnvironment(value)
		if err != nil {
			return "", fmt.Errorf("mcp: server %q %s: %w", name, field, err)
		}
		return value, nil
	}
	command, err := expand("command", config.Command)
	if err != nil {
		return serverSpec{}, err
	}
	endpoint, err := expand("url", config.URL)
	if err != nil {
		return serverSpec{}, err
	}
	cwd, err := expand("cwd", config.CWD)
	if err != nil {
		return serverSpec{}, err
	}
	args, err := expandList("args", config.Args, expand)
	if err != nil {
		return serverSpec{}, err
	}
	environment, err := expandMap("env", config.Env, expand)
	if err != nil {
		return serverSpec{}, err
	}
	headers, err := expandMap("headers", config.Headers, expand)
	if err != nil {
		return serverSpec{}, err
	}

	spec := serverSpec{
		Name:         name,
		Transport:    transport,
		Command:      command,
		Args:         args,
		Env:          environment,
		CWD:          cwd,
		URL:          endpoint,
		Headers:      headers,
		IncludeTools: copyStringList(config.IncludeTools),
		ExcludeTools: append([]string(nil), config.ExcludeTools...),
	}
	if transport == transportStdio {
		if command == "" {
			return serverSpec{}, fmt.Errorf("mcp: stdio server %q needs command", name)
		}
		if endpoint != "" || len(headers) > 0 {
			return serverSpec{}, fmt.Errorf("mcp: stdio server %q cannot set url or headers", name)
		}
		if cwd == "" {
			spec.CWD = baseDir
		} else if !filepath.IsAbs(cwd) {
			spec.CWD = filepath.Join(baseDir, cwd)
		}
		return spec, nil
	}
	if endpoint == "" {
		return serverSpec{}, fmt.Errorf("mcp: HTTP server %q needs url", name)
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return serverSpec{}, fmt.Errorf("mcp: HTTP server %q has invalid url %q", name, endpoint)
	}
	if command != "" || len(args) > 0 || len(environment) > 0 || cwd != "" {
		return serverSpec{}, fmt.Errorf("mcp: HTTP server %q cannot set command, args, env, or cwd", name)
	}
	return spec, nil
}

func expandEnvironment(value string) (string, error) {
	var missing string
	expanded := environmentPattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := environmentPattern.FindStringSubmatch(match)
		if current, ok := os.LookupEnv(parts[1]); ok {
			return current
		}
		if parts[2] != "" {
			return parts[3]
		}
		missing = parts[1]
		return match
	})
	if missing != "" {
		return "", fmt.Errorf("environment variable %s is not set", missing)
	}
	if strings.Contains(expanded, "${") {
		return "", fmt.Errorf("invalid environment expression")
	}
	return expanded, nil
}

func expandList(field string, values []string, expand func(string, string) (string, error)) ([]string, error) {
	out := make([]string, len(values))
	for i, value := range values {
		expanded, err := expand(fmt.Sprintf("%s[%d]", field, i), value)
		if err != nil {
			return nil, err
		}
		out[i] = expanded
	}
	return out, nil
}

func expandMap(field string, values map[string]string, expand func(string, string) (string, error)) (map[string]string, error) {
	out := make(map[string]string, len(values))
	for key, value := range values {
		expanded, err := expand(field+"."+key, value)
		if err != nil {
			return nil, err
		}
		out[key] = expanded
	}
	return out, nil
}

func copyStringList(values *[]string) *[]string {
	if values == nil {
		return nil
	}
	copied := append([]string(nil), (*values)...)
	return &copied
}
