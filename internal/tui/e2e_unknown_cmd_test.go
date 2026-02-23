package tui

import (
	"testing"
)

func TestE2E_UnknownSlashCommand(t *testing.T) {
	m, _ := testModel(t)

	result, _ := submitCommand(m, "/nonexistent")

	// Should remain in input mode (not crash or switch modes).
	if result.mode != modeInput {
		t.Errorf("mode = %d, want modeInput", result.mode)
	}
}

func TestE2E_UnknownSlashCommand_DoesNotCrash(t *testing.T) {
	m, _ := testModel(t)

	// Test various invalid commands. These should not panic.
	// Some may fuzzy-correct to real commands (e.g. /helpme → /help),
	// so we only check that they don't panic.
	commands := []string{
		"/",
		"/xyz",
		"/helpme",
		"/q",
		"/123",
		"/ spaces",
	}

	for _, cmd := range commands {
		t.Run(cmd, func(t *testing.T) {
			result, _ := submitCommand(m, cmd)
			// Should not panic. Some commands may fuzzy-correct to valid
			// commands that switch modes (e.g. /helpme → /help opens modeHelp),
			// so we just verify no crash occurs.
			_ = result
		})
	}
}

func TestE2E_RegularMessage_SentToLoop(t *testing.T) {
	m, _ := testModel(t)

	result, _ := submitCommand(m, "Hello, Claude!")

	// Regular messages switch to streaming mode.
	if result.mode != modeStreaming {
		t.Errorf("mode = %d, want modeStreaming (%d)", result.mode, modeStreaming)
	}
	if result.textInput.Focused() {
		t.Error("text input should be blurred during streaming")
	}
}

func TestE2E_EmptyLookup(t *testing.T) {
	m, _ := testModel(t)

	// "/" alone should be treated as the empty command name.
	_, ok := m.slashReg.lookup("")
	if ok {
		t.Error("empty string should not match any command")
	}
}
