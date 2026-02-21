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

// Messages returns the current message list.
func (h *History) Messages() []api.Message {
	return h.messages
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
