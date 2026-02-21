package conversation

import (
	"context"
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
	client      *api.Client
	history     *History
	system      []api.SystemBlock
	tools       []api.ToolDefinition
	toolExec    ToolExecutor
	handler     api.StreamHandler
}

// LoopConfig configures the agentic loop.
type LoopConfig struct {
	Client      *api.Client
	System      []api.SystemBlock
	Tools       []api.ToolDefinition
	ToolExec    ToolExecutor
	Handler     api.StreamHandler
}

// NewLoop creates a new agentic conversation loop.
func NewLoop(cfg LoopConfig) *Loop {
	return &Loop{
		client:  cfg.Client,
		history: NewHistory(),
		system:  cfg.System,
		tools:   cfg.Tools,
		toolExec: cfg.ToolExec,
		handler: cfg.Handler,
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

			output, err := l.toolExec.Execute(ctx, block.Name, block.Input)
			if err != nil {
				result := MakeToolResult(block.ID,
					fmt.Sprintf("Error executing tool: %v", err), true)
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
