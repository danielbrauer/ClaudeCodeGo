package tui

import (
	"strings"

	"github.com/anthropics/claude-code-go/internal/tools"
)

// renderTodoList produces the styled todo list for the live region.
func renderTodoList(todos []tools.TodoItem) string {
	if len(todos) == 0 {
		return ""
	}

	var b strings.Builder
	for _, item := range todos {
		var icon, line string
		switch item.Status {
		case "in_progress":
			icon = todoInProgressStyle.Render("[~]")
			line = todoInProgressStyle.Render(item.ActiveForm)
		case "completed":
			icon = todoCompletedStyle.Render("[x]")
			line = todoCompletedStyle.Render(item.Content)
		default: // pending
			icon = todoPendingStyle.Render("[ ]")
			line = todoPendingStyle.Render(item.Content)
		}
		b.WriteString("  " + icon + " " + line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
