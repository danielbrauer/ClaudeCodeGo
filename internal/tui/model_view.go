package tui

import (
	"strings"
)

// View renders the live region of the TUI.
func (m model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Help screen (takes over the entire view).
	if m.mode == modeHelp {
		b.WriteString(m.renderHelpScreen())
		return b.String()
	}

	// Diff dialog (takes over the entire view).
	if m.mode == modeDiff && m.diffData != nil {
		b.WriteString(renderDiffView(m.diffData, m.diffSelected, m.diffViewMode, m.width))
		return b.String()
	}

	// Also show a loading indicator while diff is loading.
	if m.mode == modeDiff && m.diffData == nil {
		b.WriteString(m.spinner.View() + " Loading diff...\n")
		return b.String()
	}

	// Streaming text (during API response).
	if m.streamingText != "" {
		rendered := m.mdRenderer.render(m.streamingText)
		b.WriteString(rendered)
		b.WriteString("\n")
	}

	// Active tool spinner.
	if m.activeTool != "" {
		b.WriteString(m.spinner.View())
		b.WriteString(" ")
		b.WriteString(toolNameStyle.Render(m.activeTool))
		if m.toolSummary != "" {
			b.WriteString("  " + toolSummaryStyle.Render(m.toolSummary))
		}
		b.WriteString("\n")
	} else if m.mode == modeStreaming && m.streamingText == "" {
		// Show a general "thinking" spinner when waiting for the API.
		b.WriteString(m.spinner.View())
		b.WriteString(" Thinking...\n")
	}

	// Config panel.
	if m.mode == modeConfig && m.configPanel != nil {
		b.WriteString(m.renderConfigPanel())
		b.WriteString("\n")
		// Status bar.
		b.WriteString(renderStatusBar(m.modelName, &m.tokens, m.width, m.fastMode))
		return b.String()
	}

	// Permission prompt.
	if m.permissionPending != nil {
		b.WriteString(renderPermissionPrompt(m.permissionPending.ToolName, m.permissionPending.Summary, m.permissionPending.Suggestions))
		b.WriteString("\n")
	}

	// AskUser prompt.
	if m.askUserPending != nil && m.askQuestionIdx < len(m.askUserPending.Questions) {
		b.WriteString(m.renderAskUserPrompt())
		b.WriteString("\n")
	}

	// Resume session picker.
	if m.mode == modeResume && len(m.resumeSessions) > 0 {
		b.WriteString(m.renderResumePicker())
	}

	// Model picker.
	if m.mode == modeModelPicker {
		b.WriteString(m.renderModelPicker())
		b.WriteString("\n")
	}

	// Todo list.
	if len(m.todos) > 0 {
		b.WriteString(renderTodoList(m.todos))
		b.WriteString("\n")
	}

	// Completion suggestions (shown above the input).
	if m.mode == modeInput && len(m.completions) > 0 {
		b.WriteString(m.renderCompletions())
	}

	// Input area with borders.
	if m.mode == modeInput || (m.mode == modeStreaming && m.textInput.Value() != "") {
		b.WriteString(m.renderInputArea())
	}

	// Streaming hints: queue count and interrupt hint.
	if extra := m.streamingExtraHint(); extra != "" {
		b.WriteString(extra)
	}

	// Status line (custom command output) or default status bar.
	if m.statusLineText != "" {
		b.WriteString(statusBarStyle.Render(m.statusLineText))
	} else {
		b.WriteString(renderStatusBar(m.modelName, &m.tokens, m.width, m.fastMode))
	}

	return b.String()
}

// renderInputArea renders the input textarea with borders and hints.
func (m model) renderInputArea() string {
	var b strings.Builder

	// Top border.
	b.WriteString(renderInputBorder(m.width))
	b.WriteString("\n")

	if m.mode == modeInput {
		// Set placeholder dynamically: show a suggestion when the input
		// field is blank. Prioritize dynamic suggestions, then static
		// template suggestions before the first submit.
		if m.textInput.Value() == "" {
			if m.dynSuggestion != "" {
				m.textInput.Placeholder = m.dynSuggestion
			} else if m.submitCount < 1 {
				if m.queue.Len() > 0 {
					m.textInput.Placeholder = "Press Esc to remove queued messages"
				} else {
					m.textInput.Placeholder = m.promptSuggestion
				}
			} else {
				m.textInput.Placeholder = ""
			}
		} else {
			m.textInput.Placeholder = ""
		}
	} else {
		// Streaming mode — show a hint that input will be queued.
		m.textInput.Placeholder = ""
	}

	b.WriteString(m.textInput.View())
	b.WriteString("\n")

	// Bottom border.
	b.WriteString(renderInputBorder(m.width))
	b.WriteString("\n")

	// Hints line below the input area (centralised in hints.go).
	if hint := m.inputAreaHint(); hint != "" {
		b.WriteString("  " + shortcutsHintStyle.Render(hint))
		b.WriteString("\n")
	}

	return b.String()
}
