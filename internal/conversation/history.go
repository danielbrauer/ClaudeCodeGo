package conversation

import (
	"encoding/json"

	"github.com/anthropics/claude-code-go/internal/api"
)

// History manages conversation messages for the agentic loop.
type History struct {
	messages []api.Message
}

// NewHistory creates an empty conversation history.
func NewHistory() *History {
	return &History{}
}

// NewHistoryFrom creates a history pre-populated with messages (for session resume).
func NewHistoryFrom(msgs []api.Message) *History {
	cp := make([]api.Message, len(msgs))
	copy(cp, msgs)
	return &History{messages: cp}
}

// Messages returns the current message list.
func (h *History) Messages() []api.Message {
	return h.messages
}

// SetMessages replaces the message list (for session resume or compaction).
func (h *History) SetMessages(msgs []api.Message) {
	h.messages = msgs
}

// AddUserMessage appends a user text message.
func (h *History) AddUserMessage(text string) {
	h.messages = append(h.messages, api.NewTextMessage(api.RoleUser, text))
}

// AddAssistantResponse appends the assistant's response (with content blocks).
func (h *History) AddAssistantResponse(blocks []api.ContentBlock) {
	h.messages = append(h.messages, api.NewBlockMessage(api.RoleAssistant, blocks))
}

// AddToolResults appends tool result blocks as a user message.
func (h *History) AddToolResults(results []api.ContentBlock) {
	h.messages = append(h.messages, api.NewBlockMessage(api.RoleUser, results))
}

// Len returns the number of messages.
func (h *History) Len() int {
	return len(h.messages)
}

// ReplaceRange replaces messages[start:end] with replacement messages.
// Used by compaction to swap out detailed messages with a summary.
func (h *History) ReplaceRange(start, end int, replacement []api.Message) {
	if start < 0 || end > len(h.messages) || start > end {
		return
	}
	var newMsgs []api.Message
	newMsgs = append(newMsgs, h.messages[:start]...)
	newMsgs = append(newMsgs, replacement...)
	newMsgs = append(newMsgs, h.messages[end:]...)
	h.messages = newMsgs
}

// MakeToolResult creates a tool_result content block.
func MakeToolResult(toolUseID string, content string, isError bool) api.ContentBlock {
	contentJSON, _ := json.Marshal(content)
	return api.ContentBlock{
		Type:      api.ContentTypeToolResult,
		ToolUseID: toolUseID,
		Content:   contentJSON,
		IsError:   isError,
	}
}
