package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
)

// markdownRenderer renders markdown text to styled ANSI output.
type markdownRenderer struct {
	renderer *glamour.TermRenderer
	width    int
}

// newMarkdownRenderer creates a renderer with the given terminal width.
func newMarkdownRenderer(width int) *markdownRenderer {
	if width < 40 {
		width = 80
	}
	r, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width-4), // small margin for safety
	)
	return &markdownRenderer{renderer: r, width: width}
}

// render converts markdown text to styled ANSI output.
func (r *markdownRenderer) render(md string) string {
	if r.renderer == nil {
		return md
	}
	out, err := r.renderer.Render(md)
	if err != nil {
		return md
	}
	// glamour often adds trailing newlines; trim for tighter display.
	return strings.TrimRight(out, "\n")
}

// updateWidth recreates the renderer with a new terminal width.
func (r *markdownRenderer) updateWidth(width int) {
	if width < 40 {
		width = 80
	}
	if width == r.width {
		return
	}
	r.width = width
	newR, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width-4),
	)
	if err == nil {
		r.renderer = newR
	}
}

// renderDiff produces a colored inline diff from old_string and new_string.
// Each line of old_string is prefixed with "- " in red, each line of
// new_string with "+ " in green.
func renderDiff(oldStr, newStr string) string {
	var b strings.Builder

	oldLines := strings.Split(oldStr, "\n")
	newLines := strings.Split(newStr, "\n")

	for _, line := range oldLines {
		b.WriteString(diffRemoveStyle.Render("  - "+line) + "\n")
	}
	for _, line := range newLines {
		b.WriteString(diffAddStyle.Render("  + "+line) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderToolComplete produces the scrollback line for a completed tool call.
// It returns the tool bullet line and optional extra detail (e.g. diff).
func renderToolComplete(name string, input json.RawMessage) string {
	var b strings.Builder

	bullet := toolBulletStyle.Render("  ")
	toolName := toolNameStyle.Render(name)
	summary := extractToolSummary(name, input)
	summaryStr := ""
	if summary != "" {
		summaryStr = "  " + toolSummaryStyle.Render(summary)
	}

	b.WriteString(bullet + toolName + summaryStr)

	// For FileEdit, show inline diff.
	if name == "FileEdit" {
		oldStr, newStr := extractEditStrings(input)
		if oldStr != "" || newStr != "" {
			b.WriteString("\n")
			b.WriteString(renderDiff(oldStr, newStr))
		}
	}

	return b.String()
}

// extractToolSummary returns a short description for a tool call.
func extractToolSummary(name string, input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
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

	switch name {
	case "Bash":
		if s := getString("command"); s != "" {
			if len(s) > 200 {
				s = s[:197] + "..."
			}
			return fmt.Sprintf("$ %s", s)
		}
	case "FileRead":
		return getString("file_path")
	case "FileEdit":
		return getString("file_path")
	case "FileWrite":
		return getString("file_path")
	case "Glob":
		return getString("pattern")
	case "Grep":
		if s := getString("pattern"); s != "" {
			return fmt.Sprintf("/%s/", s)
		}
	case "Agent":
		return getString("description")
	case "TodoWrite":
		return "updating task list"
	case "AskUserQuestion":
		return "asking user"
	case "WebFetch":
		return getString("url")
	case "WebSearch":
		if s := getString("query"); s != "" {
			return "searching: " + s
		}
	case "NotebookEdit":
		return getString("notebook_path")
	case "ExitPlanMode":
		return "plan ready"
	case "Config":
		return getString("setting")
	case "EnterWorktree":
		return "creating worktree"
	case "TaskOutput":
		if s := getString("task_id"); s != "" {
			return "reading task " + s
		}
	case "TaskStop":
		return "stopping task"
	}
	return ""
}

// extractEditStrings pulls old_string and new_string from FileEdit input.
func extractEditStrings(input json.RawMessage) (string, string) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(input, &m); err != nil {
		return "", ""
	}
	var oldStr, newStr string
	if v, ok := m["old_string"]; ok {
		json.Unmarshal(v, &oldStr)
	}
	if v, ok := m["new_string"]; ok {
		json.Unmarshal(v, &newStr)
	}
	return oldStr, newStr
}
