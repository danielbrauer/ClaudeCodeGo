package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// registerReviewCommand registers /review.
func registerReviewCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "review",
		Description: "Review a pull request",
		Execute:     executeReview,
	})
}

func executeReview(m *model, args string) (tea.Model, tea.Cmd) {
	arg := strings.TrimSpace(args)
	return sendToLoop(m, buildReviewPrompt(arg))
}
