package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/anthropics/claude-code-go/internal/tools"
)

// renderTodoList produces the styled todo list for the live region.
//
// Each line is styled as a single unit (one lipgloss.Render call) rather than
// styling the icon and text separately. This produces one set of ANSI escape
// sequences per line instead of two, which prevents Bubble Tea's inline
// renderer from miscalculating physical line widths during repaints.
func renderTodoList(todos []tools.TodoItem) string {
	if len(todos) == 0 {
		return ""
	}

	lines := make([]string, 0, len(todos))
	for _, item := range todos {
		var icon, text string
		var style lipgloss.Style
		switch item.Status {
		case "in_progress":
			icon = "[~]"
			text = item.ActiveForm
			style = todoInProgressStyle
		case "completed":
			icon = "[x]"
			text = item.Content
			style = todoCompletedStyle
		default: // pending
			icon = "[ ]"
			text = item.Content
			style = todoPendingStyle
		}
		lines = append(lines, style.Render("  "+icon+" "+text))
	}
	return strings.Join(lines, "\n")
}
