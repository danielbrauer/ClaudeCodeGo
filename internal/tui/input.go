package tui

import (
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
)

// newTextInput creates and configures the multi-line text input editor.
func newTextInput(width int) textarea.Model {
	ti := textarea.New()
	ti.Placeholder = "Type a message (Enter to send, Shift+Enter for newline)"
	ti.Prompt = promptStyle.Render("> ")
	ti.CharLimit = 0 // no limit
	ti.SetWidth(width)
	ti.SetHeight(1)
	ti.ShowLineNumbers = false
	ti.FocusedStyle.CursorLine = lipgloss.NewStyle() // no cursor line highlight
	ti.Focus()
	return ti
}
