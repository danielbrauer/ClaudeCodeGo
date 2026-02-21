package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/anthropics/claude-code-go/internal/api"
)

// ToolExecutor executes tool calls and returns results.
type ToolExecutor interface {
	Execute(ctx context.Context, name string, input []byte) (string, error)
	HasTool(name string) bool
}

// Loop is the main agentic conversation loop.
type Loop struct {
	client   *api.Client
	history  *History
	system   []api.SystemBlock
	tools    []api.ToolDefinition
	toolExec ToolExecutor
	handler  api.StreamHandler
}

// LoopConfig configures the agentic loop.
type LoopConfig struct {
	Client   *api.Client
	System   []api.SystemBlock
	Tools    []api.ToolDefinition
	ToolExec ToolExecutor
	Handler  api.StreamHandler
}

// NewLoop creates a new agentic conversation loop.
func NewLoop(cfg LoopConfig) *Loop {
	return &Loop{
		client:   cfg.Client,
		history:  NewHistory(),
		system:   cfg.System,
		tools:    cfg.Tools,
		toolExec: cfg.ToolExec,
		handler:  cfg.Handler,
	}
}

// SendMessage sends a user message and runs the agentic loop until the
// assistant produces a final text response (stop_reason = "end_turn").
func (l *Loop) SendMessage(ctx context.Context, userMessage string) error {
	l.history.AddUserMessage(userMessage)
	return l.run(ctx)
}

func (l *Loop) run(ctx context.Context) error {
	for {
		req := &api.CreateMessageRequest{
			Messages: l.history.Messages(),
			System:   l.system,
			Tools:    l.tools,
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

		// Check if we need to execute tools.
		if resp.StopReason != api.StopReasonToolUse {
			// No tool calls - conversation turn is done.
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

			output, execErr := l.toolExec.Execute(ctx, block.Name, block.Input)
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
		// Loop back to call API again with tool results.
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
	}
	return ""
}
