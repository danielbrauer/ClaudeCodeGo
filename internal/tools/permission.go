package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// TerminalPermissionHandler prompts the user for permission via the terminal.
type TerminalPermissionHandler struct {
	reader *bufio.Reader
}

// NewTerminalPermissionHandler creates a permission handler that reads from stdin.
func NewTerminalPermissionHandler() *TerminalPermissionHandler {
	return &TerminalPermissionHandler{
		reader: bufio.NewReader(os.Stdin),
	}
}

// RequestPermission prompts the user to allow or deny a tool call.
func (h *TerminalPermissionHandler) RequestPermission(ctx context.Context, toolName string, input json.RawMessage) (bool, error) {
	summary := summarizeToolInput(toolName, input)
	fmt.Printf("\n--- Permission Required ---\n")
	fmt.Printf("Tool: %s\n", toolName)
	if summary != "" {
		fmt.Printf("  %s\n", summary)
	}
	fmt.Print("Allow? [y/n]: ")

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	line, err := h.reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("reading input: %w", err)
	}

	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes", nil
}

// summarizeToolInput produces a short description of what the tool will do.
func summarizeToolInput(toolName string, input json.RawMessage) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(input, &m); err != nil {
		return ""
	}

	switch toolName {
	case "Bash":
		if cmd, ok := m["command"]; ok {
			var s string
			json.Unmarshal(cmd, &s)
			if len(s) > 120 {
				s = s[:117] + "..."
			}
			return fmt.Sprintf("$ %s", s)
		}
	case "FileWrite":
		if fp, ok := m["file_path"]; ok {
			var s string
			json.Unmarshal(fp, &s)
			return fmt.Sprintf("Write to: %s", s)
		}
	case "FileEdit":
		if fp, ok := m["file_path"]; ok {
			var s string
			json.Unmarshal(fp, &s)
			return fmt.Sprintf("Edit: %s", s)
		}
	}
	return ""
}

// AlwaysAllowPermissionHandler approves all tool calls without prompting.
// Useful for non-interactive / scripting modes.
type AlwaysAllowPermissionHandler struct{}

// RequestPermission always returns true.
func (h *AlwaysAllowPermissionHandler) RequestPermission(_ context.Context, _ string, _ json.RawMessage) (bool, error) {
	return true, nil
}
