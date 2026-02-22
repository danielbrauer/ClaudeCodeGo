// Package tui implements the rich terminal UI for the Claude Code CLI using Bubble Tea.
package tui

import (
	"encoding/json"

	"github.com/anthropics/claude-code-go/internal/api"
)

// MessageStartMsg carries token usage from the start of an API response.
type MessageStartMsg struct {
	Usage api.Usage
}

// TextDeltaMsg carries incremental text from the API stream.
type TextDeltaMsg struct {
	Index int
	Text  string
}

// InputJSONDeltaMsg carries incremental tool input JSON.
type InputJSONDeltaMsg struct {
	Index int
	JSON  string
}

// ContentBlockStartMsg signals the start of a content block.
type ContentBlockStartMsg struct {
	Index int
	Block api.ContentBlock
}

// ContentBlockStopMsg signals the end of a content block.
// For tool_use blocks, Name and Input are populated.
type ContentBlockStopMsg struct {
	Index int
	Name  string
	Input json.RawMessage
}

// MessageDeltaMsg carries stop_reason and final usage.
type MessageDeltaMsg struct {
	Delta api.MessageDeltaBody
	Usage *api.Usage
}

// MessageStopMsg signals the end of the streamed message.
type MessageStopMsg struct{}

// StreamErrorMsg signals a streaming error.
type StreamErrorMsg struct {
	Err error
}

// LoopDoneMsg signals the agentic loop has finished.
type LoopDoneMsg struct {
	Err error
}

// PermissionRequestMsg is sent when a tool needs user approval.
// The tool's goroutine blocks on ResultCh until the TUI sends a response.
type PermissionRequestMsg struct {
	ToolName string
	Input    json.RawMessage
	Summary  string
	ResultCh chan bool
}

// SubmitInputMsg is sent when the user presses Enter to submit input.
type SubmitInputMsg struct {
	Text string
}
