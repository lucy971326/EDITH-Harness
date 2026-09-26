package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"harness/internal/tools"
)

const (
	toolTimeout         = 10 * time.Minute
	maxInstructionBytes = 2 * 1024
)

// 活对象。MCP Server 的一条已连接会话及其工具路由。
type serverConnection struct {
	name         string
	stdio        bool
	session      *sdk.ClientSession
	definitions  []tools.Definition
	remoteNames  map[string]string
	instructions string
}

func connectServer(ctx context.Context, spec serverSpec) (*serverConnection, error) {
	var transport sdk.Transport
	if spec.Transport == transportStdio {
		command := exec.Command(spec.Command, spec.Args...)
		command.Dir = spec.CWD
		command.Env = mergedEnvironment(spec.Env)
		transport = &sdk.CommandTransport{Command: command}
	} else {
		client := &http.Client{Transport: headerTransport{base: http.DefaultTransport, headers: spec.Headers}}
		transport = &sdk.StreamableClientTransport{
			Endpoint:             spec.URL,
			HTTPClient:           client,
			DisableStandaloneSSE: true,
		}
	}
	client := sdk.NewClient(
		&sdk.Implementation{Name: "harness", Version: "v1"},
		&sdk.ClientOptions{Capabilities: &sdk.ClientCapabilities{}},
	)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp: connect server %q: %w", spec.Name, err)
	}
	connection, err := discoverServer(ctx, spec, session)
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	return connection, nil
}

func discoverServer(ctx context.Context, spec serverSpec, session *sdk.ClientSession) (*serverConnection, error) {
	allNames := make(map[string]struct{})
	remoteTools := make([]*sdk.Tool, 0)
	for remote, err := range session.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("mcp: list tools from %q: %w", spec.Name, err)
		}
		if remote == nil || remote.Name == "" {
			return nil, fmt.Errorf("mcp: server %q returned tool with empty name", spec.Name)
		}
		if _, duplicate := allNames[remote.Name]; duplicate {
			return nil, fmt.Errorf("mcp: server %q returned duplicate tool %q", spec.Name, remote.Name)
		}
		allNames[remote.Name] = struct{}{}
		remoteTools = append(remoteTools, remote)
	}
	err := validateFilters(spec, allNames)
	if err != nil {
		return nil, err
	}
	definitions := make([]tools.Definition, 0, len(remoteTools))
	routes := make(map[string]string, len(remoteTools))
	for _, remote := range remoteTools {
		if !included(spec, remote.Name) {
			continue
		}
		schema, err := json.Marshal(remote.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("mcp: marshal schema for %s/%s: %w", spec.Name, remote.Name, err)
		}
		name := "mcp__" + spec.Name + "__" + remote.Name
		if len(name) > 128 {
			return nil, fmt.Errorf("mcp: wrapped tool name %q exceeds 128 bytes", name)
		}
		description := strings.TrimSpace(remote.Description)
		if description == "" {
			description = fmt.Sprintf("MCP tool %s from server %s.", remote.Name, spec.Name)
		}
		definitions = append(definitions, tools.Definition{Name: name, Description: description, InputSchema: schema})
		routes[name] = remote.Name
	}
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].Name < definitions[j].Name })
	instructions := ""
	if initialized := session.InitializeResult(); initialized != nil {
		instructions = truncateInstruction(strings.TrimSpace(initialized.Instructions))
	}
	return &serverConnection{
		name:         spec.Name,
		stdio:        spec.Transport == transportStdio,
		session:      session,
		definitions:  definitions,
		remoteNames:  routes,
		instructions: instructions,
	}, nil
}

func stateFromConnections(connections []*serverConnection) *workspaceState {
	snapshot, routes := snapshotFromConnections(connections)
	return &workspaceState{snapshot: snapshot, routes: routes}
}

func snapshotFromConnections(connections []*serverConnection) (tools.Snapshot, map[string]*serverConnection) {
	var snapshot tools.Snapshot
	routes := make(map[string]*serverConnection)
	for _, connection := range connections {
		toolNames := make([]string, 0, len(connection.definitions))
		for _, definition := range connection.definitions {
			snapshot.Definitions = append(snapshot.Definitions, definition)
			toolNames = append(toolNames, definition.Name)
			routes[definition.Name] = connection
		}
		if connection.instructions != "" && len(toolNames) > 0 {
			snapshot.Instructions = append(snapshot.Instructions, tools.Instruction{
				Source:    connection.name,
				ToolNames: toolNames,
				Text:      connection.instructions,
			})
		}
	}
	sort.Slice(snapshot.Definitions, func(i, j int) bool { return snapshot.Definitions[i].Name < snapshot.Definitions[j].Name })
	return snapshot, routes
}

func copySnapshot(snapshot tools.Snapshot) tools.Snapshot {
	out := tools.Snapshot{
		Definitions: append([]tools.Definition(nil), snapshot.Definitions...),
	}
	for _, instruction := range snapshot.Instructions {
		instruction.ToolNames = append([]string(nil), instruction.ToolNames...)
		out.Instructions = append(out.Instructions, instruction)
	}
	return out
}

func validateFilters(spec serverSpec, available map[string]struct{}) error {
	if spec.IncludeTools != nil {
		for _, name := range *spec.IncludeTools {
			if _, ok := available[name]; !ok {
				return fmt.Errorf("mcp: server %q includes unknown tool %q", spec.Name, name)
			}
		}
	}
	for _, name := range spec.ExcludeTools {
		if _, ok := available[name]; !ok {
			return fmt.Errorf("mcp: server %q excludes unknown tool %q", spec.Name, name)
		}
	}
	return nil
}

func included(spec serverSpec, name string) bool {
	if spec.IncludeTools != nil {
		found := false
		for _, allowed := range *spec.IncludeTools {
			found = found || allowed == name
		}
		if !found {
			return false
		}
	}
	for _, excluded := range spec.ExcludeTools {
		if excluded == name {
			return false
		}
	}
	return true
}

func mergedEnvironment(overrides map[string]string) []string {
	values := make(map[string]string)
	for _, pair := range os.Environ() {
		if key, value, ok := strings.Cut(pair, "="); ok {
			values[key] = value
		}
	}
	for key, value := range overrides {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}

func truncateInstruction(text string) string {
	if len(text) <= maxInstructionBytes {
		return text
	}
	text = text[:maxInstructionBytes]
	for !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	return text
}

// 活对象。给 Streamable HTTP 请求补充静态 Header 的 transport。
type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t headerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	cloned := request.Clone(request.Context())
	cloned.Header = request.Header.Clone()
	for name, value := range t.headers {
		if cloned.Header.Get(name) == "" {
			cloned.Header.Set(name, value)
		}
	}
	return t.base.RoundTrip(cloned)
}
