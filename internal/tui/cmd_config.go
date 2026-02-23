package tui

import tea "github.com/charmbracelet/bubbletea"

// registerConfigCommand registers /config and its alias /settings.
func registerConfigCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "config",
		Description: "Open config panel",
		Execute:     executeConfig,
	})

	r.register(SlashCommand{
		Name:        "settings",
		Description: "Open config panel",
		IsAlias:     true,
		Execute:     executeConfig,
	})
}

func executeConfig(m *model, args string) (tea.Model, tea.Cmd) {
	if m.settings != nil {
		m.configPanel = newConfigPanel(m.settings)
		m.mode = modeConfig
		m.textInput.Blur()
		return *m, nil
	}
	return *m, tea.Println(errorStyle.Render("No settings loaded."))
}
