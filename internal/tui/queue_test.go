package tui

import (
	"testing"
)

func TestInputQueue_EnqueueDequeue(t *testing.T) {
	var q inputQueue

	// Empty queue returns false.
	_, ok := q.Dequeue()
	if ok {
		t.Fatal("expected empty queue to return false")
	}

	// Enqueue items.
	q.Enqueue("first")
	q.Enqueue("second")
	q.Enqueue("third")

	if q.Len() != 3 {
		t.Fatalf("expected len 3, got %d", q.Len())
	}

	// Dequeue in FIFO order.
	text, ok := q.Dequeue()
	if !ok || text != "first" {
		t.Fatalf("expected 'first', got %q (ok=%v)", text, ok)
	}

	text, ok = q.Dequeue()
	if !ok || text != "second" {
		t.Fatalf("expected 'second', got %q (ok=%v)", text, ok)
	}

	if q.Len() != 1 {
		t.Fatalf("expected len 1, got %d", q.Len())
	}

	text, ok = q.Dequeue()
	if !ok || text != "third" {
		t.Fatalf("expected 'third', got %q (ok=%v)", text, ok)
	}

	// Now empty.
	_, ok = q.Dequeue()
	if ok {
		t.Fatal("expected empty queue after draining")
	}
}

func TestInputQueue_Clear(t *testing.T) {
	var q inputQueue

	q.Enqueue("a")
	q.Enqueue("b")
	q.Clear()

	if q.Len() != 0 {
		t.Fatalf("expected empty queue after clear, got %d", q.Len())
	}

	_, ok := q.Dequeue()
	if ok {
		t.Fatal("expected false after clear")
	}
}

func TestInputQueue_RemoveLast(t *testing.T) {
	var q inputQueue

	// Remove from empty queue.
	_, ok := q.RemoveLast()
	if ok {
		t.Fatal("expected false for empty queue")
	}

	q.Enqueue("first")
	q.Enqueue("second")
	q.Enqueue("third")

	// Remove the most recently added item.
	text, ok := q.RemoveLast()
	if !ok || text != "third" {
		t.Fatalf("expected 'third', got %q (ok=%v)", text, ok)
	}

	if q.Len() != 2 {
		t.Fatalf("expected len 2, got %d", q.Len())
	}

	// Remaining items are still in order.
	text, ok = q.Dequeue()
	if !ok || text != "first" {
		t.Fatalf("expected 'first', got %q", text)
	}
	text, ok = q.Dequeue()
	if !ok || text != "second" {
		t.Fatalf("expected 'second', got %q", text)
	}
}

func TestInputQueue_Items(t *testing.T) {
	var q inputQueue

	q.Enqueue("a")
	q.Enqueue("b")

	items := q.Items()
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Text != "a" || items[1].Text != "b" {
		t.Fatalf("unexpected items: %+v", items)
	}

	// Items returns a copy — mutating it shouldn't affect the queue.
	items[0].Text = "modified"
	original := q.Items()
	if original[0].Text != "a" {
		t.Fatal("Items should return a copy, not a reference")
	}
}

func TestTruncateText(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 8, "hello..."},
		{"abcdefgh", 3, "abc"},
		{"", 5, ""},
		{"ab", 2, "ab"},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc"},
	}

	for _, tt := range tests {
		got := truncateText(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateText(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestPluralS(t *testing.T) {
	if pluralS(0) != "s" {
		t.Error("expected 's' for 0")
	}
	if pluralS(1) != "" {
		t.Error("expected '' for 1")
	}
	if pluralS(2) != "s" {
		t.Error("expected 's' for 2")
	}
}
