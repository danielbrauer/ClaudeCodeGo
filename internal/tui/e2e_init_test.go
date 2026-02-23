package tui

import (
	"testing"
)

func TestE2E_InitCommand_SwitchesToStreaming(t *testing.T) {
	m, _ := testModel(t)

	result, _ := submitCommand(m, "/init")

	// /init sends a prompt to the loop and switches to streaming mode.
	if result.mode != modeStreaming {
		t.Errorf("mode = %d, want modeStreaming (%d)", result.mode, modeStreaming)
	}
}

func TestE2E_InitCommand_BlursInput(t *testing.T) {
	m, _ := testModel(t)

	result, _ := submitCommand(m, "/init")

	// Text input should be blurred during streaming.
	if result.textInput.Focused() {
		t.Error("text input should be blurred during /init streaming")
	}
}
