package tui

import (
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// registerContinueCommand registers /continue.
func registerContinueCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "continue",
		Description: "Resume the most recent session",
		Execute:     executeContinue,
	})
}

func executeContinue(m *model, args string) (tea.Model, tea.Cmd) {
	if m.sessStore == nil {
		return *m, tea.Println(errorStyle.Render("Session store not available."))
	}
	sess, err := m.sessStore.MostRecent()
	if err != nil {
		return *m, tea.Println(errorStyle.Render("No previous session found."))
	}
	// Switch to the most recent session directly.
	m.session.ID = sess.ID
	m.session.Model = sess.Model
	m.session.CWD = sess.CWD
	m.session.Messages = sess.Messages
	m.session.CreatedAt = sess.CreatedAt
	m.session.UpdatedAt = sess.UpdatedAt
	m.loop.History().SetMessages(sess.Messages)

	summary := sessionSummary(sess)
	line := resumeHeaderStyle.Render("Resumed session ") +
		resumeIDStyle.Render(sess.ID) +
		resumeHeaderStyle.Render(" ("+summary+")")
	return *m, tea.Batch(tea.Println(line), textarea.Blink)
}
