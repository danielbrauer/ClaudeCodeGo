package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// registerLoopCommands registers /compact, /init, /review.
func registerLoopCommands(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "compact",
		Description: "Compact conversation history",
		Execute:     executeCompact,
	})

	r.register(SlashCommand{
		Name:        "init",
		Description: "Initialize a new CLAUDE.md file with codebase documentation",
		Execute:     executeInit,
	})

	r.register(SlashCommand{
		Name:        "review",
		Description: "Review a pull request",
		Execute:     executeReview,
	})
}

func executeCompact(m *model, args string) (tea.Model, tea.Cmd) {
	m.mode = modeStreaming
	return *m, func() tea.Msg {
		err := m.loop.Compact(m.ctx)
		if err != nil {
			return LoopDoneMsg{Err: err}
		}
		return LoopDoneMsg{}
	}
}

func executeInit(m *model, args string) (tea.Model, tea.Cmd) {
	return sendToLoop(m, initPrompt)
}

func executeReview(m *model, args string) (tea.Model, tea.Cmd) {
	arg := strings.TrimSpace(args)
	return sendToLoop(m, buildReviewPrompt(arg))
}
