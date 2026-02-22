package mcp

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/anthropics/claude-code-go/internal/tools"
)

// Manager coordinates MCP server lifecycles: starting servers,
// discovering tools, registering them, and shutting down.
type Manager struct {
	mu      sync.Mutex
	clients map[string]*MCPClient // keyed by server name
	cwd     string
}

// NewManager creates a new MCP manager.
func NewManager(cwd string) *Manager {
	return &Manager{
		clients: make(map[string]*MCPClient),
		cwd:     cwd,
	}
}

// StartServers connects to all configured MCP servers, discovers their tools,
// and registers them in the provided tool registry.
func (m *Manager) StartServers(ctx context.Context, configs map[string]ServerConfig, registry *tools.Registry) error {
	var firstErr error

	for name, cfg := range configs {
		client, err := m.startServer(ctx, name, cfg)
		if err != nil {
			fmt.Printf("Warning: MCP server %q failed to start: %v\n", name, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		m.mu.Lock()
		m.clients[name] = client
		m.mu.Unlock()

		// Discover and register tools from this server.
		mcpTools, err := client.ListTools(ctx)
		if err != nil {
			fmt.Printf("Warning: MCP server %q tool discovery failed: %v\n", name, err)
			continue
		}

		for _, tool := range mcpTools {
			wrapper := NewMCPToolWrapper(name, tool, client)
			registry.Register(wrapper)
		}

		fmt.Printf("MCP server %q: %d tools registered\n", name, len(mcpTools))
	}

	return firstErr
}

// startServer creates a transport, connects, and initializes a single MCP server.
func (m *Manager) startServer(ctx context.Context, name string, cfg ServerConfig) (*MCPClient, error) {
	transport, err := m.transportForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create transport: %w", err)
	}

	// For SSE transports, establish the connection first.
	if sseT, ok := transport.(*SSETransport); ok {
		if err := sseT.Connect(ctx); err != nil {
			transport.Close()
			return nil, fmt.Errorf("SSE connect: %w", err)
		}
	}

	client := NewMCPClient(name, transport)

	if err := client.Initialize(ctx); err != nil {
		transport.Close()
		return nil, fmt.Errorf("initialize: %w", err)
	}

	return client, nil
}

// transportForConfig creates the appropriate transport based on the config.
func (m *Manager) transportForConfig(cfg ServerConfig) (Transport, error) {
	if cfg.URL != "" {
		return NewSSETransport(cfg.URL), nil
	}
	if cfg.Command == "" {
		return nil, fmt.Errorf("server config must have either 'url' or 'command'")
	}
	return NewStdioTransport(cfg.Command, cfg.Args, cfg.Env, m.cwd)
}

// Shutdown gracefully closes all server connections.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, client := range m.clients {
		if err := client.Close(); err != nil {
			fmt.Printf("Warning: error closing MCP server %q: %v\n", name, err)
		}
	}
	m.clients = make(map[string]*MCPClient)
}

// Servers returns the sorted list of connected server names.
func (m *Manager) Servers() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	names := make([]string, 0, len(m.clients))
	for name := range m.clients {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Client returns the client for a named server.
func (m *Manager) Client(name string) (*MCPClient, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	client, ok := m.clients[name]
	return client, ok
}

// ServerStatus returns a human-readable status string for an MCP server.
func (m *Manager) ServerStatus(name string) string {
	m.mu.Lock()
	client, ok := m.clients[name]
	m.mu.Unlock()

	if !ok {
		return fmt.Sprintf("%s: not connected", name)
	}

	info := client.ServerInfoResult()
	caps := client.Capabilities()

	status := fmt.Sprintf("%s: connected", name)
	if info.Name != "" {
		status += fmt.Sprintf(" (server: %s", info.Name)
		if info.Version != "" {
			status += fmt.Sprintf(" v%s", info.Version)
		}
		status += ")"
	}

	features := []string{}
	if caps.Tools != nil {
		features = append(features, "tools")
	}
	if caps.Resources != nil {
		features = append(features, "resources")
	}
	if len(features) > 0 {
		status += fmt.Sprintf(" [%s]", joinStrings(features, ", "))
	}

	return status
}

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
