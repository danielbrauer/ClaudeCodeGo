package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// isExitCommand returns true if the input is a bare exit command.
// The JS CLI recognizes these without a slash prefix.
func isExitCommand(text string) bool {
	switch text {
	case "exit", "quit", ":q", ":q!", ":wq", ":wq!":
		return true
	}
	return false
}

// handleSubmit processes submitted text (user message or slash command).
func (m model) handleSubmit(text string) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	m.submitCount++

	// Echo user input to scrollback.
	userLine := userLabelStyle.Render("> ") + text
	cmds = append(cmds, tea.Println(userLine))

	// Check for bare exit commands (exit, quit, :q, :q!, :wq, :wq!).
	if isExitCommand(text) {
		m.quitting = true
		return m, tea.Batch(append(cmds, tea.Quit)...)
	}

	// Check for slash commands.
	if strings.HasPrefix(text, "/") {
		cmdName := strings.TrimPrefix(text, "/")
		parts := strings.SplitN(cmdName, " ", 2)
		cmdName = parts[0]
		cmdArgs := ""
		if len(parts) > 1 {
			cmdArgs = parts[1]
		}

		// Fuzzy auto-correct: if the command isn't an exact match, try to
		// find the best fuzzy match and silently correct it.
		if _, exact := m.slashReg.lookup(cmdName); !exact {
			if best, ok := m.slashReg.fuzzyBest(cmdName); ok {
				hint := permHintStyle.Render(fmt.Sprintf("  (corrected /%s → /%s)", cmdName, best))
				cmds = append(cmds, tea.Println(hint))
				cmdName = best
			}
		}

		if cmd, ok := m.slashReg.lookup(cmdName); ok && cmd.Execute != nil {
			result, cmdCmd := cmd.Execute(&m, cmdArgs)
			if cmdCmd != nil {
				cmds = append(cmds, cmdCmd)
			}
			return result, tea.Batch(cmds...)
		}

		errMsg := "Unknown command: /" + cmdName + " (type /help for available commands)"
		cmds = append(cmds, tea.Println(errMsg))
		return m, tea.Batch(cmds...)
	}

	// Regular message: send to the agentic loop.
	m.mode = modeStreaming

	loopCmd := func() tea.Msg {
		err := m.loop.SendMessage(m.ctx, text)
		return LoopDoneMsg{Err: err}
	}

	cmds = append(cmds, loopCmd, m.spinner.Tick)
	return m, tea.Batch(cmds...)
}
