package conversation

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/anthropics/claude-code-go/internal/api"
)

// ephemeralCache is the cache control value used for all prompt caching.
var ephemeralCache = &api.CacheControl{Type: "ephemeral"}

// IsCachingEnabled checks environment variables to determine if prompt
// caching should be used for the given model. Caching is enabled by default
// and can be disabled per-model or globally via environment variables,
// matching the JS CLI behavior.
func IsCachingEnabled(model string) bool {
	if envBool("DISABLE_PROMPT_CACHING") {
		return false
	}
	modelLower := strings.ToLower(model)
	if envBool("DISABLE_PROMPT_CACHING_HAIKU") && strings.Contains(modelLower, "haiku") {
		return false
	}
	if envBool("DISABLE_PROMPT_CACHING_SONNET") && strings.Contains(modelLower, "sonnet") {
		return false
	}
	if envBool("DISABLE_PROMPT_CACHING_OPUS") && strings.Contains(modelLower, "opus") {
		return false
	}
	return true
}

// envBool returns true if the named environment variable is set to a truthy value.
func envBool(key string) bool {
	v := os.Getenv(key)
	return v == "1" || strings.EqualFold(v, "true")
}

// WithSystemPromptCaching returns a copy of the system blocks with
// cache_control added to the last block. This caches the entire system
// prompt as a prefix, matching the API's 4-block cache_control limit
// (1 system + 1 tools + 2 messages = 4).
func WithSystemPromptCaching(blocks []api.SystemBlock) []api.SystemBlock {
	if len(blocks) == 0 {
		return blocks
	}
	out := make([]api.SystemBlock, len(blocks))
	copy(out, blocks)
	out[len(out)-1].CacheControl = ephemeralCache
	return out
}

// WithToolsCaching returns a copy of the tool definitions with
// cache_control added to the last tool. This caches the entire tool
// definition list as a prefix, matching the JS CLI behavior.
func WithToolsCaching(tools []api.ToolDefinition) []api.ToolDefinition {
	if len(tools) == 0 {
		return tools
	}
	out := make([]api.ToolDefinition, len(tools))
	copy(out, tools)
	out[len(out)-1].CacheControl = ephemeralCache
	return out
}

// WithMessageCaching returns a copy of the message list with cache_control
// added to the last content block of the last ~2 messages.
//
// This matches the JS CLI behavior: cache breakpoints are placed on the
// last 2 messages so that on the next API call, only the newest message
// needs to be processed as fresh input tokens. Older messages are read
// from cache.
//
// For assistant messages, thinking/redacted_thinking blocks are excluded
// from cache control placement (cache_control goes on the last
// non-thinking block).
func WithMessageCaching(msgs []api.Message) []api.Message {
	if len(msgs) == 0 {
		return msgs
	}
	out := make([]api.Message, len(msgs))
	copy(out, msgs)

	// Apply cache_control to the last 2 messages.
	start := len(out) - 2
	if start < 0 {
		start = 0
	}
	for i := start; i < len(out); i++ {
		out[i] = addCacheControlToMessage(out[i])
	}
	return out
}

// addCacheControlToMessage adds cache_control to the last eligible content
// block of a message. Returns a new Message with modified content; the
// original is not mutated.
func addCacheControlToMessage(msg api.Message) api.Message {
	// Try to decode as []ContentBlock.
	var blocks []api.ContentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err == nil && len(blocks) > 0 {
		// Find last non-thinking block.
		lastIdx := -1
		for j := len(blocks) - 1; j >= 0; j-- {
			if blocks[j].Type != "thinking" && blocks[j].Type != "redacted_thinking" {
				lastIdx = j
				break
			}
		}
		if lastIdx >= 0 {
			modified := make([]api.ContentBlock, len(blocks))
			copy(modified, blocks)
			modified[lastIdx].CacheControl = ephemeralCache
			content, _ := json.Marshal(modified)
			return api.Message{Role: msg.Role, Content: content}
		}
		return msg
	}

	// Content is a plain string — wrap in a text block with cache_control.
	var text string
	if err := json.Unmarshal(msg.Content, &text); err == nil {
		block := []api.ContentBlock{{
			Type:         api.ContentTypeText,
			Text:         text,
			CacheControl: ephemeralCache,
		}}
		content, _ := json.Marshal(block)
		return api.Message{Role: msg.Role, Content: content}
	}

	return msg
}
