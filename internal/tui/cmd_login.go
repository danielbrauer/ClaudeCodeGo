package tui

import tea "github.com/charmbracelet/bubbletea"

// registerLoginCommand registers /login.
func registerLoginCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "login",
		Description: "Sign in to your Anthropic account",
		Execute:     executeLogin,
	})
}

func executeLogin(m *model, args string) (tea.Model, tea.Cmd) {
	m.exitAction = ExitLogin
	m.quitting = true
	return *m, tea.Batch(tea.Println("Exiting session for re-authentication..."), tea.Quit)
}
