package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// handleTabComplete triggers or cycles forward through fuzzy slash command completions.
func (m model) handleTabComplete() (tea.Model, tea.Cmd) {
	text := m.textInput.Value()

	// Only complete when the input starts with "/".
	if !strings.HasPrefix(text, "/") {
		return m, nil
	}

	// Extract the command portion (no leading slash, no args).
	raw := strings.TrimPrefix(text, "/")
	parts := strings.SplitN(raw, " ", 2)
	typed := parts[0]

	// If we already have completions, cycle to the next one.
	if len(m.completions) > 0 && m.completionBase == typed || len(m.completions) > 0 {
		m.completionIdx = (m.completionIdx + 1) % len(m.completions)
		m.applyCompletion()
		return m, nil
	}

	// Build new completions.
	matches := m.slashReg.fuzzyComplete(typed)
	if len(matches) == 0 {
		return m, nil
	}

	m.completionBase = typed
	m.completions = matches
	m.completionIdx = 0
	m.applyCompletion()
	return m, nil
}

// handleTabCompletePrev cycles backward through completions (Shift+Tab).
func (m model) handleTabCompletePrev() (tea.Model, tea.Cmd) {
	if len(m.completions) == 0 {
		// Start fresh same as Tab.
		return m.handleTabComplete()
	}
	m.completionIdx--
	if m.completionIdx < 0 {
		m.completionIdx = len(m.completions) - 1
	}
	m.applyCompletion()
	return m, nil
}

// applyCompletion replaces the text input content with the selected completion.
func (m *model) applyCompletion() {
	if m.completionIdx < 0 || m.completionIdx >= len(m.completions) {
		return
	}
	completed := "/" + m.completions[m.completionIdx]
	m.textInput.Reset()
	m.textInput.SetValue(completed)
	// Move cursor to end.
	m.textInput.CursorEnd()
}

// clearCompletions resets completion state.
func (m *model) clearCompletions() {
	m.completions = nil
	m.completionIdx = -1
	m.completionBase = ""
}

// renderCompletions renders the inline completion suggestions.
func (m model) renderCompletions() string {
	var b strings.Builder
	maxShow := 8
	if len(m.completions) < maxShow {
		maxShow = len(m.completions)
	}

	for i := 0; i < maxShow; i++ {
		name := m.completions[i]
		desc := ""
		if cmd, ok := m.slashReg.lookup(name); ok {
			desc = cmd.Description
		}
		if i == m.completionIdx {
			b.WriteString(askSelectedStyle.Render("  > /"+name) + " " + permHintStyle.Render(desc) + "\n")
		} else {
			b.WriteString(permHintStyle.Render("    /"+name+" "+desc) + "\n")
		}
	}

	if len(m.completions) > maxShow {
		b.WriteString(permHintStyle.Render(fmt.Sprintf("    ... and %d more", len(m.completions)-maxShow)) + "\n")
	}

	return b.String()
}
