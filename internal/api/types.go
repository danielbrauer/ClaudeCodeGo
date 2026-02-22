// Package api implements the Claude Messages API client.
package api

import "encoding/json"

// Model identifiers.
const (
	ModelClaude4Opus   = "claude-opus-4-20250514"
	ModelClaude4Sonnet = "claude-sonnet-4-20250514"
	ModelClaude35Haiku = "claude-3-5-haiku-20241022"
)

// Friendly model name mapping.
var ModelAliases = map[string]string{
	"opus":   ModelClaude4Opus,
	"sonnet": ModelClaude4Sonnet,
	"haiku":  ModelClaude35Haiku,
}

// ModelOption describes a model available for selection.
type ModelOption struct {
	Alias       string // short name: "opus", "sonnet", "haiku"
	ID          string // full model ID
	DisplayName string // human-readable: "Opus 4", "Sonnet 4", "Haiku 3.5"
	Description string // brief capability note
}

// AvailableModels is the ordered list of models shown in the /model picker.
var AvailableModels = []ModelOption{
	{Alias: "sonnet", ID: ModelClaude4Sonnet, DisplayName: "Sonnet 4", Description: "Best for everyday tasks (default)"},
	{Alias: "opus", ID: ModelClaude4Opus, DisplayName: "Opus 4", Description: "Most capable for complex work"},
	{Alias: "haiku", ID: ModelClaude35Haiku, DisplayName: "Haiku 3.5", Description: "Fastest for quick answers"},
}

// ModelDisplayName returns a friendly display name for a model ID or alias.
func ModelDisplayName(model string) string {
	for _, opt := range AvailableModels {
		if model == opt.ID || model == opt.Alias {
			return opt.DisplayName
		}
	}
	return model
}

// ResolveModelAlias resolves a model alias to its full ID. If the input
// is not a known alias, it is returned as-is (assumed to be a full model ID).
func ResolveModelAlias(input string) string {
	if resolved, ok := ModelAliases[input]; ok {
		return resolved
	}
	return input
}

// Role constants for messages.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Content block type constants.
const (
	ContentTypeText      = "text"
	ContentTypeImage     = "image"
	ContentTypeToolUse   = "tool_use"
	ContentTypeToolResult = "tool_result"
)

// Stop reason constants.
const (
	StopReasonEndTurn   = "end_turn"
	StopReasonToolUse   = "tool_use"
	StopReasonMaxTokens = "max_tokens"
	StopReasonStopSeq   = "stop_sequence"
)

// CreateMessageRequest is the request body for POST /v1/messages.
type CreateMessageRequest struct {
	Model     string            `json:"model"`
	MaxTokens int               `json:"max_tokens"`
	Messages  []Message         `json:"messages"`
	System    []SystemBlock     `json:"system,omitempty"`
	Tools     []ToolDefinition  `json:"tools,omitempty"`
	Stream    bool              `json:"stream,omitempty"`
	Metadata  *RequestMetadata  `json:"metadata,omitempty"`
	StopSeqs  []string          `json:"stop_sequences,omitempty"`
	Temp      *float64          `json:"temperature,omitempty"`
	TopP      *float64          `json:"top_p,omitempty"`
	TopK      *int              `json:"top_k,omitempty"`
}

// RequestMetadata holds metadata sent with API requests.
type RequestMetadata struct {
	UserID string `json:"user_id,omitempty"`
}

// SystemBlock is a system prompt block (text or cache control).
type SystemBlock struct {
	Type         string        `json:"type"`
	Text         string        `json:"text,omitempty"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

// CacheControl instructs the API to cache certain content.
type CacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// Message is a single conversation message.
type Message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string or []ContentBlock
}

// NewTextMessage creates a simple text message.
func NewTextMessage(role, text string) Message {
	content, _ := json.Marshal(text)
	return Message{Role: role, Content: content}
}

// NewBlockMessage creates a message with content blocks.
func NewBlockMessage(role string, blocks []ContentBlock) Message {
	content, _ := json.Marshal(blocks)
	return Message{Role: role, Content: content}
}

// ContentBlock is a union type for text, image, tool_use, and tool_result blocks.
type ContentBlock struct {
	Type string `json:"type"`

	// Text block fields.
	Text string `json:"text,omitempty"`

	// Image block fields.
	Source *ImageSource `json:"source,omitempty"`

	// Tool use block fields.
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// Tool result block fields.
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"` // string or []ContentBlock
	IsError   bool            `json:"is_error,omitempty"`

	// Cache control for any block.
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

// ImageSource holds image data for image content blocks.
type ImageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // e.g. "image/png"
	Data      string `json:"data"`
}

// ToolDefinition is sent to the API to describe an available tool.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// MessageResponse is the full (non-streaming) response from the Messages API.
type MessageResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"` // "message"
	Role         string         `json:"role"` // "assistant"
	Content      []ContentBlock `json:"content"`
	Model        string         `json:"model"`
	StopReason   string         `json:"stop_reason"`
	StopSequence *string        `json:"stop_sequence"`
	Usage        Usage          `json:"usage"`
}

// Usage tracks token consumption.
type Usage struct {
	InputTokens              int  `json:"input_tokens"`
	OutputTokens             int  `json:"output_tokens"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens,omitempty"`
}

// APIError represents an error response from the API.
type APIError struct {
	Type    string        `json:"type"`
	Error   APIErrorBody  `json:"error"`
}

// APIErrorBody is the error detail.
type APIErrorBody struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}
