package tui

import (
	"fmt"
	"strings"
)

// registerMCPCommand registers /mcp.
func registerMCPCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "mcp",
		Description: "Show MCP server status",
		Execute:     textCommand(mcpText),
	})
}

func mcpText(m *model) string {
	if m.mcpStatus == nil {
		return "No MCP servers configured."
	}
	servers := m.mcpStatus.Servers()
	if len(servers) == 0 {
		return "No MCP servers connected."
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("MCP servers (%d):\n", len(servers)))
	for _, name := range servers {
		b.WriteString("  " + m.mcpStatus.ServerStatus(name) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
