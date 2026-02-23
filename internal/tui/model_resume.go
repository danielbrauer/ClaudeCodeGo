package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/session"
)

// handleResumeKey processes key events during the session picker.
func (m model) handleResumeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.resumeSessions) == 0 {
		m.mode = modeInput
		m.textInput.Focus()
		return m, textarea.Blink
	}

	switch msg.Type {
	case tea.KeyUp:
		if m.resumeCursor > 0 {
			m.resumeCursor--
		}
		return m, nil

	case tea.KeyDown:
		if m.resumeCursor < len(m.resumeSessions)-1 {
			m.resumeCursor++
		}
		return m, nil

	case tea.KeyEnter:
		sess := m.resumeSessions[m.resumeCursor]
		// Switch the current session to the selected one.
		m.session.ID = sess.ID
		m.session.Model = sess.Model
		m.session.CWD = sess.CWD
		m.session.Messages = sess.Messages
		m.session.CreatedAt = sess.CreatedAt
		m.session.UpdatedAt = sess.UpdatedAt

		// Replace the loop's history with the resumed session's messages.
		m.loop.History().SetMessages(sess.Messages)

		// Clear picker state.
		m.resumeSessions = nil
		m.resumeCursor = 0
		m.mode = modeInput
		m.textInput.Focus()

		summary := sessionSummary(sess)
		line := resumeHeaderStyle.Render("Resumed session ") +
			resumeIDStyle.Render(sess.ID) +
			resumeHeaderStyle.Render(" ("+summary+")")
		return m, tea.Batch(tea.Println(line), textarea.Blink)

	case tea.KeyEsc, tea.KeyCtrlC:
		m.resumeSessions = nil
		m.resumeCursor = 0
		m.mode = modeInput
		m.textInput.Focus()
		return m, textarea.Blink
	}

	return m, nil
}

// renderResumePicker renders the session selection list.
func (m model) renderResumePicker() string {
	var b strings.Builder
	b.WriteString(resumeHeaderStyle.Render("Select a session to resume:") + "\n")

	// Show at most 10 sessions.
	maxVisible := 10
	if len(m.resumeSessions) < maxVisible {
		maxVisible = len(m.resumeSessions)
	}

	// Calculate scroll window.
	start := 0
	if m.resumeCursor >= maxVisible {
		start = m.resumeCursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(m.resumeSessions) {
		end = len(m.resumeSessions)
		start = end - maxVisible
		if start < 0 {
			start = 0
		}
	}

	for i := start; i < end; i++ {
		sess := m.resumeSessions[i]
		timeStr := relativeTime(sess.UpdatedAt)
		msgCount := len(sess.Messages)
		firstMsg := firstUserMessage(sess)
		if len(firstMsg) > 60 {
			firstMsg = firstMsg[:57] + "..."
		}

		desc := timeStr + " | " + pluralize(msgCount, "message", "messages")
		if firstMsg != "" {
			desc += " | " + firstMsg
		}

		if i == m.resumeCursor {
			b.WriteString(askSelectedStyle.Render("  > "+desc) + "\n")
		} else {
			b.WriteString(askOptionStyle.Render("    "+desc) + "\n")
		}
	}

	if len(m.resumeSessions) > maxVisible {
		b.WriteString(permHintStyle.Render("  (showing " + pluralize(maxVisible, "session", "sessions") +
			" of " + pluralize(len(m.resumeSessions), "", "") + ")") + "\n")
	}

	b.WriteString(permHintStyle.Render("  Use arrow keys to navigate, Enter to select, Esc to cancel"))
	return b.String()
}

// relativeTime formats a time as a human-readable relative string.
func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("%dh ago", h)
	case d < 30*24*time.Hour:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	default:
		return t.Format("Jan 2")
	}
}

// firstUserMessage extracts the text of the first user message in a session.
func firstUserMessage(sess *session.Session) string {
	for _, msg := range sess.Messages {
		if msg.Role != api.RoleUser {
			continue
		}
		// Content can be a JSON string or []ContentBlock.
		var text string
		if err := json.Unmarshal(msg.Content, &text); err == nil {
			return strings.TrimSpace(text)
		}
		var blocks []api.ContentBlock
		if err := json.Unmarshal(msg.Content, &blocks); err == nil {
			for _, b := range blocks {
				if b.Type == api.ContentTypeText && b.Text != "" {
					return strings.TrimSpace(b.Text)
				}
			}
		}
		break
	}
	return ""
}

// sessionSummary returns a short summary string for a session.
func sessionSummary(sess *session.Session) string {
	parts := []string{
		relativeTime(sess.UpdatedAt),
		pluralize(len(sess.Messages), "message", "messages"),
	}
	return strings.Join(parts, ", ")
}

// pluralize returns "N item" or "N items" based on count.
func pluralize(n int, singular, plural string) string {
	if singular == "" && plural == "" {
		return fmt.Sprintf("%d", n)
	}
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}
