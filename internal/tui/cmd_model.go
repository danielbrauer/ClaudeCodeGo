package tui

import tea "github.com/charmbracelet/bubbletea"

// registerModelCommand registers /model.
func registerModelCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "model",
		Description: "Show or switch model",
		Execute:     executeModel,
	})
}

func executeModel(m *model, args string) (tea.Model, tea.Cmd) {
	parts := []string{"model"}
	if args != "" {
		parts = append(parts, args)
	}
	return m.handleModelCommand(parts)
}
