package conversation

import (
	"context"
	"fmt"

	"github.com/anthropics/claude-code-go/internal/api"
)

// Default context window limits by model family.
const (
	// DefaultMaxInputTokens is the threshold at which compaction is triggered.
	// This is set conservatively below the actual context limit to leave room
	// for the next response.
	DefaultMaxInputTokens = 150_000

	// DefaultPreserveRecent is the number of recent messages to keep during compaction.
	DefaultPreserveRecent = 4
)

// Compactor handles context window management by summarizing older messages
// when the conversation approaches the token limit.
type Compactor struct {
	Client         *api.Client
	MaxInputTokens int // trigger threshold
	PreserveRecent int // number of recent messages to keep
}

// NewCompactor creates a compactor with the given settings.
func NewCompactor(client *api.Client) *Compactor {
	return &Compactor{
		Client:         client,
		MaxInputTokens: DefaultMaxInputTokens,
		PreserveRecent: DefaultPreserveRecent,
	}
}

// ShouldCompact returns true if the conversation should be compacted
// based on the token usage from the most recent API response.
func (c *Compactor) ShouldCompact(usage api.Usage) bool {
	return usage.InputTokens >= c.MaxInputTokens
}

// Compact summarizes older messages in the history, replacing them with a
// concise summary to free up context window space.
func (c *Compactor) Compact(ctx context.Context, history *History) error {
	msgs := history.Messages()
	if len(msgs) <= c.PreserveRecent {
		return nil // nothing to compact
	}

	// Split messages: older ones to summarize, recent ones to keep.
	splitPoint := len(msgs) - c.PreserveRecent
	if splitPoint <= 0 {
		return nil
	}

	olderMsgs := msgs[:splitPoint]

	// Build a summarization request.
	summary, err := c.summarize(ctx, olderMsgs)
	if err != nil {
		return fmt.Errorf("summarizing messages: %w", err)
	}

	// Replace the older messages with a summary message.
	summaryMsg := api.NewTextMessage(api.RoleUser, summary)
	history.ReplaceRange(0, splitPoint, []api.Message{summaryMsg})

	return nil
}

// summarize calls the API to generate a concise summary of the given messages.
func (c *Compactor) summarize(ctx context.Context, messages []api.Message) (string, error) {
	systemPrompt := []api.SystemBlock{
		{
			Type: "text",
			Text: `You are a conversation summarizer. Your job is to create a concise summary of the conversation so far that preserves all important context, decisions made, files modified, commands run, and their results. The summary should enable continuing the conversation without loss of critical information.

Be concise but thorough. Include:
- Key decisions and their rationale
- Files that were read, created, or modified (with paths)
- Important command outputs or errors
- Current state of any ongoing task
- Any constraints or requirements mentioned by the user`,
		},
	}

	// Create a user message asking for the summary.
	summaryRequest := api.NewTextMessage(api.RoleUser,
		"Please summarize the above conversation concisely, preserving all important context for continuation.")

	// Build messages: the conversation to summarize + the summary request.
	allMsgs := make([]api.Message, len(messages)+1)
	copy(allMsgs, messages)
	allMsgs[len(allMsgs)-1] = summaryRequest

	req := &api.CreateMessageRequest{
		Messages: allMsgs,
		System:   systemPrompt,
	}

	// Use a no-op handler since we just want the final response.
	resp, err := c.Client.CreateMessageStream(ctx, req, &noOpStreamHandler{})
	if err != nil {
		return "", fmt.Errorf("API call for summarization: %w", err)
	}

	if resp == nil || len(resp.Content) == 0 {
		return "", fmt.Errorf("empty summarization response")
	}

	// Extract text from the response.
	var summary string
	for _, block := range resp.Content {
		if block.Type == api.ContentTypeText {
			summary += block.Text
		}
	}

	if summary == "" {
		return "", fmt.Errorf("no text in summarization response")
	}

	return fmt.Sprintf("[Conversation Summary]\n%s", summary), nil
}

// noOpStreamHandler discards all streaming events (used for summarization calls).
type noOpStreamHandler struct{}

func (h *noOpStreamHandler) OnMessageStart(msg api.MessageResponse)                     {}
func (h *noOpStreamHandler) OnContentBlockStart(index int, block api.ContentBlock)       {}
func (h *noOpStreamHandler) OnTextDelta(index int, text string)                          {}
func (h *noOpStreamHandler) OnThinkingDelta(index int, thinking string)                  {}
func (h *noOpStreamHandler) OnSignatureDelta(index int, signature string)                {}
func (h *noOpStreamHandler) OnInputJSONDelta(index int, partialJSON string)              {}
func (h *noOpStreamHandler) OnContentBlockStop(index int)                                {}
func (h *noOpStreamHandler) OnMessageDelta(delta api.MessageDeltaBody, usage *api.Usage) {}
func (h *noOpStreamHandler) OnMessageStop()                                              {}
func (h *noOpStreamHandler) OnError(err error)                                           {}
