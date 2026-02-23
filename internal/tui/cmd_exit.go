package tui

import tea "github.com/charmbracelet/bubbletea"

// registerExitCommands registers /quit and /exit.
func registerExitCommands(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "quit",
		Description: "Exit the program",
		Execute:     executeQuit,
	})

	r.register(SlashCommand{
		Name:        "exit",
		Description: "Exit the program",
		IsAlias:     true,
		Execute:     executeQuit,
	})
}

func executeQuit(m *model, args string) (tea.Model, tea.Cmd) {
	m.quitting = true
	return *m, tea.Quit
}
