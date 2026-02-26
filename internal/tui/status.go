package tui

import (
	"fmt"

	"github.com/anthropics/claude-code-go/internal/config"
)

// Pricing per million tokens (USD) for supported models.
// These match the published Anthropic pricing as of 2025.
var modelPricing = map[string]struct{ Input, Output, CacheRead, CacheWrite float64 }{
	"claude-opus-4-6":            {Input: 15.0, Output: 75.0, CacheRead: 1.5, CacheWrite: 18.75},
	"claude-sonnet-4-6":          {Input: 3.0, Output: 15.0, CacheRead: 0.3, CacheWrite: 3.75},
	"claude-haiku-4-5-20251001":  {Input: 0.8, Output: 4.0, CacheRead: 0.08, CacheWrite: 1.0},
}

// tokenTracker accumulates token usage across the session.
type tokenTracker struct {
	TotalInputTokens  int
	TotalOutputTokens int
	TotalCacheRead    int
	TotalCacheWrite   int
	TurnCount         int
	TotalCostUSD      float64
	modelID           string // current model for pricing
}

// setModel updates the pricing model.
func (t *tokenTracker) setModel(model string) {
	t.modelID = model
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
	t.updateCost(inputTokens, 0, cacheRead, cacheWrite)
}

// addOutput records output tokens from a message_delta event.
func (t *tokenTracker) addOutput(outputTokens int) {
	t.TotalOutputTokens += outputTokens
	t.TurnCount++
	t.updateCost(0, outputTokens, nil, nil)
}

// updateCost recalculates cost based on the current model pricing.
func (t *tokenTracker) updateCost(inputTokens, outputTokens int, cacheRead, cacheWrite *int) {
	pricing, ok := modelPricing[t.modelID]
	if !ok {
		return
	}
	// Cost = tokens * price_per_million / 1_000_000
	t.TotalCostUSD += float64(inputTokens) * pricing.Input / 1_000_000
	t.TotalCostUSD += float64(outputTokens) * pricing.Output / 1_000_000
	if cacheRead != nil {
		t.TotalCostUSD += float64(*cacheRead) * pricing.CacheRead / 1_000_000
	}
	if cacheWrite != nil {
		t.TotalCostUSD += float64(*cacheWrite) * pricing.CacheWrite / 1_000_000
	}
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
	costStr := "N/A"
	if tracker.TotalCostUSD > 0 {
		costStr = fmt.Sprintf("$%.4f", tracker.TotalCostUSD)
	}
	return fmt.Sprintf(`Token Usage:
  Input tokens:  %d
  Output tokens: %d
  Cache read:    %d
  Cache write:   %d
  API turns:     %d
  Total cost:    %s`,
		tracker.TotalInputTokens,
		tracker.TotalOutputTokens,
		tracker.TotalCacheRead,
		tracker.TotalCacheWrite,
		tracker.TurnCount,
		costStr)
}

// formatTokenCount formats a token count with K suffix for readability.
func formatTokenCount(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000.0)
	}
	return fmt.Sprintf("%d", n)
}
