package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// WebSearchInput is the input schema for the WebSearch tool.
type WebSearchInput struct {
	Query          string   `json:"query"`
	AllowedDomains []string `json:"allowed_domains,omitempty"`
	BlockedDomains []string `json:"blocked_domains,omitempty"`
}

// WebSearchTool performs web searches.
// In the official CLI, WebSearch is a server-side tool — the API itself performs
// the search. The tool definition is sent so the model knows it's available,
// but execution is handled server-side. This implementation returns a message
// indicating that server-side handling is expected.
type WebSearchTool struct{}

// NewWebSearchTool creates a new WebSearch tool.
func NewWebSearchTool() *WebSearchTool {
	return &WebSearchTool{}
}

func (t *WebSearchTool) Name() string { return "WebSearch" }

func (t *WebSearchTool) Description() string {
	return `Search the web and return results. Provides up-to-date information for current events and recent data. Returns search results with titles, URLs, and snippets. Supports domain filtering via allowed_domains and blocked_domains.`
}

func (t *WebSearchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "The search query to use",
      "minLength": 2
    },
    "allowed_domains": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Only include search results from these domains"
    },
    "blocked_domains": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Never include search results from these domains"
    }
  },
  "required": ["query"],
  "additionalProperties": false
}`)
}

func (t *WebSearchTool) RequiresPermission(_ json.RawMessage) bool {
	return false // read-only, no local side effects
}

func (t *WebSearchTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var in WebSearchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parsing WebSearch input: %w", err)
	}

	if in.Query == "" {
		return "Error: query is required", nil
	}

	// WebSearch is a server-side tool in the official CLI.
	// The API handles the actual search. This tool definition exists so the model
	// knows it can request web searches. If execution reaches here, the server
	// did not handle it, so we return a helpful message.
	result := map[string]interface{}{
		"query":   in.Query,
		"results": []interface{}{},
		"message": "Web search is a server-side capability. Results are provided by the API.",
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}
