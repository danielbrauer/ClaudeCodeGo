package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// handleAskUserKey processes key events during an ask-user prompt.
func (m model) handleAskUserKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.askUserPending == nil || len(m.askUserPending.Questions) == 0 {
		m.mode = modeStreaming
		return m, nil
	}

	q := m.askUserPending.Questions[m.askQuestionIdx]
	numOptions := len(q.Options) + 1 // +1 for "Other"

	if m.askCustomInput {
		// Typing custom text for "Other" option.
		switch msg.Type {
		case tea.KeyEnter:
			m.askAnswers[q.Question] = m.askCustomText
			m.askCustomInput = false
			m.askCustomText = ""
			return m.advanceAskUser()
		case tea.KeyBackspace:
			if len(m.askCustomText) > 0 {
				m.askCustomText = m.askCustomText[:len(m.askCustomText)-1]
			}
			return m, nil
		case tea.KeyCtrlC:
			m.askUserPending.ResponseCh <- m.askAnswers
			m.askUserPending = nil
			m.mode = modeStreaming
			return m, nil
		default:
			if msg.Type == tea.KeyRunes {
				m.askCustomText += string(msg.Runes)
			} else if msg.Type == tea.KeySpace {
				m.askCustomText += " "
			}
			return m, nil
		}
	}

	switch msg.Type {
	case tea.KeyUp:
		if m.askCursor > 0 {
			m.askCursor--
		}
		return m, nil

	case tea.KeyDown:
		if m.askCursor < numOptions-1 {
			m.askCursor++
		}
		return m, nil

	case tea.KeyEnter:
		if m.askCursor == numOptions-1 {
			// "Other" selected: switch to custom text input.
			m.askCustomInput = true
			m.askCustomText = ""
			return m, nil
		}
		// Regular option selected.
		m.askAnswers[q.Question] = q.Options[m.askCursor].Label
		return m.advanceAskUser()

	case tea.KeyCtrlC:
		// Cancel: send whatever answers we have.
		m.askUserPending.ResponseCh <- m.askAnswers
		m.askUserPending = nil
		m.mode = modeStreaming
		return m, nil
	}

	return m, nil
}

// advanceAskUser moves to the next question or completes the ask-user flow.
func (m model) advanceAskUser() (tea.Model, tea.Cmd) {
	m.askQuestionIdx++
	m.askCursor = 0

	if m.askQuestionIdx >= len(m.askUserPending.Questions) {
		// All questions answered. Print summary to scrollback.
		var lines []string
		for _, q := range m.askUserPending.Questions {
			answer := m.askAnswers[q.Question]
			lines = append(lines, askHeaderStyle.Render("["+q.Header+"]")+" "+q.Question+" "+askSelectedStyle.Render(answer))
		}
		m.askUserPending.ResponseCh <- m.askAnswers
		m.askUserPending = nil
		m.mode = modeStreaming

		var cmds []tea.Cmd
		for _, line := range lines {
			cmds = append(cmds, tea.Println(line))
		}
		return m, tea.Batch(cmds...)
	}

	return m, nil
}

// renderAskUserPrompt renders the current ask-user question.
func (m model) renderAskUserPrompt() string {
	if m.askUserPending == nil || m.askQuestionIdx >= len(m.askUserPending.Questions) {
		return ""
	}

	q := m.askUserPending.Questions[m.askQuestionIdx]
	var b strings.Builder

	b.WriteString(askHeaderStyle.Render("["+q.Header+"]") + " " + askQuestionStyle.Render(q.Question) + "\n")

	for i, opt := range q.Options {
		prefix := "  "
		if i == m.askCursor && !m.askCustomInput {
			b.WriteString(askSelectedStyle.Render(prefix+"> "+opt.Label) + " " + askOptionStyle.Render(opt.Description) + "\n")
		} else {
			b.WriteString(askOptionStyle.Render(prefix+"  "+opt.Label+" "+opt.Description) + "\n")
		}
	}

	// "Other" option.
	otherIdx := len(q.Options)
	if m.askCursor == otherIdx && !m.askCustomInput {
		b.WriteString(askSelectedStyle.Render("  > Other (custom input)") + "\n")
	} else if m.askCustomInput {
		b.WriteString(askSelectedStyle.Render("  > Other: "+m.askCustomText+"_") + "\n")
	} else {
		b.WriteString(askOptionStyle.Render("    Other (custom input)") + "\n")
	}

	b.WriteString(permHintStyle.Render("  Use arrow keys to navigate, Enter to select"))

	return b.String()
}
