package tui

import tea "github.com/charmbracelet/bubbletea"

// registerHelpCommand registers /help.
func registerHelpCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "help",
		Description: "Show help and available commands",
		Execute:     executeHelp,
	})
}

func executeHelp(m *model, args string) (tea.Model, tea.Cmd) {
	m.helpTab = 0
	m.helpScrollOff = 0
	m.mode = modeHelp
	m.textInput.Blur()
	return *m, nil
}
