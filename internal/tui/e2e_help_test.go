package tui

import (
	"strings"
	"testing"
)

func TestE2E_HelpCommand(t *testing.T) {
	m, _ := testModel(t)

	// Execute /help via the registry (it has a non-nil Execute).
	cmd, ok := m.slashReg.lookup("help")
	if !ok {
		t.Fatal("/help not registered")
	}
	output := cmd.Execute(&m)

	if !strings.Contains(output, "Available commands:") {
		t.Error("help output should contain 'Available commands:'")
	}

	// Should list key commands.
	for _, name := range []string{"/help", "/model", "/cost", "/context", "/compact", "/clear", "/memory", "/init", "/quit"} {
		if !strings.Contains(output, name) {
			t.Errorf("help output should list %s", name)
		}
	}

	// Aliases should be hidden from help.
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		for _, alias := range []string{"/exit", "/reset", "/new", "/settings"} {
			if strings.HasPrefix(trimmed, alias+" ") || strings.HasPrefix(trimmed, alias+"\t") || trimmed == alias {
				t.Errorf("help should not list alias %s, found: %q", alias, line)
			}
		}
	}
}

func TestE2E_HelpCommand_ViaHandleSubmit(t *testing.T) {
	m, _ := testModel(t)

	result, _ := submitCommand(m, "/help")

	// After /help, model should remain in modeInput.
	if result.mode != modeInput {
		t.Errorf("mode = %d, want modeInput (%d)", result.mode, modeInput)
	}
}
