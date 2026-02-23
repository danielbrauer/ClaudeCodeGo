package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestQueueing_EnqueueDuringStreaming(t *testing.T) {
	m, _ := testModel(t)

	// Simulate: user sends first message, model enters streaming mode.
	m, _ = submitCommand(m, "hello")
	if m.mode != modeStreaming {
		t.Fatalf("expected modeStreaming, got %d", m.mode)
	}

	// Simulate: user types and presses Enter during streaming.
	result, _ := m.handleStreamingKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(model)
	// No text in input → should be a no-op.
	if m.queue.Len() != 0 {
		t.Fatalf("expected no queued messages for empty input, got %d", m.queue.Len())
	}

	// Set some text in the input and press Enter.
	m.textInput.SetValue("follow up question")
	result, cmd := m.handleStreamingKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(model)

	if m.queue.Len() != 1 {
		t.Fatalf("expected 1 queued message, got %d", m.queue.Len())
	}
	if m.textInput.Value() != "" {
		t.Fatal("expected input to be cleared after queueing")
	}
	if cmd == nil {
		t.Fatal("expected tea.Println cmd for queued echo")
	}
}

func TestQueueing_MultipleMsgsQueued(t *testing.T) {
	m, _ := testModel(t)

	// Enter streaming mode.
	m, _ = submitCommand(m, "start")

	// Queue multiple messages.
	m.textInput.SetValue("msg1")
	result, _ := m.handleStreamingKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(model)

	m.textInput.SetValue("msg2")
	result, _ = m.handleStreamingKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(model)

	m.textInput.SetValue("msg3")
	result, _ = m.handleStreamingKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(model)

	if m.queue.Len() != 3 {
		t.Fatalf("expected 3 queued messages, got %d", m.queue.Len())
	}

	// Verify FIFO order.
	items := m.queue.Items()
	if items[0].Text != "msg1" || items[1].Text != "msg2" || items[2].Text != "msg3" {
		t.Fatalf("unexpected queue order: %+v", items)
	}
}

func TestQueueing_EscapeRemovesLastQueued(t *testing.T) {
	m, _ := testModel(t)

	// Enter streaming mode.
	m, _ = submitCommand(m, "start")

	// Queue two messages.
	m.textInput.SetValue("keep this")
	result, _ := m.handleStreamingKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(model)

	m.textInput.SetValue("remove this")
	result, _ = m.handleStreamingKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(model)

	if m.queue.Len() != 2 {
		t.Fatalf("expected 2 queued messages, got %d", m.queue.Len())
	}

	// Press Escape with empty input → removes last queued message.
	result, _ = m.handleStreamingKey(tea.KeyMsg{Type: tea.KeyEscape})
	m = result.(model)

	if m.queue.Len() != 1 {
		t.Fatalf("expected 1 queued message after escape, got %d", m.queue.Len())
	}

	items := m.queue.Items()
	if items[0].Text != "keep this" {
		t.Fatalf("expected 'keep this', got %q", items[0].Text)
	}
}

func TestQueueing_EscapeClearsInputFirst(t *testing.T) {
	m, _ := testModel(t)

	// Enter streaming mode.
	m, _ = submitCommand(m, "start")

	// Queue a message.
	m.textInput.SetValue("queued")
	result, _ := m.handleStreamingKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(model)

	// Type something new, then press Escape.
	m.textInput.SetValue("some text")
	result, _ = m.handleStreamingKey(tea.KeyMsg{Type: tea.KeyEscape})
	m = result.(model)

	// Escape should clear the input text, NOT remove the queued message.
	if m.textInput.Value() != "" {
		t.Fatal("expected input to be cleared by Escape")
	}
	if m.queue.Len() != 1 {
		t.Fatalf("expected queued message to remain, got %d", m.queue.Len())
	}
}

func TestQueueing_CtrlCClearsQueue(t *testing.T) {
	m, _ := testModel(t)

	// Enter streaming mode.
	m, _ = submitCommand(m, "start")

	// Queue messages.
	m.textInput.SetValue("msg1")
	result, _ := m.handleStreamingKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(model)

	m.textInput.SetValue("msg2")
	result, _ = m.handleStreamingKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(model)

	// Ctrl+C clears the queue.
	result, _ = m.handleStreamingKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = result.(model)

	if m.queue.Len() != 0 {
		t.Fatalf("expected queue cleared after Ctrl+C, got %d", m.queue.Len())
	}
}

func TestQueueing_LoopDoneDequeues(t *testing.T) {
	m, _ := testModel(t)

	// Manually put model in streaming mode and add queued items.
	m.mode = modeStreaming
	m.queue.Enqueue("queued msg")

	// Simulate LoopDoneMsg.
	result, cmd := m.Update(LoopDoneMsg{})
	m = result.(model)

	// The queued message should have been dequeued and submitted.
	// Model should stay in streaming mode (processing the queued msg).
	if m.mode != modeStreaming {
		t.Fatalf("expected modeStreaming (processing queued msg), got %d", m.mode)
	}
	if m.queue.Len() != 0 {
		t.Fatalf("expected queue to be empty after dequeue, got %d", m.queue.Len())
	}
	if cmd == nil {
		t.Fatal("expected a cmd from handleSubmit")
	}
}

func TestQueueing_LoopDoneReturnsToInputWhenEmpty(t *testing.T) {
	m, _ := testModel(t)

	// Streaming mode, empty queue.
	m.mode = modeStreaming

	result, _ := m.Update(LoopDoneMsg{})
	m = result.(model)

	if m.mode != modeInput {
		t.Fatalf("expected modeInput when queue is empty, got %d", m.mode)
	}
}

func TestQueueing_ViewShowsQueueCount(t *testing.T) {
	m, _ := testModel(t)

	// Streaming with queued messages.
	m.mode = modeStreaming
	m.queue.Enqueue("msg1")
	m.queue.Enqueue("msg2")

	view := m.View()
	if !strings.Contains(view, "2 messages queued") {
		t.Fatalf("expected queue count in view, got:\n%s", view)
	}
}

func TestQueueing_ViewShowsInputDuringStreaming(t *testing.T) {
	m, _ := testModel(t)

	// Streaming mode with some text typed.
	m.mode = modeStreaming
	m.textInput.SetValue("typing something")

	view := m.View()

	// Should show the input borders and hint.
	if !strings.Contains(view, "enter to queue") {
		t.Fatalf("expected queue hint in view, got:\n%s", view)
	}
}

func TestQueueing_ViewShowsInputWhenEmptyDuringStreaming(t *testing.T) {
	m, _ := testModel(t)

	// Streaming mode with empty input — input area should still be visible.
	m.mode = modeStreaming

	view := m.View()

	// Should show the input area with interrupt hint even when input is empty.
	if !strings.Contains(view, "esc to interrupt") {
		t.Fatalf("expected interrupt hint in view during streaming, got:\n%s", view)
	}
	if !strings.Contains(view, "enter to queue") {
		t.Fatalf("expected queue hint in view during streaming, got:\n%s", view)
	}
}

func TestQueueing_ClearCommandClearsQueue(t *testing.T) {
	m, _ := testModel(t)

	// Queue some messages.
	m.queue.Enqueue("msg1")
	m.queue.Enqueue("msg2")

	// Run /clear.
	m, _ = submitCommand(m, "/clear")

	if m.queue.Len() != 0 {
		t.Fatalf("expected queue cleared after /clear, got %d", m.queue.Len())
	}
}
