package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/anthropics/claude-code-go/internal/conversation"
	"github.com/anthropics/claude-code-go/internal/session"
)

// registerClearCommand registers /clear and its aliases /reset, /new.
func registerClearCommand(r *slashRegistry) {
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
