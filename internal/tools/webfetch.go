package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// WebFetchInput is the input schema for the WebFetch tool.
type WebFetchInput struct {
	URL    string `json:"url"`
	Prompt string `json:"prompt"`
}

// webFetchCacheEntry stores a cached fetch result.
type webFetchCacheEntry struct {
	content   string
	fetchedAt time.Time
}

// WebFetchTool fetches URL content and processes it with a prompt.
type WebFetchTool struct {
	apiClient  *apiClientForWebFetch
	httpClient *http.Client
	mu         sync.Mutex
	cache      map[string]*webFetchCacheEntry
}

// apiClientForWebFetch is the minimal interface the WebFetch tool needs from the API client.
type apiClientForWebFetch interface {
	CreateMessageSimple(ctx context.Context, system string, userMessage string) (string, error)
}

// apiClientWrapper wraps our api.Client to provide a simple message interface.
type apiClientWrapper struct {
	client interface {
		CreateMessageStream(ctx context.Context, req interface{}, handler interface{}) (interface{}, error)
	}
}

// NewWebFetchTool creates a new WebFetch tool. The client parameter is used
// to process fetched content with the API. If client is nil, content is returned directly.
func NewWebFetchTool(httpClient *http.Client) *WebFetchTool {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}
	return &WebFetchTool{
		httpClient: httpClient,
		cache:      make(map[string]*webFetchCacheEntry),
	}
}

func (t *WebFetchTool) Name() string { return "WebFetch" }

func (t *WebFetchTool) Description() string {
	return `Fetches content from a URL and returns it. The URL must be a fully-formed valid URL. HTTP URLs will be automatically upgraded to HTTPS. Includes a 15-minute cache for repeated access to the same URL.`
}

func (t *WebFetchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "url": {
      "type": "string",
      "description": "The URL to fetch content from",
      "format": "uri"
    },
    "prompt": {
      "type": "string",
      "description": "The prompt to run on the fetched content"
    }
  },
  "required": ["url", "prompt"],
  "additionalProperties": false
}`)
}

func (t *WebFetchTool) RequiresPermission(_ json.RawMessage) bool {
	return true // network access
}

func (t *WebFetchTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var in WebFetchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parsing WebFetch input: %w", err)
	}

	if in.URL == "" {
		return "Error: url is required", nil
	}
	if in.Prompt == "" {
		return "Error: prompt is required", nil
	}

	// Upgrade HTTP to HTTPS.
	url := in.URL
	if strings.HasPrefix(url, "http://") {
		url = "https://" + url[7:]
	}

	startTime := time.Now()

	// Check cache.
	t.mu.Lock()
	if entry, ok := t.cache[url]; ok && time.Since(entry.fetchedAt) < 15*time.Minute {
		t.mu.Unlock()
		durationMs := time.Since(startTime).Milliseconds()
		return t.buildResult(url, entry.content, 200, "OK", len(entry.content), durationMs), nil
	}
	t.mu.Unlock()

	// Fetch the URL.
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Sprintf("Error creating request: %v", err), nil
	}
	req.Header.Set("User-Agent", "ClaudeCode/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Sprintf("Error fetching URL: %v", err), nil
	}
	defer resp.Body.Close()

	// Read body with size limit (10MB).
	const maxBodySize = 10 * 1024 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return fmt.Sprintf("Error reading response: %v", err), nil
	}

	content := string(body)

	// Basic HTML to text conversion.
	if strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		content = htmlToText(content)
	}

	// Truncate if very large.
	const maxContent = 100_000
	if len(content) > maxContent {
		content = content[:maxContent] + "\n... (content truncated)"
	}

	// Cache the result.
	t.mu.Lock()
	t.cache[url] = &webFetchCacheEntry{
		content:   content,
		fetchedAt: time.Now(),
	}
	t.mu.Unlock()

	durationMs := time.Since(startTime).Milliseconds()
	return t.buildResult(url, content, resp.StatusCode, http.StatusText(resp.StatusCode), len(body), durationMs), nil
}

// buildResult creates the JSON output for the tool.
func (t *WebFetchTool) buildResult(url, content string, code int, codeText string, bytes int, durationMs int64) string {
	result := map[string]interface{}{
		"url":        url,
		"result":     content,
		"code":       code,
		"codeText":   codeText,
		"bytes":      bytes,
		"durationMs": durationMs,
	}
	out, _ := json.Marshal(result)
	return string(out)
}

// htmlToText strips HTML tags and extracts text content.
func htmlToText(html string) string {
	// Remove script and style blocks.
	reScript := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	html = reScript.ReplaceAllString(html, "")
	reStyle := regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	html = reStyle.ReplaceAllString(html, "")

	// Replace common block elements with newlines.
	reBlock := regexp.MustCompile(`(?i)<(?:br|p|div|h[1-6]|li|tr)[^>]*>`)
	html = reBlock.ReplaceAllString(html, "\n")

	// Strip all remaining tags.
	reTags := regexp.MustCompile(`<[^>]+>`)
	html = reTags.ReplaceAllString(html, "")

	// Decode common HTML entities.
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&quot;", "\"")
	html = strings.ReplaceAll(html, "&#39;", "'")
	html = strings.ReplaceAll(html, "&nbsp;", " ")

	// Collapse multiple blank lines.
	reBlank := regexp.MustCompile(`\n{3,}`)
	html = reBlank.ReplaceAllString(html, "\n\n")

	return strings.TrimSpace(html)
}
