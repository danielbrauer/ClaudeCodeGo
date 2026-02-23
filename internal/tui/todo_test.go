package tui

import (
	"strings"
	"testing"

	"github.com/anthropics/claude-code-go/internal/tools"
)

func TestRenderTodoList_Empty(t *testing.T) {
	got := renderTodoList(nil)
	if got != "" {
		t.Errorf("expected empty string for nil todos, got %q", got)
	}
	got = renderTodoList([]tools.TodoItem{})
	if got != "" {
		t.Errorf("expected empty string for empty slice, got %q", got)
	}
}

func TestRenderTodoList_SingleItem(t *testing.T) {
	todos := []tools.TodoItem{
		{Content: "Fix bug", Status: "pending", ActiveForm: "Fixing bug"},
	}
	got := renderTodoList(todos)
	if strings.Contains(got, "\n") {
		t.Errorf("single item should produce no newlines, got %q", got)
	}
	if !strings.Contains(got, "[ ]") {
		t.Errorf("pending item should contain [ ], got %q", got)
	}
	if !strings.Contains(got, "Fix bug") {
		t.Errorf("pending item should show Content, got %q", got)
	}
}

func TestRenderTodoList_ConsistentIndentation(t *testing.T) {
	todos := []tools.TodoItem{
		{Content: "Create test file with pytest structure and CEC mocking", Status: "in_progress", ActiveForm: "Creating test file with pytest structure and CEC mocking"},
		{Content: "Write HTTP route tests (status, on, off)", Status: "pending", ActiveForm: "Writing HTTP route tests"},
		{Content: "Write input validation tests", Status: "pending", ActiveForm: "Writing input validation tests"},
	}
	got := renderTodoList(todos)

	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), got)
	}

	// Each line should start with "  " (2-space indent) after stripping ANSI.
	for i, line := range lines {
		stripped := stripANSI(line)
		if !strings.HasPrefix(stripped, "  ") {
			t.Errorf("line %d should start with 2-space indent, got %q", i, stripped)
		}
		// The third character should be '[' (start of icon).
		if len(stripped) < 3 || stripped[2] != '[' {
			t.Errorf("line %d should have '[' at position 2, got %q", i, stripped)
		}
	}
}

func TestRenderTodoList_StatusIcons(t *testing.T) {
	todos := []tools.TodoItem{
		{Content: "Done task", Status: "completed", ActiveForm: "Doing task"},
		{Content: "Current task", Status: "in_progress", ActiveForm: "Working on task"},
		{Content: "Future task", Status: "pending", ActiveForm: "Planning task"},
	}
	got := renderTodoList(todos)
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	// Check icons in stripped text.
	stripped0 := stripANSI(lines[0])
	stripped1 := stripANSI(lines[1])
	stripped2 := stripANSI(lines[2])

	if !strings.Contains(stripped0, "[x]") {
		t.Errorf("completed item should have [x], got %q", stripped0)
	}
	if !strings.Contains(stripped1, "[~]") {
		t.Errorf("in_progress item should have [~], got %q", stripped1)
	}
	if !strings.Contains(stripped2, "[ ]") {
		t.Errorf("pending item should have [ ], got %q", stripped2)
	}
}

func TestRenderTodoList_InProgressUsesActiveForm(t *testing.T) {
	todos := []tools.TodoItem{
		{Content: "Run tests", Status: "in_progress", ActiveForm: "Running tests"},
	}
	got := renderTodoList(todos)
	stripped := stripANSI(got)

	if !strings.Contains(stripped, "Running tests") {
		t.Errorf("in_progress should use ActiveForm, got %q", stripped)
	}
	if strings.Contains(stripped, "Run tests") {
		t.Errorf("in_progress should not use Content, got %q", stripped)
	}
}

func TestRenderTodoList_CompletedUsesContent(t *testing.T) {
	todos := []tools.TodoItem{
		{Content: "Run tests", Status: "completed", ActiveForm: "Running tests"},
	}
	got := renderTodoList(todos)
	stripped := stripANSI(got)

	if !strings.Contains(stripped, "Run tests") {
		t.Errorf("completed should use Content, got %q", stripped)
	}
}

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' {
			// Skip until we find the terminator (letter).
			i++
			if i < len(s) && s[i] == '[' {
				i++
				for i < len(s) && !isANSITerminator(s[i]) {
					i++
				}
				if i < len(s) {
					i++ // skip the terminator
				}
			}
		} else {
			out.WriteByte(s[i])
			i++
		}
	}
	return out.String()
}

func isANSITerminator(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}
