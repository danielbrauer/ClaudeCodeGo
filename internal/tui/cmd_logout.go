package tui

import tea "github.com/charmbracelet/bubbletea"

// registerLogoutCommand registers /logout.
func registerLogoutCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "logout",
		Description: "Log out from your Anthropic account",
		Execute:     executeLogout,
	})
}

func executeLogout(m *model, args string) (tea.Model, tea.Cmd) {
	if m.logoutFunc != nil {
		if err := m.logoutFunc(); err != nil {
			return *m, tea.Println(errorStyle.Render("Failed to log out."))
		}
	}
	m.quitting = true
	return *m, tea.Batch(
		tea.Println("Successfully logged out from your Anthropic account."),
		tea.Quit,
	)
}
