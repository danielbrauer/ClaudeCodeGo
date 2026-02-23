package mock

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/anthropics/claude-code-go/internal/api"
)

// Responder decides what the mock backend returns for a given API request.
type Responder interface {
	// Respond returns the MessageResponse for the given request.
	// It may inspect the request messages, tools, etc. to decide.
	Respond(req *api.CreateMessageRequest) *api.MessageResponse
}

// ResponderFunc adapts a plain function to the Responder interface.
type ResponderFunc func(req *api.CreateMessageRequest) *api.MessageResponse

func (f ResponderFunc) Respond(req *api.CreateMessageRequest) *api.MessageResponse {
	return f(req)
}

// --- Built-in responders ---

// StaticResponder always returns the same response.
type StaticResponder struct {
	Response *api.MessageResponse
}

func (r *StaticResponder) Respond(_ *api.CreateMessageRequest) *api.MessageResponse {
	return r.Response
}

// ScriptedResponder returns responses from a pre-defined sequence. After the
// sequence is exhausted, it returns the last response for all subsequent calls.
// This is useful for testing multi-turn conversations.
type ScriptedResponder struct {
	mu        sync.Mutex
	responses []*api.MessageResponse
	index     int
}

// NewScriptedResponder creates a responder that plays back the given responses
// in order. The responses slice must have at least one entry.
func NewScriptedResponder(responses []*api.MessageResponse) *ScriptedResponder {
	if len(responses) == 0 {
		panic("ScriptedResponder requires at least one response")
	}
	return &ScriptedResponder{responses: responses}
}

func (r *ScriptedResponder) Respond(_ *api.CreateMessageRequest) *api.MessageResponse {
	r.mu.Lock()
	defer r.mu.Unlock()
	resp := r.responses[r.index]
	if r.index < len(r.responses)-1 {
		r.index++
	}
	return resp
}

// CallCount returns the number of times Respond has been called.
func (r *ScriptedResponder) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.index < len(r.responses)-1 {
		return r.index
	}
	return r.index
}

// EchoResponder returns a text response that echoes the last user message.
// Useful for basic connectivity/integration tests.
type EchoResponder struct {
	callCount atomic.Int32
}

func (r *EchoResponder) Respond(req *api.CreateMessageRequest) *api.MessageResponse {
	n := r.callCount.Add(1)

	// Extract the last user message text.
	text := "(no message)"
	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		if msg.Role != api.RoleUser {
			continue
		}
		// Try to decode as plain string first.
		var s string
		if err := json.Unmarshal(msg.Content, &s); err == nil {
			text = s
			break
		}
		// Try as content blocks.
		var blocks []api.ContentBlock
		if err := json.Unmarshal(msg.Content, &blocks); err == nil {
			for _, b := range blocks {
				if b.Type == api.ContentTypeText {
					text = b.Text
					break
				}
			}
			break
		}
		break
	}

	return TextResponse(fmt.Sprintf("Echo: %s", text), int(n))
}

// CallCount returns the number of requests handled.
func (r *EchoResponder) CallCount() int32 {
	return r.callCount.Load()
}

// --- Response builder helpers ---

// TextResponse creates a simple text-only MessageResponse with stop_reason end_turn.
func TextResponse(text string, seqNum int) *api.MessageResponse {
	return &api.MessageResponse{
		ID:         fmt.Sprintf("msg_mock_%d", seqNum),
		Type:       "message",
		Role:       api.RoleAssistant,
		Model:      api.ModelClaude46Sonnet,
		StopReason: api.StopReasonEndTurn,
		Content: []api.ContentBlock{
			{Type: api.ContentTypeText, Text: text},
		},
		Usage: api.Usage{InputTokens: 10, OutputTokens: 20},
	}
}

// ToolUseResponse creates a MessageResponse that requests one tool call,
// with stop_reason tool_use.
func ToolUseResponse(toolID, toolName string, input json.RawMessage, seqNum int) *api.MessageResponse {
	return &api.MessageResponse{
		ID:         fmt.Sprintf("msg_mock_%d", seqNum),
		Type:       "message",
		Role:       api.RoleAssistant,
		Model:      api.ModelClaude46Sonnet,
		StopReason: api.StopReasonToolUse,
		Content: []api.ContentBlock{
			{
				Type:  api.ContentTypeToolUse,
				ID:    toolID,
				Name:  toolName,
				Input: input,
			},
		},
		Usage: api.Usage{InputTokens: 10, OutputTokens: 30},
	}
}

// ToolUseWithTextResponse creates a response that contains both a text block
// and a tool_use block (the model "thinks aloud" before calling a tool).
func ToolUseWithTextResponse(text, toolID, toolName string, input json.RawMessage, seqNum int) *api.MessageResponse {
	return &api.MessageResponse{
		ID:         fmt.Sprintf("msg_mock_%d", seqNum),
		Type:       "message",
		Role:       api.RoleAssistant,
		Model:      api.ModelClaude46Sonnet,
		StopReason: api.StopReasonToolUse,
		Content: []api.ContentBlock{
			{Type: api.ContentTypeText, Text: text},
			{
				Type:  api.ContentTypeToolUse,
				ID:    toolID,
				Name:  toolName,
				Input: input,
			},
		},
		Usage: api.Usage{InputTokens: 10, OutputTokens: 40},
	}
}

// MultiToolUseResponse creates a response that requests multiple tool calls.
func MultiToolUseResponse(calls []ToolCall, seqNum int) *api.MessageResponse {
	blocks := make([]api.ContentBlock, len(calls))
	for i, call := range calls {
		blocks[i] = api.ContentBlock{
			Type:  api.ContentTypeToolUse,
			ID:    call.ID,
			Name:  call.Name,
			Input: call.Input,
		}
	}
	return &api.MessageResponse{
		ID:         fmt.Sprintf("msg_mock_%d", seqNum),
		Type:       "message",
		Role:       api.RoleAssistant,
		Model:      api.ModelClaude46Sonnet,
		StopReason: api.StopReasonToolUse,
		Content:    blocks,
		Usage:      api.Usage{InputTokens: 10, OutputTokens: 50},
	}
}

// ToolCall describes a single tool invocation for MultiToolUseResponse.
type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}
