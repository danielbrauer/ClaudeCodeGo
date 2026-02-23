// Package tools implements the built-in tool set for the Claude Code CLI.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/config"
)

// Tool is the interface that all built-in tools implement.
type Tool interface {
	// Name returns the tool name as sent to the API (e.g. "Bash", "FileRead").
	Name() string

	// Description returns a human-readable description for the API.
	Description() string

	// InputSchema returns the JSON Schema for the tool's input parameters.
	InputSchema() json.RawMessage

	// Execute runs the tool with the given JSON input and returns the text result.
	Execute(ctx context.Context, input json.RawMessage) (string, error)

	// RequiresPermission returns true if this tool call needs user approval.
	RequiresPermission(input json.RawMessage) bool
}

// PermissionHandler prompts the user for tool execution permission.
type PermissionHandler interface {
	// RequestPermission asks the user whether to allow a tool call.
	// It returns true if the user approves.
	RequestPermission(ctx context.Context, toolName string, input json.RawMessage) (bool, error)
}

// RichPermissionHandler is an extended permission handler that returns
// detailed permission results including decision reasons and suggestions.
// If the handler implements this interface, the registry will use it for
// richer permission checking.
type RichPermissionHandler interface {
	PermissionHandler
	// CheckPermission evaluates permission rules and returns a rich result.
	CheckPermission(toolName string, input json.RawMessage) config.PermissionResult
}

// PermissionContextProvider gives access to the session-level permission context.
type PermissionContextProvider interface {
	GetPermissionContext() *config.ToolPermissionContext
}

// Registry holds registered tools and dispatches execution.
// It implements conversation.ToolExecutor.
type Registry struct {
	mu         sync.RWMutex
	tools      map[string]Tool
	order      []string // preserves registration order
	permission PermissionHandler
}

// NewRegistry creates a new tool registry.
func NewRegistry(permission PermissionHandler) *Registry {
	return &Registry{
		tools:      make(map[string]Tool),
		permission: permission,
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := t.Name()
	if _, exists := r.tools[name]; !exists {
		r.order = append(r.order, name)
	}
	r.tools[name] = t
}

// HasTool returns true if the named tool is registered.
func (r *Registry) HasTool(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.tools[name]
	return ok
}

// Execute runs the named tool with the given JSON input.
// It checks permissions before execution if required.
func (r *Registry) Execute(ctx context.Context, name string, input []byte) (string, error) {
	r.mu.RLock()
	tool, ok := r.tools[name]
	perm := r.permission
	r.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}

	rawInput := json.RawMessage(input)

	// Check permission if needed.
	if tool.RequiresPermission(rawInput) && perm != nil {
		// Try rich permission check first.
		if rph, ok := perm.(RichPermissionHandler); ok {
			result := rph.CheckPermission(name, rawInput)
			switch result.Behavior {
			case config.BehaviorAllow:
				// Permission granted by rules — proceed.
			case config.BehaviorDeny:
				msg := "Permission denied."
				if result.Message != "" {
					msg = result.Message
				}
				return msg, fmt.Errorf("permission denied")
			default:
				// BehaviorAsk or BehaviorPassthrough — fall back to interactive prompt.
				allowed, err := perm.RequestPermission(ctx, name, rawInput)
				if err != nil {
					return "", fmt.Errorf("permission check: %w", err)
				}
				if !allowed {
					return "Permission denied by user.", fmt.Errorf("permission denied")
				}
			}
		} else {
			// Simple permission handler.
			allowed, err := perm.RequestPermission(ctx, name, rawInput)
			if err != nil {
				return "", fmt.Errorf("permission check: %w", err)
			}
			if !allowed {
				return "Permission denied by user.", fmt.Errorf("permission denied")
			}
		}
	}

	result, err := tool.Execute(ctx, rawInput)
	if err != nil {
		return result, err
	}
	return result, nil
}

// LastPermissionResult returns the most recent rich permission result for
// a tool execution, if the handler supports it. Returns nil otherwise.
func (r *Registry) LastPermissionResult(name string, input json.RawMessage) *config.PermissionResult {
	r.mu.RLock()
	perm := r.permission
	r.mu.RUnlock()

	if rph, ok := perm.(RichPermissionHandler); ok {
		result := rph.CheckPermission(name, input)
		return &result
	}
	return nil
}

// GetPermissionContext returns the session-level permission context, if
// the handler supports it.
func (r *Registry) GetPermissionContext() *config.ToolPermissionContext {
	r.mu.RLock()
	perm := r.permission
	r.mu.RUnlock()

	if pcp, ok := perm.(PermissionContextProvider); ok {
		return pcp.GetPermissionContext()
	}
	return nil
}

// SetPermissionHandler replaces the permission handler at runtime.
// The argument is interface{} to avoid import cycles with the tui package;
// it must implement PermissionHandler.
func (r *Registry) SetPermissionHandler(h interface{}) {
	if ph, ok := h.(PermissionHandler); ok {
		r.mu.Lock()
		r.permission = ph
		r.mu.Unlock()
	}
}

// Definitions returns API tool definitions for all registered tools,
// in registration order.
func (r *Registry) Definitions() []api.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]api.ToolDefinition, 0, len(r.order))
	for _, name := range r.order {
		t := r.tools[name]
		defs = append(defs, api.ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.InputSchema(),
		})
	}
	return defs
}
