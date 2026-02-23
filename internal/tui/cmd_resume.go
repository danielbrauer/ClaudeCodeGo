package tui

import tea "github.com/charmbracelet/bubbletea"

// registerResumeCommand registers /resume.
func registerResumeCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "resume",
		Description: "Resume a previous session",
		Execute:     executeResume,
	})
}

func executeResume(m *model, args string) (tea.Model, tea.Cmd) {
	if m.sessStore == nil {
		return *m, tea.Println(errorStyle.Render("Session store not available."))
	}
	sessions, err := m.sessStore.List()
	if err != nil || len(sessions) == 0 {
		return *m, tea.Println(errorStyle.Render("No sessions found."))
	}
	m.resumeSessions = sessions
	m.resumeCursor = 0
	m.mode = modeResume
	m.textInput.Blur()
	return *m, nil
}
