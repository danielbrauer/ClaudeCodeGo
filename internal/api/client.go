package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	DefaultBaseURL    = "https://api.anthropic.com"
	DefaultAPIVersion = "2023-06-01"
	DefaultMaxTokens  = 16384
)

// TokenSource provides access tokens for API authentication.
type TokenSource interface {
	GetAccessToken(ctx context.Context) (string, error)
}

// RefreshableTokenSource extends TokenSource with the ability to invalidate
// cached tokens, forcing a re-fetch/refresh on the next call.
// Issue 15: used for 401 auto-retry.
type RefreshableTokenSource interface {
	TokenSource
	InvalidateToken()
}

// Client is the Claude Messages API client.
type Client struct {
	baseURL     string
	apiVersion  string
	httpClient  *http.Client
	tokenSource TokenSource
	model       string
	maxTokens   int
	userAgent   string // Issue 14: User-Agent header
}

// ClientOption configures the client.
type ClientOption func(*Client)

// WithBaseURL sets a custom API base URL.
func WithBaseURL(url string) ClientOption {
	return func(c *Client) { c.baseURL = url }
}

// WithModel sets the default model.
func WithModel(model string) ClientOption {
	return func(c *Client) { c.model = model }
}

// WithMaxTokens sets the default max tokens.
func WithMaxTokens(n int) ClientOption {
	return func(c *Client) { c.maxTokens = n }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *Client) { c.httpClient = hc }
}

// WithVersion sets the CLI version for the User-Agent header.
// Issue 14: No User-Agent header on API requests.
func WithVersion(version string) ClientOption {
	return func(c *Client) { c.userAgent = "claude-code/" + version }
}

// NewClient creates a new API client.
func NewClient(tokenSource TokenSource, opts ...ClientOption) *Client {
	c := &Client{
		baseURL:     DefaultBaseURL,
		apiVersion:  DefaultAPIVersion,
		httpClient:  http.DefaultClient,
		tokenSource: tokenSource,
		model:       ModelClaude46Opus,
		maxTokens:   DefaultMaxTokens,
		userAgent:   "claude-code/dev",
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Model returns the current model.
func (c *Client) Model() string {
	return c.model
}

// SetModel changes the model used for subsequent API calls.
func (c *Client) SetModel(model string) {
	c.model = model
}

// CreateMessageStream sends a streaming Messages API request and dispatches
// events to the provided handler. It returns the final assembled response.
func (c *Client) CreateMessageStream(
	ctx context.Context,
	req *CreateMessageRequest,
	handler StreamHandler,
) (*MessageResponse, error) {
	// Apply defaults.
	if req.Model == "" {
		req.Model = c.model
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = c.maxTokens
	}
	req.Stream = true

	// Collect extra beta headers needed for this request.
	var extraBetas []string
	if req.Speed == "fast" {
		extraBetas = append(extraBetas, FastModeBeta)
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	// Issue 15: 401 auto-retry loop. Attempts at most 2 requests.
	resp, err := c.doAPIRequest(ctx, body, extraBetas)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	// Parse the SSE stream using an assembler that collects the final response.
	assembler := newResponseAssembler(handler)
	if err := ParseSSEStream(resp.Body, assembler); err != nil {
		return nil, err
	}

	return assembler.Response(), nil
}

// doAPIRequest sends the API request with auth headers. On a 401 response,
// it invalidates the token, refreshes, and retries once.
// Issue 15: 401 auto-retry on API calls.
func (c *Client) doAPIRequest(ctx context.Context, body []byte, extraBetas []string) (*http.Response, error) {
	for attempt := 0; attempt < 2; attempt++ {
		token, err := c.tokenSource.GetAccessToken(ctx)
		if err != nil {
			return nil, fmt.Errorf("getting access token: %w", err)
		}

		httpReq, err := http.NewRequestWithContext(
			ctx, "POST", c.baseURL+"/v1/messages?beta=true", bytes.NewReader(body),
		)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		httpReq.Header.Set("Authorization", "Bearer "+token)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("anthropic-version", c.apiVersion)
		betaValues := []string{"claude-code-20250219", "oauth-2025-04-20"}
		betaValues = append(betaValues, extraBetas...)
		httpReq.Header.Set("anthropic-beta", strings.Join(betaValues, ","))
		httpReq.Header.Set("x-app", "cli")
		// Issue 14: User-Agent header.
		httpReq.Header.Set("User-Agent", c.userAgent)
		// Issue 16: Accept header — use application/json, not text/event-stream.
		// Streaming is controlled by the "stream" body parameter, not Accept.
		httpReq.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("sending request: %w", err)
		}

		// Issue 15: On 401, invalidate token and retry once.
		if resp.StatusCode == 401 && attempt == 0 {
			resp.Body.Close()
			if rts, ok := c.tokenSource.(RefreshableTokenSource); ok {
				rts.InvalidateToken()
				continue
			}
		}

		return resp, nil
	}

	// Should not be reached, but handle gracefully.
	return nil, fmt.Errorf("API request failed after retry")
}

// responseAssembler collects streaming events into a final MessageResponse.
type responseAssembler struct {
	handler  StreamHandler
	response *MessageResponse
	blocks   map[int]*ContentBlock
	jsonBuf  map[int]*bytes.Buffer
}

func newResponseAssembler(handler StreamHandler) *responseAssembler {
	return &responseAssembler{
		handler: handler,
		blocks:  make(map[int]*ContentBlock),
		jsonBuf: make(map[int]*bytes.Buffer),
	}
}

func (a *responseAssembler) Response() *MessageResponse {
	return a.response
}

func (a *responseAssembler) OnMessageStart(msg MessageResponse) {
	a.response = &msg
	a.handler.OnMessageStart(msg)
}

func (a *responseAssembler) OnContentBlockStart(index int, block ContentBlock) {
	a.blocks[index] = &block
	if block.Type == ContentTypeToolUse {
		a.jsonBuf[index] = &bytes.Buffer{}
	}
	a.handler.OnContentBlockStart(index, block)
}

func (a *responseAssembler) OnTextDelta(index int, text string) {
	if b, ok := a.blocks[index]; ok {
		b.Text += text
	}
	a.handler.OnTextDelta(index, text)
}

func (a *responseAssembler) OnInputJSONDelta(index int, partialJSON string) {
	if buf, ok := a.jsonBuf[index]; ok {
		buf.WriteString(partialJSON)
	}
	a.handler.OnInputJSONDelta(index, partialJSON)
}

func (a *responseAssembler) OnContentBlockStop(index int) {
	// Finalize tool_use input JSON.
	if buf, ok := a.jsonBuf[index]; ok {
		if b, ok := a.blocks[index]; ok {
			b.Input = json.RawMessage(buf.Bytes())
		}
	}

	// Add completed block to response.
	if a.response != nil {
		if b, ok := a.blocks[index]; ok {
			// Grow the content slice if needed.
			for len(a.response.Content) <= index {
				a.response.Content = append(a.response.Content, ContentBlock{})
			}
			a.response.Content[index] = *b
		}
	}

	a.handler.OnContentBlockStop(index)
}

func (a *responseAssembler) OnMessageDelta(delta MessageDeltaBody, usage *Usage) {
	if a.response != nil {
		a.response.StopReason = delta.StopReason
		a.response.StopSequence = delta.StopSequence
		if usage != nil {
			a.response.Usage.OutputTokens = usage.OutputTokens
		}
	}
	a.handler.OnMessageDelta(delta, usage)
}

func (a *responseAssembler) OnMessageStop() {
	a.handler.OnMessageStop()
}

func (a *responseAssembler) OnError(err error) {
	a.handler.OnError(err)
}
