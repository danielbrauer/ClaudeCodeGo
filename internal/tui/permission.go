package tui

import (
	"context"
	"encoding/json"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/anthropics/claude-code-go/internal/config"
)

// PermissionResponse represents the user's response to a permission prompt.
type PermissionResponse int

const (
	PermissionAllow       PermissionResponse = iota // allow this once
	PermissionDeny                                  // deny this once
	PermissionAlwaysAllow                           // allow and add session rule
)

// TUIPermissionHandler implements tools.PermissionHandler and
// tools.RichPermissionHandler by first checking rules via a wrapped
// RuleBasedPermissionHandler, then sending interactive permission requests
// to the Bubble Tea event loop for "ask" decisions.
type TUIPermissionHandler struct {
	program  *tea.Program
	ruleHandler *config.RuleBasedPermissionHandler
}

// NewTUIPermissionHandler creates a permission handler wired to the given
// program. If ruleHandler is non-nil, rules are checked before prompting.
func NewTUIPermissionHandler(p *tea.Program, ruleHandler *config.RuleBasedPermissionHandler) *TUIPermissionHandler {
	return &TUIPermissionHandler{
		program:     p,
		ruleHandler: ruleHandler,
	}
}

// CheckPermission evaluates permission rules first, then falls back to
// interactive TUI prompt. This implements tools.RichPermissionHandler.
func (h *TUIPermissionHandler) CheckPermission(toolName string, input json.RawMessage) config.PermissionResult {
	if h.ruleHandler != nil {
		result := h.ruleHandler.CheckPermission(toolName, input)
		if result.Behavior == config.BehaviorAllow || result.Behavior == config.BehaviorDeny {
			return result
		}
		// For "ask" or "passthrough", return the result with suggestions
		// so the TUI can display them.
		return result
	}
	// No rule handler — everything needs asking.
	return config.PermissionResult{
		Behavior: config.BehaviorAsk,
		Message:  "This operation requires approval",
	}
}

// GetPermissionContext returns the session-level permission context from
// the wrapped rule handler. This implements tools.PermissionContextProvider.
func (h *TUIPermissionHandler) GetPermissionContext() *config.ToolPermissionContext {
	if h.ruleHandler != nil {
		return h.ruleHandler.GetPermissionContext()
	}
	return nil
}

// RequestPermission sends a permission prompt to the TUI and blocks until
// the user responds. This is called from the agentic loop's goroutine
// when CheckPermission returns "ask" or "passthrough".
func (h *TUIPermissionHandler) RequestPermission(
	ctx context.Context, toolName string, input json.RawMessage,
) (bool, error) {
	if h.program == nil {
		return false, nil
	}

	// Get suggestions from the rule handler if available.
	var suggestions []config.PermissionSuggestion
	if h.ruleHandler != nil {
		result := h.ruleHandler.CheckPermission(toolName, input)
		suggestions = result.Suggestions
	}

	resultCh := make(chan PermissionResponse, 1)
	h.program.Send(PermissionRequestMsg{
		ToolName:    toolName,
		Input:       input,
		Summary:     summarizeForPermission(toolName, input),
		Suggestions: suggestions,
		ResultCh:    resultCh,
	})

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case response := <-resultCh:
		switch response {
		case PermissionAllow:
			return true, nil
		case PermissionAlwaysAllow:
			// Add a session-level "always allow" rule.
			h.addAlwaysAllowRule(toolName, input, suggestions)
			return true, nil
		default:
			return false, nil
		}
	}
}

// addAlwaysAllowRule adds a session-level rule to always allow this type
// of operation going forward.
func (h *TUIPermissionHandler) addAlwaysAllowRule(toolName string, input json.RawMessage, suggestions []config.PermissionSuggestion) {
	if h.ruleHandler == nil {
		return
	}
	permCtx := h.ruleHandler.GetPermissionContext()
	if permCtx == nil {
		return
	}

	// Use the first suggestion if available, otherwise create a generic one.
	var ruleStr string
	if len(suggestions) > 0 && len(suggestions[0].Rules) > 0 {
		ruleStr = config.FormatRuleString(suggestions[0].Rules[0])
	} else {
		ruleStr = toolName
	}

	permCtx.AddRules("allow", "session", []string{ruleStr})
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
	case "FileWrite", "Write":
		if s := getString("file_path"); s != "" {
			return fmt.Sprintf("Write to: %s", s)
		}
	case "FileEdit", "Edit":
		if s := getString("file_path"); s != "" {
			return fmt.Sprintf("Edit: %s", s)
		}
	case "NotebookEdit":
		if s := getString("notebook_path"); s != "" {
			return fmt.Sprintf("Edit notebook: %s", s)
		}
	case "WebFetch":
		if s := getString("url"); s != "" {
			return fmt.Sprintf("Fetch: %s", s)
		}
	case "WebSearch":
		if s := getString("query"); s != "" {
			return fmt.Sprintf("Search: %s", s)
		}
	case "EnterWorktree", "Worktree":
		if s := getString("branch"); s != "" {
			return fmt.Sprintf("Create worktree: %s", s)
		}
	}
	return ""
}

// renderPermissionPrompt produces the permission prompt text for the live region.
// If suggestions are provided, it shows an "always allow" option.
func renderPermissionPrompt(toolName, summary string, suggestions []config.PermissionSuggestion) string {
	title := permTitleStyle.Render("Permission Required")
	tool := "  Tool: " + permToolStyle.Render(toolName)

	result := title + "\n" + tool
	if summary != "" {
		result += "\n  " + permSummaryStyle.Render(summary)
	}

	// Show suggestion hint if available.
	if len(suggestions) > 0 && len(suggestions[0].Rules) > 0 {
		ruleStr := config.FormatRuleString(suggestions[0].Rules[0])
		result += "\n" + permHintStyle.Render("  Rule: "+ruleStr)
	}

	// Build hint line with highlighted key letters.
	hint := "  Press " +
		permKeyStyle.Render("y") + permActionStyle.Render(" to allow, ") +
		permKeyStyle.Render("n") + permActionStyle.Render(" to deny")
	if len(suggestions) > 0 {
		hint += permActionStyle.Render(", ") +
			permKeyStyle.Render("a") + permActionStyle.Render(" to always allow")
	}
	result += "\n" + hint

	return result
}

// renderPermissionResultLine produces the scrollback line for a resolved permission.
func renderPermissionResultLine(toolName, summary string, response PermissionResponse) string {
	var verdict string
	switch response {
	case PermissionAllow:
		verdict = diffAddStyle.Render("allowed")
	case PermissionAlwaysAllow:
		verdict = diffAddStyle.Render("always allowed")
	default:
		verdict = diffRemoveStyle.Render("denied")
	}
	line := toolBulletStyle.Render("  ") + permToolStyle.Render(toolName)
	if summary != "" {
		line += "  " + toolSummaryStyle.Render(summary)
	}
	line += "  " + verdict
	return line
}
