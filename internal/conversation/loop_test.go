package conversation

import (
	"testing"

	"github.com/anthropics/claude-code-go/internal/api"
)

func TestLoop_Clear(t *testing.T) {
	history := NewHistory()
	history.AddUserMessage("hello")
	history.AddUserMessage("world")

	loop := NewLoop(LoopConfig{
		History: history,
	})

	if loop.History().Len() != 2 {
		t.Fatalf("before clear: Len = %d, want 2", loop.History().Len())
	}

	loop.Clear()

	if loop.History().Len() != 0 {
		t.Errorf("after clear: Len = %d, want 0", loop.History().Len())
	}
	if loop.History().Messages() != nil {
		t.Errorf("after clear: Messages should be nil, got %v", loop.History().Messages())
	}
}

func TestLoop_ClearEmptyHistory(t *testing.T) {
	loop := NewLoop(LoopConfig{})

	// Clear on empty history should be a no-op.
	loop.Clear()

	if loop.History().Len() != 0 {
		t.Errorf("Len = %d, want 0", loop.History().Len())
	}
}

func TestLoop_ClearThenAddMessages(t *testing.T) {
	history := NewHistory()
	history.AddUserMessage("old message")

	loop := NewLoop(LoopConfig{
		History: history,
	})

	loop.Clear()

	// After clear, new messages should work normally.
	loop.History().AddUserMessage("new message")

	if loop.History().Len() != 1 {
		t.Fatalf("Len = %d, want 1", loop.History().Len())
	}

	msg := loop.History().Messages()[0]
	if msg.Role != api.RoleUser {
		t.Errorf("Role = %q, want %q", msg.Role, api.RoleUser)
	}
}

func TestLoop_SetOnTurnComplete(t *testing.T) {
	called := false
	loop := NewLoop(LoopConfig{
		OnTurnComplete: func(h *History) {
			t.Error("original callback should not be called")
		},
	})

	// Replace the callback.
	loop.SetOnTurnComplete(func(h *History) {
		called = true
	})

	// Trigger notifyTurnComplete indirectly by checking the field was set.
	// Since notifyTurnComplete is unexported, we verify via History() and
	// the fact that the loop was constructed with the new callback.
	// We can test this by adding a message and verifying the callback fires.
	loop.History().AddUserMessage("test")

	// Call the internal notify method via the exported Clear + manual check.
	// Since we can't call notifyTurnComplete directly, verify the setter worked
	// by reading back the behavior.
	if loop.onTurnComplete == nil {
		t.Fatal("onTurnComplete should not be nil after SetOnTurnComplete")
	}

	loop.onTurnComplete(loop.History())
	if !called {
		t.Error("replacement callback was not called")
	}
}

func TestLoop_SetOnTurnCompleteNil(t *testing.T) {
	loop := NewLoop(LoopConfig{
		OnTurnComplete: func(h *History) {
			t.Error("should not be called")
		},
	})

	loop.SetOnTurnComplete(nil)

	if loop.onTurnComplete != nil {
		t.Error("expected onTurnComplete to be nil")
	}
}
