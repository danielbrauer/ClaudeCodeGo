package tui

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/anthropics/claude-code-go/internal/api"
)

// suggestionPrompt is the system prompt used to generate next-turn suggestions.
// It closely matches the JS CLI's prompt for behavioral parity.
const suggestionPrompt = `[SUGGESTION MODE: Suggest what the user might naturally type next into Claude Code.]

FIRST: Look at the user's recent messages and original request.

Your job is to predict what THEY would type - not what you think they should do.

THE TEST: Would they think "I was just about to type that"?

EXAMPLES:
User asked "fix the bug and run tests", bug is fixed → "run the tests"
After code written → "try it out"
Claude offers options → suggest the one the user would likely pick, based on conversation
Claude asks to continue → "yes" or "go ahead"
Task complete, obvious follow-up → "commit this" or "push it"
After error or misunderstanding → silence (let them assess/correct)

Be specific: "run the tests" beats "continue".

NEVER SUGGEST:
- Evaluative ("looks good", "thanks")
- Questions ("what about...?")
- Claude-voice ("Let me...", "I'll...", "Here's...")
- New ideas they didn't ask about
- Multiple sentences

Stay silent if the next step isn't obvious from what the user said.

Format: 2-12 words, match the user's style. Or nothing.

Reply with ONLY the suggestion, no quotes or explanation.`

// promptSuggestionResult is the message sent back to the TUI when a
// suggestion has been generated (or the generation failed/was empty).
type promptSuggestionResult struct {
	text string // empty means no suggestion
}

// generatePromptSuggestionCmd returns a tea.Cmd that generates a prompt
// suggestion by making a lightweight API call with the current conversation
// history.
func generatePromptSuggestionCmd(
	ctx context.Context,
	client *api.Client,
	messages []api.Message,
) tea.Cmd {
	return func() tea.Msg {
		if client == nil || len(messages) == 0 {
			return promptSuggestionResult{}
		}

		// Only generate suggestions after at least 2 assistant messages
		// (i.e., at least one full turn).
		assistantCount := 0
		for _, m := range messages {
			if m.Role == api.RoleAssistant {
				assistantCount++
			}
		}
		if assistantCount < 1 {
			return promptSuggestionResult{}
		}

		// Build a minimal request: same conversation, suggestion system prompt,
		// no tools, small max_tokens.
		req := &api.CreateMessageRequest{
			Messages: messages,
			System: []api.SystemBlock{
				{Type: "text", Text: suggestionPrompt},
			},
			MaxTokens: 100,
		}

		resp, err := client.CreateMessage(ctx, req)
		if err != nil {
			return promptSuggestionResult{}
		}

		// Extract the suggestion text from the response.
		for _, block := range resp.Content {
			if block.Type == api.ContentTypeText && strings.TrimSpace(block.Text) != "" {
				suggestion := strings.TrimSpace(block.Text)
				if isValidSuggestion(suggestion) {
					return promptSuggestionResult{text: suggestion}
				}
				return promptSuggestionResult{}
			}
		}

		return promptSuggestionResult{}
	}
}

// isValidSuggestion filters out bad suggestions, matching the JS CLI's
// validation logic. Returns true if the suggestion should be shown.
func isValidSuggestion(s string) bool {
	if s == "" {
		return false
	}

	lower := strings.ToLower(s)
	words := strings.Fields(s)
	wordCount := len(words)

	// "done" by itself is not useful.
	if lower == "done" {
		return false
	}

	// Meta/empty responses.
	if lower == "nothing found" || lower == "nothing found." ||
		strings.HasPrefix(lower, "nothing to suggest") ||
		strings.HasPrefix(lower, "no suggestion") {
		return false
	}

	// Error messages that leaked through.
	if strings.HasPrefix(lower, "api error:") ||
		strings.HasPrefix(lower, "prompt is too long") ||
		strings.HasPrefix(lower, "request timed out") ||
		strings.HasPrefix(lower, "invalid api key") ||
		strings.HasPrefix(lower, "image was too large") {
		return false
	}

	// Prefixed labels like "Suggestion: ..." or "Next: ...".
	if prefixedLabelRe.MatchString(s) {
		return false
	}

	// Single words (except known good ones).
	if wordCount < 2 {
		if strings.HasPrefix(s, "/") {
			return true // slash commands are fine
		}
		goodSingleWords := map[string]bool{
			"yes": true, "yeah": true, "yep": true, "yea": true,
			"yup": true, "sure": true, "ok": true, "okay": true,
			"push": true, "commit": true, "deploy": true,
			"stop": true, "continue": true, "check": true,
			"exit": true, "quit": true, "no": true,
		}
		if !goodSingleWords[lower] {
			return false
		}
	}

	// Too many words.
	if wordCount > 12 {
		return false
	}

	// Too long.
	if utf8.RuneCountInString(s) >= 100 {
		return false
	}

	// Multiple sentences.
	if multipleSentenceRe.MatchString(s) {
		return false
	}

	// Has formatting (newlines, markdown bold).
	if strings.ContainsAny(s, "\n*") {
		return false
	}

	// Evaluative phrases.
	if evaluativeRe.MatchString(lower) {
		return false
	}

	// Claude-voice (starts like Claude would respond, not what a user types).
	if claudeVoiceRe.MatchString(s) {
		return false
	}

	return true
}

var (
	prefixedLabelRe    = regexp.MustCompile(`^\w+:\s`)
	multipleSentenceRe = regexp.MustCompile(`[.!?]\s+[A-Z]`)
	evaluativeRe       = regexp.MustCompile(`thanks|thank you|looks good|sounds good|that works|that worked|that's all|nice|great|perfect|makes sense|awesome|excellent`)
	claudeVoiceRe      = regexp.MustCompile(`(?i)^(let me|i'll|i've|i'm|i can|i would|i think|i notice|here's|here is|here are|that's|this is|this will|you can|you should|you could|sure,|of course|certainly)`)
)
