package conversation

import (
	"testing"

	"github.com/anthropics/claude-code-go/internal/api"
)

func TestNewHistoryFrom(t *testing.T) {
	msgs := []api.Message{
		api.NewTextMessage("user", "hello"),
		api.NewTextMessage("assistant", "hi"),
	}

	h := NewHistoryFrom(msgs)
	if h.Len() != 2 {
		t.Errorf("Len = %d, want 2", h.Len())
	}

	// Verify content was copied.
	if h.Messages()[0].Role != "user" {
		t.Errorf("Messages[0].Role = %q, want %q", h.Messages()[0].Role, "user")
	}
}

func TestSetMessages(t *testing.T) {
	h := NewHistory()
	h.AddUserMessage("old message")

	newMsgs := []api.Message{
		api.NewTextMessage("user", "new message"),
	}
	h.SetMessages(newMsgs)

	if h.Len() != 1 {
		t.Errorf("Len = %d, want 1", h.Len())
	}
}

func TestReplaceRange(t *testing.T) {
	h := NewHistory()
	h.AddUserMessage("msg1")
	h.AddUserMessage("msg2")
	h.AddUserMessage("msg3")
	h.AddUserMessage("msg4")

	// Replace messages 1-3 with a single summary.
	replacement := []api.Message{
		api.NewTextMessage("user", "summary of msg2 and msg3"),
	}
	h.ReplaceRange(1, 3, replacement)

	if h.Len() != 3 {
		t.Fatalf("Len = %d, want 3", h.Len())
	}

	// First message unchanged.
	msgs := h.Messages()
	var text string
	json := string(msgs[0].Content)
	if json != `"msg1"` {
		t.Errorf("Messages[0] content = %s, want \"msg1\"", json)
	}

	// Middle message is the summary.
	text = string(msgs[1].Content)
	if text != `"summary of msg2 and msg3"` {
		t.Errorf("Messages[1] content = %s, want summary", text)
	}

	// Last message unchanged.
	text = string(msgs[2].Content)
	if text != `"msg4"` {
		t.Errorf("Messages[2] content = %s, want \"msg4\"", text)
	}
}

func TestReplaceRangeInvalidBounds(t *testing.T) {
	h := NewHistory()
	h.AddUserMessage("msg1")

	// Invalid range should be a no-op.
	h.ReplaceRange(-1, 5, nil)
	if h.Len() != 1 {
		t.Errorf("Len = %d, want 1 (no-op for invalid range)", h.Len())
	}

	h.ReplaceRange(2, 1, nil) // start > end
	if h.Len() != 1 {
		t.Errorf("Len = %d, want 1 (no-op for start > end)", h.Len())
	}
}

func TestNewHistoryFromIndependence(t *testing.T) {
	msgs := []api.Message{
		api.NewTextMessage("user", "hello"),
	}

	h := NewHistoryFrom(msgs)

	// Modifying the original slice should not affect the history.
	msgs[0] = api.NewTextMessage("user", "modified")

	if string(h.Messages()[0].Content) != `"hello"` {
		t.Error("NewHistoryFrom should copy messages, not reference them")
	}
}
