package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/config"
)

// registerInfoCommands registers /version, /cost, /context, /mcp, /fast.
func registerInfoCommands(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "version",
		Description: "Show version",
		Execute:     textCommand(versionText),
	})

	r.register(SlashCommand{
		Name:        "cost",
		Description: "Show token usage and cost",
		Execute:     textCommand(costText),
	})

	r.register(SlashCommand{
		Name:        "context",
		Description: "Show context window usage",
		Execute:     textCommand(contextText),
	})

	r.register(SlashCommand{
		Name:        "mcp",
		Description: "Show MCP server status",
		Execute:     textCommand(mcpText),
	})

	r.register(SlashCommand{
		Name:        "fast",
		Description: "Toggle fast mode (" + api.FastModeDisplayName + " only)",
		Execute:     executeFast,
	})
}

// Named text-producing functions — these are the underlying logic for text
// commands. They are callable directly for testing.

func versionText(m *model) string {
	return fmt.Sprintf("claude %s (Go)", m.version)
}

func costText(m *model) string {
	return renderCostSummary(&m.tokens)
}

func contextText(m *model) string {
	return fmt.Sprintf("Messages in history: %d", m.loop.History().Len())
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

func executeFast(m *model, args string) (tea.Model, tea.Cmd) {
	applyFastMode(m, !m.fastMode)

	// Persist to user settings.
	_ = config.SaveUserSetting("fastMode", m.fastMode)

	if m.fastMode {
		return *m, tea.Println("Fast mode ON")
	}
	return *m, tea.Println("Fast mode OFF")
}
