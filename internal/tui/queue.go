package tui

// inputQueue holds user messages that were submitted while the agent was busy.
// When the agentic loop finishes, queued messages are automatically sent one
// at a time, simulating the same "type-ahead" behavior as the official JS CLI.
type inputQueue struct {
	items []queuedInput
}

// queuedInput represents a single queued user message.
type queuedInput struct {
	Text string
}

// Enqueue adds a user message to the end of the queue.
func (q *inputQueue) Enqueue(text string) {
	q.items = append(q.items, queuedInput{Text: text})
}

// Dequeue removes and returns the first queued message, or empty string + false
// if the queue is empty.
func (q *inputQueue) Dequeue() (string, bool) {
	if len(q.items) == 0 {
		return "", false
	}
	item := q.items[0]
	q.items = q.items[1:]
	return item.Text, true
}

// Len returns the number of queued messages.
func (q *inputQueue) Len() int {
	return len(q.items)
}

// Clear removes all queued messages.
func (q *inputQueue) Clear() {
	q.items = nil
}

// Items returns a copy of the current queued items for display purposes.
func (q *inputQueue) Items() []queuedInput {
	out := make([]queuedInput, len(q.items))
	copy(out, q.items)
	return out
}

// RemoveLast removes the most recently enqueued item (for editing/cancelling).
// Returns the removed item's text, or empty + false if nothing to remove.
func (q *inputQueue) RemoveLast() (string, bool) {
	if len(q.items) == 0 {
		return "", false
	}
	last := q.items[len(q.items)-1]
	q.items = q.items[:len(q.items)-1]
	return last.Text, true
}

// pluralS returns "s" if n != 1, for simple English pluralization.
func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// truncateText shortens text to maxLen characters, adding "..." if truncated.
func truncateText(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
