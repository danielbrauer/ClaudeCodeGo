package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textarea"

	"github.com/anthropics/claude-code-go/internal/conversation"
	"github.com/anthropics/claude-code-go/internal/session"
)

// registerSessionCommands registers /clear, /reset, /new, /resume, /continue.
func registerSessionCommands(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "clear",
		Description: "Clear conversation history and free up context",
		Execute:     executeClear,
	})

	r.register(SlashCommand{
		Name:        "reset",
		Description: "Clear conversation history and free up context",
		IsAlias:     true,
		Execute:     executeClear,
	})

	r.register(SlashCommand{
		Name:        "new",
		Description: "Clear conversation history and free up context",
		IsAlias:     true,
		Execute:     executeClear,
	})

	r.register(SlashCommand{
		Name:        "resume",
		Description: "Resume a previous session",
		Execute:     executeResume,
	})

	r.register(SlashCommand{
		Name:        "continue",
		Description: "Resume the most recent session",
		Execute:     executeContinue,
	})
}

func executeClear(m *model, args string) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Clear conversation history.
	m.loop.Clear()

	// Reset token tracking.
	m.tokens = tokenTracker{}

	// Clear todo list.
	m.todos = nil

	// Create a new session, preserving the model and CWD.
	if m.session != nil {
		m.session = &session.Session{
			ID:    session.GenerateID(),
			Model: m.session.Model,
			CWD:   m.session.CWD,
		}

		// Update the turn-complete callback to reference the new session.
		newSess := m.session
		store := m.sessStore
		m.loop.SetOnTurnComplete(func(h *conversation.History) {
			if store != nil && newSess != nil {
				newSess.Messages = h.Messages()
				_ = store.Save(newSess)
			}
		})

		if m.sessStore != nil {
			if err := m.sessStore.Save(m.session); err != nil {
				errLine := errorStyle.Render("Warning: failed to save new session: " + err.Error())
				cmds = append(cmds, tea.Println(errLine))
			}
		}
	}

	cmds = append(cmds, tea.Println("Conversation cleared. Starting fresh."))
	return *m, tea.Batch(cmds...)
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
