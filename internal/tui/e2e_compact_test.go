package tui

import (
	"context"
	"testing"

	"github.com/anthropics/claude-code-go/internal/conversation"
	"github.com/anthropics/claude-code-go/internal/mock"
)

func TestE2E_CompactCommand_SwitchesToStreaming(t *testing.T) {
	m, _ := testModel(t)

	result, _ := submitCommand(m, "/compact")

	// /compact runs async, so the model should switch to modeStreaming.
	if result.mode != modeStreaming {
		t.Errorf("mode = %d, want modeStreaming (%d)", result.mode, modeStreaming)
	}
}

func TestE2E_CompactCommand_WithCompactor(t *testing.T) {
	// Create a mock backend that can handle compaction requests.
	responder := &mock.StaticResponder{
		Response: mock.TextResponse("Summary of conversation.", 1),
	}
	b := mock.NewBackend(responder)
	t.Cleanup(b.Close)

	client := b.Client()
	compactor := conversation.NewCompactor(client)

	// Pre-populate history with messages so compaction has something to work with.
	m, _ := testModel(t,
		withCompactor(compactor),
		withResponder(responder),
	)

	// Add enough messages to make compaction meaningful.
	for i := 0; i < 10; i++ {
		m.loop.History().AddUserMessage("test message")
		m.loop.History().AddAssistantResponse(nil)
	}

	initialLen := m.loop.History().Len()
	if initialLen < 10 {
		t.Fatalf("expected at least 10 messages, got %d", initialLen)
	}

	// Run compact directly (not via handleSubmit which runs async).
	err := m.loop.Compact(context.Background())
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}

	// After compaction, history should be shorter.
	if m.loop.History().Len() >= initialLen {
		t.Errorf("history should be shorter after compaction: before=%d, after=%d",
			initialLen, m.loop.History().Len())
	}
}

func TestE2E_CompactCommand_NoCompactor(t *testing.T) {
	m, _ := testModel(t) // no compactor configured

	// Direct call to loop.Compact should error.
	err := m.loop.Compact(context.Background())
	if err == nil {
		t.Error("Compact without compactor should return error")
	}
}
