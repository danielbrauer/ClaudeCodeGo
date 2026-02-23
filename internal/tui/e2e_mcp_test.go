package tui

import (
	"strings"
	"testing"
)

func TestE2E_MCPCommand_NoServers(t *testing.T) {
	m, _ := testModel(t) // mcpStatus is nil

	output := mcpText(&m)

	if !strings.Contains(output, "No MCP servers configured") {
		t.Errorf("mcp output with nil status should say no servers configured, got %q", output)
	}
}

func TestE2E_MCPCommand_EmptyServers(t *testing.T) {
	mcp := &mockMCPStatus{
		servers:  []string{},
		statuses: map[string]string{},
	}
	m, _ := testModel(t, withMCPStatus(mcp))

	output := mcpText(&m)

	if !strings.Contains(output, "No MCP servers connected") {
		t.Errorf("mcp output with empty servers should say no servers connected, got %q", output)
	}
}

func TestE2E_MCPCommand_WithServers(t *testing.T) {
	mcp := &mockMCPStatus{
		servers: []string{"github", "slack"},
		statuses: map[string]string{
			"github": "github: connected (3 tools)",
			"slack":  "slack: connected (5 tools)",
		},
	}
	m, _ := testModel(t, withMCPStatus(mcp))

	output := mcpText(&m)

	if !strings.Contains(output, "MCP servers (2)") {
		t.Errorf("mcp output should show server count, got %q", output)
	}
	if !strings.Contains(output, "github: connected (3 tools)") {
		t.Errorf("mcp output should show github status, got %q", output)
	}
	if !strings.Contains(output, "slack: connected (5 tools)") {
		t.Errorf("mcp output should show slack status, got %q", output)
	}
}
