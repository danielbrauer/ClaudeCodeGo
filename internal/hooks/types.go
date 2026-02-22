// Package hooks implements lifecycle event hooks for the Claude Code CLI.
//
// Hooks fire at specific points in the agentic loop (PreToolUse, PostToolUse,
// UserPromptSubmit, SessionStart, Stop, PermissionRequest) and can run shell
// commands, inject prompts, or spawn sub-agents.
package hooks

// Event constants for hook lifecycle events.
const (
	EventPreToolUse        = "PreToolUse"
	EventPostToolUse       = "PostToolUse"
	EventUserPromptSubmit  = "UserPromptSubmit"
	EventSessionStart      = "SessionStart"
	EventPermissionRequest = "PermissionRequest"
	EventStop              = "Stop"
)

// HookConfig holds all hook definitions keyed by event type.
// Parsed from the "hooks" field in settings.json.
type HookConfig struct {
	PreToolUse        []HookDef `json:"PreToolUse,omitempty"`
	PostToolUse       []HookDef `json:"PostToolUse,omitempty"`
	UserPromptSubmit  []HookDef `json:"UserPromptSubmit,omitempty"`
	SessionStart      []HookDef `json:"SessionStart,omitempty"`
	PermissionRequest []HookDef `json:"PermissionRequest,omitempty"`
	Stop              []HookDef `json:"Stop,omitempty"`
}

// HookDef defines a single hook action.
type HookDef struct {
	Type    string `json:"type"`              // "command", "prompt", "agent"
	Command string `json:"command,omitempty"` // shell command (type=command)
	Prompt  string `json:"prompt,omitempty"`  // prompt text (type=prompt)
}

// HookResult is the outcome of a hook execution.
type HookResult struct {
	Output string // stdout from the hook command
	Error  error  // non-nil if the hook failed or blocked
}
