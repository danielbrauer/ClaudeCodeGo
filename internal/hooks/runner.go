package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/anthropics/claude-code-go/internal/conversation"
)

// Runner executes hooks based on a HookConfig.
// It implements conversation.HookRunner.
type Runner struct {
	config             HookConfig
	pendingInjections  []string // prompt hook content awaiting injection
}

// NewRunner creates a new hook runner from the given config.
func NewRunner(config HookConfig) *Runner {
	return &Runner{config: config}
}

// RunPreToolUse fires all PreToolUse hooks. Returns an error if any hook
// blocks the tool execution (non-zero exit code).
func (r *Runner) RunPreToolUse(ctx context.Context, toolName string, input json.RawMessage) error {
	if len(r.config.PreToolUse) == 0 {
		return nil
	}

	env := []string{
		"HOOK_EVENT=PreToolUse",
		"TOOL_NAME=" + toolName,
		"TOOL_INPUT=" + string(input),
	}

	for _, hook := range r.config.PreToolUse {
		result := r.executeHook(ctx, hook, env)
		if result.Error != nil {
			return fmt.Errorf("PreToolUse hook blocked: %w", result.Error)
		}
		// Collect prompt injections for the conversation.
		if result.PromptInject != "" {
			r.pendingInjections = append(r.pendingInjections, result.PromptInject)
		}
	}
	return nil
}

// PendingInjections returns and clears any prompt content from prompt hooks.
func (r *Runner) PendingInjections() []string {
	if len(r.pendingInjections) == 0 {
		return nil
	}
	result := r.pendingInjections
	r.pendingInjections = nil
	return result
}

// RunPostToolUse fires all PostToolUse hooks. Errors are logged but do not
// block execution.
func (r *Runner) RunPostToolUse(ctx context.Context, toolName string, input json.RawMessage, output string, isError bool) error {
	if len(r.config.PostToolUse) == 0 {
		return nil
	}

	isErrStr := "false"
	if isError {
		isErrStr = "true"
	}

	// Truncate output if very large to avoid env var size limits.
	truncatedOutput := output
	if len(truncatedOutput) > 10000 {
		truncatedOutput = truncatedOutput[:10000] + "...(truncated)"
	}

	env := []string{
		"HOOK_EVENT=PostToolUse",
		"TOOL_NAME=" + toolName,
		"TOOL_INPUT=" + string(input),
		"TOOL_OUTPUT=" + truncatedOutput,
		"TOOL_IS_ERROR=" + isErrStr,
	}

	for _, hook := range r.config.PostToolUse {
		result := r.executeHook(ctx, hook, env)
		if result.Error != nil {
			return result.Error
		}
	}
	return nil
}

// RunUserPromptSubmit fires all UserPromptSubmit hooks. A hook can modify
// or reject the user's message.
func (r *Runner) RunUserPromptSubmit(ctx context.Context, message string) (conversation.HookSubmitResult, error) {
	if len(r.config.UserPromptSubmit) == 0 {
		return conversation.HookSubmitResult{Message: message}, nil
	}

	env := []string{
		"HOOK_EVENT=UserPromptSubmit",
		"USER_MESSAGE=" + message,
	}

	currentMsg := message
	for _, hook := range r.config.UserPromptSubmit {
		result := r.executeHook(ctx, hook, env)
		if result.Error != nil {
			return conversation.HookSubmitResult{Block: true, Message: currentMsg}, result.Error
		}
		// Prompt hooks inject content.
		if result.PromptInject != "" {
			r.pendingInjections = append(r.pendingInjections, result.PromptInject)
			continue
		}
		// If the hook produced stdout, use it as the (possibly modified) message.
		if trimmed := strings.TrimSpace(result.Output); trimmed != "" {
			currentMsg = trimmed
		}
	}
	return conversation.HookSubmitResult{Message: currentMsg}, nil
}

// RunSessionStart fires all SessionStart hooks.
func (r *Runner) RunSessionStart(ctx context.Context) error {
	if len(r.config.SessionStart) == 0 {
		return nil
	}

	env := []string{
		"HOOK_EVENT=SessionStart",
	}

	for _, hook := range r.config.SessionStart {
		result := r.executeHook(ctx, hook, env)
		if result.Error != nil {
			return result.Error
		}
	}
	return nil
}

// RunStop fires all Stop hooks.
func (r *Runner) RunStop(ctx context.Context) error {
	if len(r.config.Stop) == 0 {
		return nil
	}

	env := []string{
		"HOOK_EVENT=Stop",
	}

	for _, hook := range r.config.Stop {
		result := r.executeHook(ctx, hook, env)
		if result.Error != nil {
			return result.Error
		}
	}
	return nil
}

// RunPermissionRequest fires all PermissionRequest hooks.
func (r *Runner) RunPermissionRequest(ctx context.Context, toolName string, input json.RawMessage) error {
	if len(r.config.PermissionRequest) == 0 {
		return nil
	}

	env := []string{
		"HOOK_EVENT=PermissionRequest",
		"TOOL_NAME=" + toolName,
		"TOOL_INPUT=" + string(input),
	}

	for _, hook := range r.config.PermissionRequest {
		result := r.executeHook(ctx, hook, env)
		if result.Error != nil {
			return result.Error
		}
	}
	return nil
}

// executeHook runs a single hook definition and returns the result.
func (r *Runner) executeHook(ctx context.Context, hook HookDef, extraEnv []string) HookResult {
	switch hook.Type {
	case "command":
		return r.runCommand(ctx, hook.Command, extraEnv)
	case "prompt":
		// Prompt hooks inject additional context into the conversation.
		return HookResult{Output: hook.Prompt, PromptInject: hook.Prompt}
	case "agent":
		// Agent hooks spawn a sub-process. For now, treat as a command.
		return r.runCommand(ctx, hook.Command, extraEnv)
	default:
		return HookResult{Error: fmt.Errorf("unknown hook type: %s", hook.Type)}
	}
}

// runCommand executes a shell command with the given extra environment variables.
func (r *Runner) runCommand(ctx context.Context, command string, extraEnv []string) HookResult {
	if command == "" {
		return HookResult{}
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Env = append(os.Environ(), extraEnv...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		return HookResult{
			Output: stdout.String(),
			Error:  fmt.Errorf("%s", strings.TrimSpace(errMsg)),
		}
	}

	return HookResult{Output: stdout.String()}
}
