package tui

import (
	"context"
	"encoding/json"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// TUIPermissionHandler implements tools.PermissionHandler by sending
// permission requests to the Bubble Tea event loop and waiting for
// the user's response via a channel.
type TUIPermissionHandler struct {
	program *tea.Program
}

// NewTUIPermissionHandler creates a permission handler wired to the given program.
func NewTUIPermissionHandler(p *tea.Program) *TUIPermissionHandler {
	return &TUIPermissionHandler{program: p}
}

// RequestPermission sends a permission prompt to the TUI and blocks until
// the user responds. This is called from the agentic loop's goroutine.
func (h *TUIPermissionHandler) RequestPermission(
	ctx context.Context, toolName string, input json.RawMessage,
) (bool, error) {
	if h.program == nil {
		return false, nil
	}

	resultCh := make(chan bool, 1)
	h.program.Send(PermissionRequestMsg{
		ToolName: toolName,
		Input:    input,
		Summary:  summarizeForPermission(toolName, input),
		ResultCh: resultCh,
	})

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case allowed := <-resultCh:
		return allowed, nil
	}
}

// summarizeForPermission produces a short description for the permission prompt.
func summarizeForPermission(toolName string, input json.RawMessage) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(input, &m); err != nil {
		return ""
	}
	getString := func(key string) string {
		v, ok := m[key]
		if !ok {
			return ""
		}
		var s string
		json.Unmarshal(v, &s)
		return s
	}

	switch toolName {
	case "Bash":
		if s := getString("command"); s != "" {
			if len(s) > 120 {
				s = s[:117] + "..."
			}
			return fmt.Sprintf("$ %s", s)
		}
	case "FileWrite":
		if s := getString("file_path"); s != "" {
			return fmt.Sprintf("Write to: %s", s)
		}
	case "FileEdit":
		if s := getString("file_path"); s != "" {
			return fmt.Sprintf("Edit: %s", s)
		}
	}
	return ""
}

// renderPermissionPrompt produces the permission prompt text for the live region.
func renderPermissionPrompt(toolName, summary string) string {
	title := permTitleStyle.Render("Permission Required")
	tool := "  Tool: " + permToolStyle.Render(toolName)
	hint := permHintStyle.Render("  Press y to allow, n to deny")

	result := title + "\n" + tool
	if summary != "" {
		result += "\n  " + toolSummaryStyle.Render(summary)
	}
	result += "\n" + hint
	return result
}

// renderPermissionResult produces the scrollback line for a resolved permission.
func renderPermissionResultLine(toolName, summary string, allowed bool) string {
	verdict := diffAddStyle.Render("allowed")
	if !allowed {
		verdict = diffRemoveStyle.Render("denied")
	}
	line := toolBulletStyle.Render("  ") + permToolStyle.Render(toolName)
	if summary != "" {
		line += "  " + toolSummaryStyle.Render(summary)
	}
	line += "  " + verdict
	return line
}
