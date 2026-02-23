package tui

import tea "github.com/charmbracelet/bubbletea"

// registerAuthCommands registers /login and /logout.
func registerAuthCommands(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "login",
		Description: "Sign in to your Anthropic account",
		Execute:     executeLogin,
	})

	r.register(SlashCommand{
		Name:        "logout",
		Description: "Log out from your Anthropic account",
		Execute:     executeLogout,
	})
}

func executeLogin(m *model, args string) (tea.Model, tea.Cmd) {
	m.exitAction = ExitLogin
	m.quitting = true
	return *m, tea.Batch(tea.Println("Exiting session for re-authentication..."), tea.Quit)
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
