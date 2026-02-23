package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ctrlCTimeout is the window within which a second Ctrl-C press triggers exit.
// Matches the official JS CLI's 800 ms timeout (cli.js rD7 constant).
const ctrlCTimeout = 800 * time.Millisecond

// ctrlCResetMsg is sent after ctrlCTimeout elapses to clear the pending state.
type ctrlCResetMsg struct{}

// startCtrlCTimer returns a command that sends ctrlCResetMsg after the timeout.
func startCtrlCTimer() tea.Cmd {
	return tea.Tick(ctrlCTimeout, func(time.Time) tea.Msg {
		return ctrlCResetMsg{}
	})
}

// inputAreaHint returns the hint text to display below the input area.
// All input-area hint logic is centralised here so that callers in the view
// layer don't need to reason about priority themselves.
//
// Priority (highest first):
//  1. Ctrl-C pending → "Press Ctrl-C again to exit"
//  2. Mode-specific hints (input / streaming)
func (m model) inputAreaHint() string {
	if m.ctrlCPending {
		return "Press Ctrl-C again to exit"
	}

	switch m.mode {
	case modeInput:
		if len(m.completions) > 0 {
			return ""
		}
		if m.dynSuggestion != "" && m.textInput.Value() == "" {
			return "enter to send, tab to edit, esc to dismiss"
		}
		if strings.TrimSpace(m.textInput.Value()) == "" {
			return "? for shortcuts"
		}

	case modeStreaming:
		hint := "Enter to queue message"
		if m.queue.Len() > 0 {
			hint += fmt.Sprintf(" · %d queued", m.queue.Len())
		}
		return hint
	}

	return ""
}

// streamingExtraHint returns text shown below the input area during streaming
// (queue badge when input is empty). Returns empty when not streaming or when
// there is nothing extra to show. The main interrupt/queue hint is already
// rendered by renderInputArea, so this only adds supplementary info.
func (m model) streamingExtraHint() string {
	if m.mode != modeStreaming {
		return ""
	}
	if m.textInput.Value() == "" && m.queue.Len() > 0 {
		return queuedBadgeStyle.Render(fmt.Sprintf("  %d message%s queued",
			m.queue.Len(), pluralS(m.queue.Len()))) + "\n"
	}
	return ""
}
