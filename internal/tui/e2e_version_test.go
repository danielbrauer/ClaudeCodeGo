package tui

import (
	"strings"
	"testing"
)

func TestE2E_VersionCommand(t *testing.T) {
	m, _ := testModel(t, withVersion("2.1.50"))

	cmd, ok := m.slashReg.lookup("version")
	if !ok {
		t.Fatal("/version not registered")
	}
	output := cmd.Execute(&m)

	if !strings.Contains(output, "2.1.50") {
		t.Errorf("version output should contain version string, got %q", output)
	}
	if !strings.Contains(output, "Go") {
		t.Errorf("version output should mention Go implementation, got %q", output)
	}
}

func TestE2E_VersionCommand_ViaHandleSubmit(t *testing.T) {
	m, _ := testModel(t, withVersion("0.5.0"))

	result, _ := submitCommand(m, "/version")

	// Should remain in input mode.
	if result.mode != modeInput {
		t.Errorf("mode = %d, want modeInput", result.mode)
	}
}
