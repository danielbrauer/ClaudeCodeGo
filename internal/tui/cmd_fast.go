package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/config"
)

// registerFastCommand registers /fast.
func registerFastCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "fast",
		Description: "Toggle fast mode (" + api.FastModeDisplayName + " only)",
		Execute:     executeFast,
	})
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
