package tui

import (
	"fmt"

	"github.com/anthropics/claude-code-go/internal/config"
)

// tokenTracker accumulates token usage across the session.
type tokenTracker struct {
	TotalInputTokens  int
	TotalOutputTokens int
	TotalCacheRead    int
	TotalCacheWrite   int
	TurnCount         int
}

// addInput records input tokens from a message_start event.
func (t *tokenTracker) addInput(inputTokens int, cacheRead, cacheWrite *int) {
	t.TotalInputTokens += inputTokens
	if cacheRead != nil {
		t.TotalCacheRead += *cacheRead
	}
	if cacheWrite != nil {
		t.TotalCacheWrite += *cacheWrite
	}
}

// addOutput records output tokens from a message_delta event.
func (t *tokenTracker) addOutput(outputTokens int) {
	t.TotalOutputTokens += outputTokens
	t.TurnCount++
}

// renderStatusBar returns the formatted status bar string.
func renderStatusBar(model string, tracker *tokenTracker, width int, fastMode bool, permMode config.PermissionMode) string {
	modelStr := statusModelStyle.Render(model)
	tokensStr := fmt.Sprintf("%s in / %s out",
		formatTokenCount(tracker.TotalInputTokens),
		formatTokenCount(tracker.TotalOutputTokens))

	parts := modelStr + "  " + tokensStr
	if fastMode {
		parts += "  " + fastModeStyle.Render("⚡ Fast")
	}

	// Show permission mode indicator when not in default mode.
	if permMode != config.ModeDefault && permMode != "" {
		info := config.PermissionModeMetadata[permMode]
		modeText := info.Symbol
		if modeText != "" {
			modeText += " "
		}
		modeText += info.ShortTitle
		switch info.Color {
		case "error":
			parts += "  " + permModeErrorStyle.Render(modeText)
		case "planMode":
			parts += "  " + permModePlanStyle.Render(modeText)
		case "autoAccept":
			parts += "  " + permModeAutoAcceptStyle.Render(modeText)
		default:
			parts += "  " + modeText
		}
	}

	return statusBarStyle.Render(parts)
}

// renderCostSummary returns a detailed cost breakdown for the /cost command.
func renderCostSummary(tracker *tokenTracker) string {
	return fmt.Sprintf(`Token Usage:
  Input tokens:  %d
  Output tokens: %d
  Cache read:    %d
  Cache write:   %d
  API turns:     %d`,
		tracker.TotalInputTokens,
		tracker.TotalOutputTokens,
		tracker.TotalCacheRead,
		tracker.TotalCacheWrite,
		tracker.TurnCount)
}

// formatTokenCount formats a token count with K suffix for readability.
func formatTokenCount(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000.0)
	}
	return fmt.Sprintf("%d", n)
}
