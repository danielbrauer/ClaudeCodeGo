package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/config"
)

// ToolExecutor executes tool calls and returns results.
type ToolExecutor interface {
	Execute(ctx context.Context, name string, input []byte) (string, error)
	HasTool(name string) bool
}

// HookRunner fires lifecycle hooks at various points in the agentic loop.
// A nil HookRunner means no hooks are configured.
type HookRunner interface {
	RunPreToolUse(ctx context.Context, toolName string, input json.RawMessage) error
	RunPostToolUse(ctx context.Context, toolName string, input json.RawMessage, output string, isError bool) error
	RunUserPromptSubmit(ctx context.Context, message string) (HookSubmitResult, error)
	RunSessionStart(ctx context.Context) error
	RunStop(ctx context.Context) error
	RunPermissionRequest(ctx context.Context, toolName string, input json.RawMessage) error
}

// HookSubmitResult is the outcome of a UserPromptSubmit hook.
type HookSubmitResult struct {
	Block   bool   // true = reject the message
	Message string // possibly modified message
}

// Loop is the main agentic conversation loop.
type Loop struct {
	client         *api.Client
	history        *History
	system         []api.SystemBlock
	tools          []api.ToolDefinition
	toolExec       ToolExecutor
	handler        api.StreamHandler
	compactor      *Compactor
	onTurnComplete func(history *History)
	hooks          HookRunner // Phase 7: nil = no hooks
	fastMode       bool       // when true, sends speed:"fast" on eligible models
	contextMessage string     // <system-reminder> context prepended to messages
}

// LoopConfig configures the agentic loop.
type LoopConfig struct {
	Client         *api.Client
	System         []api.SystemBlock
	Tools          []api.ToolDefinition
	ToolExec       ToolExecutor
	Handler        api.StreamHandler
	History        *History               // if non-nil, resume from this history
	Compactor      *Compactor             // if non-nil, enables auto-compaction
	OnTurnComplete func(history *History)  // called after each API round-trip
	Hooks          HookRunner             // Phase 7: nil = no hooks
	ContextMessage string                 // <system-reminder> context prepended to messages
}

// NewLoop creates a new agentic conversation loop.
func NewLoop(cfg LoopConfig) *Loop {
	history := cfg.History
	if history == nil {
		history = NewHistory()
	}
	return &Loop{
		client:         cfg.Client,
		history:        history,
		system:         cfg.System,
		tools:          cfg.Tools,
		toolExec:       cfg.ToolExec,
		handler:        cfg.Handler,
		compactor:      cfg.Compactor,
		onTurnComplete: cfg.OnTurnComplete,
		hooks:          cfg.Hooks,
		contextMessage: cfg.ContextMessage,
	}
}

// History returns the loop's conversation history.
func (l *Loop) History() *History {
	return l.history
}

// SetHandler replaces the stream handler. This allows the TUI to inject
// its own handler after the loop is created.
func (l *Loop) SetHandler(h api.StreamHandler) {
	l.handler = h
}

// SetModel changes the model used for subsequent API calls.
func (l *Loop) SetModel(model string) {
	l.client.SetModel(model)
}

// FastMode returns whether fast mode is enabled.
func (l *Loop) FastMode() bool {
	return l.fastMode
}

// SetFastMode enables or disables fast mode.
func (l *Loop) SetFastMode(on bool) {
	l.fastMode = on
}

// SetPermissionHandler replaces the permission handler on the tool executor.
// This is a no-op if the executor doesn't support it.
func (l *Loop) SetPermissionHandler(h interface{}) {
	type permSetter interface {
		SetPermissionHandler(h interface{})
	}
	if ps, ok := l.toolExec.(permSetter); ok {
		ps.SetPermissionHandler(h)
	}
}

// GetPermissionContext returns the session-level permission context from the
// tool executor, if it supports it. Returns nil otherwise.
func (l *Loop) GetPermissionContext() *config.ToolPermissionContext {
	type permCtxGetter interface {
		GetPermissionContext() *config.ToolPermissionContext
	}
	if pg, ok := l.toolExec.(permCtxGetter); ok {
		return pg.GetPermissionContext()
	}
	return nil
}

// SendMessage sends a user message and runs the agentic loop until the
// assistant produces a final text response (stop_reason = "end_turn").
func (l *Loop) SendMessage(ctx context.Context, userMessage string) error {
	// Phase 7: UserPromptSubmit hook.
	if l.hooks != nil {
		result, err := l.hooks.RunUserPromptSubmit(ctx, userMessage)
		if err != nil {
			return fmt.Errorf("UserPromptSubmit hook: %w", err)
		}
		if result.Block {
			return nil // hook rejected the message
		}
		userMessage = result.Message // hook may modify the message
	}
	l.history.AddUserMessage(userMessage)
	return l.run(ctx)
}

// Compact triggers manual context compaction.
func (l *Loop) Compact(ctx context.Context) error {
	if l.compactor == nil {
		return fmt.Errorf("compaction not configured")
	}
	return l.compactor.Compact(ctx, l.history)
}

// Clear resets the conversation history to empty, starting a fresh conversation.
func (l *Loop) Clear() {
	l.history.SetMessages(nil)
}

// SetOnTurnComplete replaces the turn-complete callback. This is used by
// /clear to point the callback at the new session after clearing.
func (l *Loop) SetOnTurnComplete(fn func(history *History)) {
	l.onTurnComplete = fn
}

func (l *Loop) run(ctx context.Context) error {
	for {
		msgs := l.history.Messages()

		// Prepend context message if configured (matching JS CLI's TN1 pattern).
		// The context message is a user message containing <system-reminder>
		// blocks with claudeMd, currentDate, and gitStatus.
		if l.contextMessage != "" {
			contextMsg := api.NewTextMessage(api.RoleUser, l.contextMessage)
			msgs = append([]api.Message{contextMsg}, msgs...)
		}

		system := l.system
		tools := l.tools

		// Apply prompt caching if enabled for the current model.
		// This adds cache_control breakpoints to system blocks, tool
		// definitions, and the last ~2 conversation messages so the API
		// can serve cached prefixes instead of reprocessing everything.
		if IsCachingEnabled(l.client.Model()) {
			system = WithSystemPromptCaching(system)
			tools = WithToolsCaching(tools)
			msgs = WithMessageCaching(msgs)
		}

		req := &api.CreateMessageRequest{
			Messages: msgs,
			System:   system,
			Tools:    tools,
		}

		// Apply fast mode: add speed:"fast" when enabled on an eligible model.
		if l.fastMode && api.IsOpus46Model(l.client.Model()) {
			req.Speed = "fast"
		}

		resp, err := l.client.CreateMessageStream(ctx, req, l.handler)
		if err != nil {
			return fmt.Errorf("API call: %w", err)
		}

		if resp == nil {
			return fmt.Errorf("no response received")
		}

		// Add assistant response to history.
		l.history.AddAssistantResponse(resp.Content)

		// Check for auto-compaction after each API response.
		if l.compactor != nil && l.compactor.ShouldCompact(resp.Usage) {
			if err := l.compactor.Compact(ctx, l.history); err != nil {
				// Log but don't fail the loop.
				log.Printf("Warning: compaction failed: %v", err)
			}
		}

		// Check if we need to execute tools.
		if resp.StopReason != api.StopReasonToolUse {
			// Phase 7: Stop hook.
			if l.hooks != nil {
				_ = l.hooks.RunStop(ctx)
			}
			// No tool calls - conversation turn is done.
			l.notifyTurnComplete()
			return nil
		}

		// Execute tool calls and collect results.
		var toolResults []api.ContentBlock
		for _, block := range resp.Content {
			if block.Type != api.ContentTypeToolUse {
				continue
			}

			if l.toolExec == nil || !l.toolExec.HasTool(block.Name) {
				result := MakeToolResult(block.ID,
					fmt.Sprintf("Tool %q is not available.", block.Name), true)
				toolResults = append(toolResults, result)
				continue
			}

			// Phase 7: PreToolUse hook.
			if l.hooks != nil {
				if err := l.hooks.RunPreToolUse(ctx, block.Name, block.Input); err != nil {
					result := MakeToolResult(block.ID,
						fmt.Sprintf("Hook blocked tool execution: %v", err), true)
					toolResults = append(toolResults, result)
					continue
				}
			}

			output, execErr := l.toolExec.Execute(ctx, block.Name, block.Input)

			// Phase 7: PostToolUse hook.
			if l.hooks != nil {
				_ = l.hooks.RunPostToolUse(ctx, block.Name, block.Input, output, execErr != nil)
			}

			if execErr != nil {
				// If tool returned output along with an error, use the output.
				msg := output
				if msg == "" {
					msg = fmt.Sprintf("Error executing tool: %v", execErr)
				}
				result := MakeToolResult(block.ID, msg, true)
				toolResults = append(toolResults, result)
			} else {
				result := MakeToolResult(block.ID, output, false)
				toolResults = append(toolResults, result)
			}
		}

		if len(toolResults) == 0 {
			// Stop reason was tool_use but no tool blocks found - shouldn't happen.
			return fmt.Errorf("stop_reason was tool_use but no tool_use blocks found")
		}

		l.history.AddToolResults(toolResults)
		l.notifyTurnComplete()
		// Loop back to call API again with tool results.
	}
}

func (l *Loop) notifyTurnComplete() {
	if l.onTurnComplete != nil {
		l.onTurnComplete(l.history)
	}
}

// PrintStreamHandler is a basic StreamHandler that prints text to stdout.
type PrintStreamHandler struct{}

func (h *PrintStreamHandler) OnMessageStart(msg api.MessageResponse) {}

func (h *PrintStreamHandler) OnContentBlockStart(index int, block api.ContentBlock) {}

func (h *PrintStreamHandler) OnTextDelta(index int, text string) {
	fmt.Print(text)
}

func (h *PrintStreamHandler) OnInputJSONDelta(index int, partialJSON string) {}

func (h *PrintStreamHandler) OnContentBlockStop(index int) {}

func (h *PrintStreamHandler) OnMessageDelta(delta api.MessageDeltaBody, usage *api.Usage) {}

func (h *PrintStreamHandler) OnMessageStop() {
	fmt.Println()
}

func (h *PrintStreamHandler) OnError(err error) {
	fmt.Fprintf(os.Stderr, "\nStream error: %v\n", err)
}

// ToolAwareStreamHandler extends PrintStreamHandler with tool call display.
// It accumulates tool input JSON from deltas and shows a summary when the
// tool call block is complete.
type ToolAwareStreamHandler struct {
	toolNames map[int]string
	jsonBufs  map[int][]byte
}

func (h *ToolAwareStreamHandler) OnMessageStart(msg api.MessageResponse) {}

func (h *ToolAwareStreamHandler) OnContentBlockStart(index int, block api.ContentBlock) {
	if block.Type == api.ContentTypeToolUse {
		if h.toolNames == nil {
			h.toolNames = make(map[int]string)
			h.jsonBufs = make(map[int][]byte)
		}
		h.toolNames[index] = block.Name
		h.jsonBufs[index] = nil
	}
}

func (h *ToolAwareStreamHandler) OnTextDelta(index int, text string) {
	fmt.Print(text)
}

func (h *ToolAwareStreamHandler) OnInputJSONDelta(index int, partialJSON string) {
	if h.jsonBufs != nil {
		h.jsonBufs[index] = append(h.jsonBufs[index], []byte(partialJSON)...)
	}
}

func (h *ToolAwareStreamHandler) OnContentBlockStop(index int) {
	if name, ok := h.toolNames[index]; ok {
		assembled := json.RawMessage(h.jsonBufs[index])
		fmt.Printf("\n[tool: %s]", name)
		summary := toolInputSummary(name, assembled)
		if summary != "" {
			fmt.Printf(" %s", summary)
		}
		fmt.Println()
		delete(h.toolNames, index)
		delete(h.jsonBufs, index)
	}
}

func (h *ToolAwareStreamHandler) OnMessageDelta(delta api.MessageDeltaBody, usage *api.Usage) {
}

func (h *ToolAwareStreamHandler) OnMessageStop() {
	fmt.Println()
}

func (h *ToolAwareStreamHandler) OnError(err error) {
	fmt.Fprintf(os.Stderr, "\nStream error: %v\n", err)
}

// toolInputSummary produces a short description from assembled tool input JSON.
func toolInputSummary(name string, input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(input, &m); err != nil {
		return ""
	}

	extractString := func(key string) string {
		v, ok := m[key]
		if !ok {
			return ""
		}
		var s string
		json.Unmarshal(v, &s)
		return s
	}

	switch name {
	case "Bash":
		if s := extractString("command"); s != "" {
			if len(s) > 200 {
				s = s[:197] + "..."
			}
			return fmt.Sprintf("$ %s", s)
		}
	case "FileRead":
		if s := extractString("file_path"); s != "" {
			return s
		}
	case "FileEdit":
		if s := extractString("file_path"); s != "" {
			return s
		}
	case "FileWrite":
		if s := extractString("file_path"); s != "" {
			return s
		}
	case "Glob":
		if s := extractString("pattern"); s != "" {
			return s
		}
	case "Grep":
		if s := extractString("pattern"); s != "" {
			return fmt.Sprintf("/%s/", s)
		}
	case "Agent":
		if s := extractString("description"); s != "" {
			return s
		}
	case "TodoWrite":
		return "updating task list"
	case "AskUserQuestion":
		return "asking user"
	case "WebFetch":
		if s := extractString("url"); s != "" {
			return s
		}
	case "WebSearch":
		if s := extractString("query"); s != "" {
			return fmt.Sprintf("searching: %s", s)
		}
	case "NotebookEdit":
		if s := extractString("notebook_path"); s != "" {
			return s
		}
	case "ExitPlanMode":
		return "plan ready"
	case "Config":
		if s := extractString("setting"); s != "" {
			return s
		}
	case "EnterWorktree":
		return "creating worktree"
	case "TaskOutput":
		if s := extractString("task_id"); s != "" {
			return fmt.Sprintf("reading task %s", s)
		}
	case "TaskStop":
		return "stopping task"
	}
	return ""
}
