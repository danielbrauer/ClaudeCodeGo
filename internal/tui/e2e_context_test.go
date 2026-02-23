package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/anthropics/claude-code-go/internal/mock"
)

func TestE2E_ContextCommand_EmptyHistory(t *testing.T) {
	m, _ := testModel(t)

	cmd, ok := m.slashReg.lookup("context")
	if !ok {
		t.Fatal("/context not registered")
	}
	output := cmd.Execute(&m)

	if !strings.Contains(output, "Messages in history: 0") {
		t.Errorf("context output should show 0 messages, got %q", output)
	}
}

func TestE2E_ContextCommand_WithMessages(t *testing.T) {
	m, _ := testModel(t, withResponder(&mock.StaticResponder{
		Response: mock.TextResponse("Hello!", 1),
	}))

	// Send a message to populate history.
	err := m.loop.SendMessage(context.Background(), "Hi")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	cmd, _ := m.slashReg.lookup("context")
	output := cmd.Execute(&m)

	// Should have 2 messages: user + assistant.
	if !strings.Contains(output, "Messages in history: 2") {
		t.Errorf("context output should show 2 messages, got %q", output)
	}
}

func TestE2E_ContextCommand_ViaHandleSubmit(t *testing.T) {
	m, _ := testModel(t)

	result, _ := submitCommand(m, "/context")

	// Should remain in input mode.
	if result.mode != modeInput {
		t.Errorf("mode = %d, want modeInput", result.mode)
	}
}
