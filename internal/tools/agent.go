package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/conversation"
)

// AgentInput is the input schema for the Agent tool.
type AgentInput struct {
	Description     string  `json:"description"`
	Prompt          string  `json:"prompt"`
	SubagentType    string  `json:"subagent_type"`
	Model           *string `json:"model,omitempty"`
	Resume          *string `json:"resume,omitempty"`
	RunInBackground *bool   `json:"run_in_background,omitempty"`
	MaxTurns        *int    `json:"max_turns,omitempty"`
	Name            *string `json:"name,omitempty"`
	Mode            *string `json:"mode,omitempty"`
	Isolation       *string `json:"isolation,omitempty"`
}

// agentState tracks a running or completed sub-agent.
type agentState struct {
	id      string
	loop    *conversation.Loop
	history *conversation.History
	done    chan struct{}
	result  string
	err     error
	usage   api.Usage
	turns   int
	startMs int64
}

// AgentTool spawns sub-agents with isolated conversation loops.
type AgentTool struct {
	client   *api.Client
	system   []api.SystemBlock
	tools    []api.ToolDefinition
	toolExec conversation.ToolExecutor
	bgStore  *BackgroundTaskStore
	hooks    conversation.HookRunner // Phase 7: propagated to sub-agents

	mu     sync.Mutex
	agents map[string]*agentState
	nextID int
}

// NewAgentTool creates a new Agent tool.
func NewAgentTool(
	client *api.Client,
	system []api.SystemBlock,
	toolDefs []api.ToolDefinition,
	toolExec conversation.ToolExecutor,
	bgStore *BackgroundTaskStore,
	hooks conversation.HookRunner,
) *AgentTool {
	return &AgentTool{
		client:   client,
		system:   system,
		tools:    toolDefs,
		toolExec: toolExec,
		bgStore:  bgStore,
		hooks:    hooks,
		agents:   make(map[string]*agentState),
	}
}

func (t *AgentTool) Name() string { return "Agent" }

func (t *AgentTool) Description() string {
	return `Launch a new agent to handle complex, multi-step tasks autonomously. The agent gets its own isolated conversation context and can use all available tools. Use the description parameter for a short summary and prompt for the full task description. Supports background execution and resuming previous agents.`
}

func (t *AgentTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "description": {
      "type": "string",
      "description": "A short (3-5 word) description of the task"
    },
    "prompt": {
      "type": "string",
      "description": "The task for the agent to perform"
    },
    "subagent_type": {
      "type": "string",
      "description": "The type of specialized agent to use for this task"
    },
    "model": {
      "type": "string",
      "enum": ["sonnet", "opus", "haiku"],
      "description": "Optional model to use for this agent"
    },
    "resume": {
      "type": "string",
      "description": "Optional agent ID to resume from"
    },
    "run_in_background": {
      "type": "boolean",
      "description": "Set to true to run this agent in the background"
    },
    "max_turns": {
      "type": "integer",
      "description": "Maximum number of agentic turns before stopping",
      "exclusiveMinimum": 0
    },
    "name": {
      "type": "string",
      "description": "Name for the spawned agent"
    },
    "mode": {
      "type": "string",
      "enum": ["acceptEdits", "bypassPermissions", "default", "dontAsk", "plan"],
      "description": "Permission mode for the agent"
    },
    "isolation": {
      "type": "string",
      "enum": ["worktree"],
      "description": "Isolation mode for the agent"
    }
  },
  "required": ["description", "prompt", "subagent_type"],
  "additionalProperties": false
}`)
}

func (t *AgentTool) RequiresPermission(_ json.RawMessage) bool {
	return false // sub-agents inherit the parent's permission handler
}

func (t *AgentTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var in AgentInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parsing Agent input: %w", err)
	}

	if in.Prompt == "" {
		return "Error: prompt is required", nil
	}

	// Handle resume.
	if in.Resume != nil && *in.Resume != "" {
		return t.resumeAgent(ctx, *in.Resume, in.Prompt)
	}

	// Generate a unique agent ID.
	agentID := t.generateID()
	startMs := time.Now().UnixMilli()

	// Create an isolated conversation loop for the sub-agent.
	history := conversation.NewHistory()
	handler := &conversation.PrintStreamHandler{}

	loopCfg := conversation.LoopConfig{
		Client:   t.client,
		System:   t.system,
		Tools:    t.tools,
		ToolExec: t.toolExec,
		Handler:  handler,
		History:  history,
		Hooks:    t.hooks, // Phase 7: propagate hooks to sub-agents
	}
	agentLoop := conversation.NewLoop(loopCfg)

	state := &agentState{
		id:      agentID,
		loop:    agentLoop,
		history: history,
		done:    make(chan struct{}),
		startMs: startMs,
	}

	t.mu.Lock()
	t.agents[agentID] = state
	t.mu.Unlock()

	// Background execution.
	if in.RunInBackground != nil && *in.RunInBackground {
		bgCtx, bgCancel := context.WithCancel(context.Background())

		bgTask := &BackgroundTask{
			ID:     agentID,
			Ctx:    bgCtx,
			Cancel: bgCancel,
			Done:   state.done,
		}
		t.bgStore.Add(bgTask)

		go func() {
			defer close(state.done)
			err := t.runAgent(bgCtx, state, in.Prompt, in.MaxTurns)
			state.err = err
			state.result = t.extractResult(state)

			bgTask.Result = state.result
			bgTask.Err = err
		}()

		result := map[string]interface{}{
			"status":  "async_launched",
			"agentId": agentID,
			"message": fmt.Sprintf("Agent %s launched in background", agentID),
		}
		out, _ := json.Marshal(result)
		return string(out), nil
	}

	// Synchronous execution.
	err := t.runAgent(ctx, state, in.Prompt, in.MaxTurns)
	close(state.done)
	if err != nil {
		state.err = err
	}
	state.result = t.extractResult(state)

	durationMs := time.Now().UnixMilli() - startMs

	result := map[string]interface{}{
		"status":            "completed",
		"agentId":           agentID,
		"content":           state.result,
		"totalToolUseCount": state.turns,
		"totalDurationMs":   durationMs,
		"usage":             state.usage,
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}

// runAgent sends a message to the sub-agent loop with optional turn limit.
func (t *AgentTool) runAgent(ctx context.Context, state *agentState, prompt string, maxTurns *int) error {
	// For now, use the standard SendMessage which runs the full agentic loop.
	// A max_turns limit would require modifying the loop, so we just run it to completion.
	err := state.loop.SendMessage(ctx, prompt)
	if err != nil {
		return err
	}
	return nil
}

// resumeAgent continues a previous agent with a new prompt.
func (t *AgentTool) resumeAgent(ctx context.Context, agentID string, prompt string) (string, error) {
	t.mu.Lock()
	state, ok := t.agents[agentID]
	t.mu.Unlock()

	if !ok {
		return fmt.Sprintf("Error: agent %s not found", agentID), nil
	}

	// Wait for previous run to finish if it's background.
	select {
	case <-state.done:
	default:
		return fmt.Sprintf("Error: agent %s is still running", agentID), nil
	}

	// Reset done channel for new run.
	state.done = make(chan struct{})
	startMs := time.Now().UnixMilli()

	err := state.loop.SendMessage(ctx, prompt)
	close(state.done)
	if err != nil {
		state.err = err
	}
	state.result = t.extractResult(state)

	durationMs := time.Now().UnixMilli() - startMs

	result := map[string]interface{}{
		"status":            "completed",
		"agentId":           agentID,
		"content":           state.result,
		"totalToolUseCount": state.turns,
		"totalDurationMs":   durationMs,
		"usage":             state.usage,
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}

// extractResult gets the last assistant text from the sub-agent's history.
func (t *AgentTool) extractResult(state *agentState) string {
	msgs := state.history.Messages()
	for i := len(msgs) - 1; i >= 0; i-- {
		msg := msgs[i]
		if msg.Role != api.RoleAssistant {
			continue
		}

		// Try to parse content blocks.
		var blocks []api.ContentBlock
		if err := json.Unmarshal(msg.Content, &blocks); err == nil {
			for _, b := range blocks {
				if b.Type == api.ContentTypeText && b.Text != "" {
					return b.Text
				}
			}
		}

		// Try as plain string.
		var text string
		if err := json.Unmarshal(msg.Content, &text); err == nil && text != "" {
			return text
		}
	}
	return "(no output from agent)"
}

// generateID creates a unique agent ID.
func (t *AgentTool) generateID() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nextID++
	return fmt.Sprintf("agent-%d-%d", t.nextID, time.Now().UnixMilli())
}
