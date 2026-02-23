package tui

import (
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// handleDiffKey processes key events in the diff dialog.
func (m model) handleDiffKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.diffData == nil {
		m.mode = modeInput
		m.textInput.Focus()
		return m, textarea.Blink
	}

	switch msg.Type {
	case tea.KeyEscape, tea.KeyCtrlC:
		// Close diff dialog.
		m.diffData = nil
		m.mode = modeInput
		m.textInput.Focus()
		return m, textarea.Blink

	case tea.KeyUp:
		if m.diffViewMode == "list" && m.diffSelected > 0 {
			m.diffSelected--
		}
		return m, nil

	case tea.KeyDown:
		if m.diffViewMode == "list" && m.diffSelected < len(m.diffData.files)-1 {
			m.diffSelected++
		}
		return m, nil

	case tea.KeyEnter:
		if m.diffViewMode == "list" && m.diffSelected < len(m.diffData.files) {
			m.diffViewMode = "detail"
		}
		return m, nil

	case tea.KeyLeft:
		if m.diffViewMode == "detail" {
			m.diffViewMode = "list"
		}
		return m, nil

	default:
		// Also handle 'q' to close.
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == 'q' {
			m.diffData = nil
			m.mode = modeInput
			m.textInput.Focus()
			return m, textarea.Blink
		}
		return m, nil
	}
}
