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

	// Test various invalid commands.
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
			// Should not panic and should stay in a valid mode.
			if result.mode != modeInput {
				t.Errorf("mode = %d after %q, want modeInput", result.mode, cmd)
			}
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
