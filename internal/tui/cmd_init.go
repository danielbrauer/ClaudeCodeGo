package tui

import tea "github.com/charmbracelet/bubbletea"

// registerInitCommand registers /init.
func registerInitCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "init",
		Description: "Initialize a new CLAUDE.md file with codebase documentation",
		Execute:     executeInit,
	})
}

func executeInit(m *model, args string) (tea.Model, tea.Cmd) {
	return sendToLoop(m, initPrompt)
}
