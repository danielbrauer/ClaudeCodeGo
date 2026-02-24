package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/anthropics/claude-code-go/internal/config"
)

// handleKey processes keyboard input based on the current mode.
func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {

	case modeHelp:
		return m.handleHelpKey(msg)

	case modeResume:
		return m.handleResumeKey(msg)

	case modeConfig:
		return m.handleConfigKey(msg)

	case modePermission:
		return m.handlePermissionKey(msg)

	case modeAskUser:
		return m.handleAskUserKey(msg)

	case modeModelPicker:
		return m.handleModelPickerKey(msg)

	case modeDiff:
		return m.handleDiffKey(msg)

	case modeStreaming:
		return m.handleStreamingKey(msg)

	case modeInput:
		return m.handleInputKey(msg)
	}

	return m, nil
}

// handleInputKey processes key events while in input mode.
func (m model) handleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		// Double-press detection: first Ctrl-C clears input and shows a
		// hint; second press within 800 ms actually exits.
		if m.ctrlCPending {
			m.quitting = true
			return m, tea.Quit
		}
		m.textInput.Reset()
		m.ctrlCPending = true
		return m, startCtrlCTimer()

	case tea.KeyTab:
		// If the input is empty and we have a dynamic suggestion,
		// accept it by filling it into the text input.
		if strings.TrimSpace(m.textInput.Value()) == "" && m.dynSuggestion != "" {
			m.textInput.SetValue(m.dynSuggestion)
			m.textInput.CursorEnd()
			return m, nil
		}
		return m.handleTabComplete()

	case tea.KeyShiftTab:
		// When completions are active, cycle backward through them.
		if len(m.completions) > 0 {
			return m.handleTabCompletePrev()
		}
		// Otherwise, cycle permission mode (matches JS Shift+Tab behavior).
		newMode := m.cyclePermissionMode()
		info := config.PermissionModeMetadata[newMode]
		modeLabel := info.Title
		if info.Symbol != "" {
			modeLabel = info.Symbol + " " + modeLabel
		}
		return m, tea.Println(permHintStyle.Render("Permission mode: " + modeLabel))

	case tea.KeyEscape:
		if len(m.completions) > 0 {
			m.clearCompletions()
			return m, nil
		}
		// Escape clears the dynamic suggestion.
		if m.dynSuggestion != "" {
			m.dynSuggestion = ""
			return m, nil
		}
		return m, nil

	case tea.KeyEnter:
		m.clearCompletions()
		text := strings.TrimSpace(m.textInput.Value())
		// If input is empty but we have a dynamic suggestion,
		// submit the suggestion directly.
		if text == "" && m.dynSuggestion != "" {
			text = m.dynSuggestion
			m.dynSuggestion = ""
			m.textInput.Reset()
			return m.handleSubmit(text)
		}
		if text == "" {
			return m, nil
		}
		m.dynSuggestion = "" // clear suggestion on any submit
		m.textInput.Reset()
		return m.handleSubmit(text)

	default:
		// Open help screen when '?' is pressed with empty input.
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == '?' {
			if strings.TrimSpace(m.textInput.Value()) == "" {
				m.helpTab = 0
				m.helpScrollOff = 0
				m.mode = modeHelp
				m.textInput.Blur()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		// Clear completions when the user types — they'll re-trigger on Tab.
		if len(m.completions) > 0 {
			m.clearCompletions()
		}
		// Clear the dynamic suggestion once the user starts typing
		// their own text.
		if m.dynSuggestion != "" && m.textInput.Value() != "" {
			m.dynSuggestion = ""
		}
		return m, cmd
	}
}

// handleStreamingKey processes key events while the agent is working.
// Users can type ahead and press Enter to queue messages for when the
// current turn finishes.
func (m model) handleStreamingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		// Ctrl+C cancels the running loop and clears the queue.
		m.queue.Clear()
		m.cancelFn()
		return m, nil

	case tea.KeyEnter:
		text := strings.TrimSpace(m.textInput.Value())
		if text == "" {
			return m, nil
		}
		m.textInput.Reset()

		// Enqueue the message for processing after the current turn.
		m.queue.Enqueue(text)

		// Echo queued message to scrollback with a "queued" indicator.
		userLine := queuedLabelStyle.Render("> ") + permHintStyle.Render(text) +
			"  " + queuedBadgeStyle.Render("(queued)")
		return m, tea.Println(userLine)

	case tea.KeyEscape:
		// Escape clears the current input being typed during streaming,
		// or removes the last queued message if input is empty.
		if strings.TrimSpace(m.textInput.Value()) != "" {
			m.textInput.Reset()
			return m, nil
		}
		if text, ok := m.queue.RemoveLast(); ok {
			hint := permHintStyle.Render("Removed queued message: " + truncateText(text, 60))
			return m, tea.Println(hint)
		}
		return m, nil

	default:
		// All other keys are forwarded to the textarea by the Update fallthrough.
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}
}
