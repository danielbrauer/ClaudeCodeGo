// Package tools implements the built-in tool set for the Claude Code CLI.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/anthropics/claude-code-go/internal/api"
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
	r.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}

	rawInput := json.RawMessage(input)

	// Check permission if needed.
	if tool.RequiresPermission(rawInput) && r.permission != nil {
		allowed, err := r.permission.RequestPermission(ctx, name, rawInput)
		if err != nil {
			return "", fmt.Errorf("permission check: %w", err)
		}
		if !allowed {
			return "Permission denied by user.", fmt.Errorf("permission denied")
		}
	}

	result, err := tool.Execute(ctx, rawInput)
	if err != nil {
		return result, err
	}
	return result, nil
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
