package mock

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/anthropics/claude-code-go/internal/api"
)

// Backend is a mock Claude API server for testing. It captures requests,
// delegates to a Responder to produce responses, and writes them as SSE
// streams — exactly as the real API does.
//
// Usage:
//
//	b := mock.NewBackend(mock.NewScriptedResponder(responses))
//	defer b.Close()
//	client := api.NewClient(&mock.StaticTokenSource{Token: "test"}, api.WithBaseURL(b.URL()))
type Backend struct {
	server    *httptest.Server
	responder Responder

	mu       sync.Mutex
	requests []*CapturedRequest
}

// CapturedRequest records the details of an API request for test assertions.
type CapturedRequest struct {
	Method  string
	Path    string
	Headers http.Header
	Body    *api.CreateMessageRequest
	RawBody []byte
}

// ToolResults extracts tool_result content blocks from the last user message
// in the request. This returns the tool results from the most recent turn,
// which is typically what tests want to assert on.
func (r *CapturedRequest) ToolResults() []api.ContentBlock {
	// Walk backwards to find the last user message with tool results.
	for i := len(r.Body.Messages) - 1; i >= 0; i-- {
		msg := r.Body.Messages[i]
		if msg.Role != api.RoleUser {
			continue
		}
		var blocks []api.ContentBlock
		if err := json.Unmarshal(msg.Content, &blocks); err != nil {
			continue
		}
		var results []api.ContentBlock
		for _, b := range blocks {
			if b.Type == api.ContentTypeToolResult {
				results = append(results, b)
			}
		}
		if len(results) > 0 {
			return results
		}
	}
	return nil
}

// AllToolResults extracts tool_result content blocks from all user messages
// in the request. This returns every tool result in the conversation history.
func (r *CapturedRequest) AllToolResults() []api.ContentBlock {
	var results []api.ContentBlock
	for _, msg := range r.Body.Messages {
		if msg.Role != api.RoleUser {
			continue
		}
		var blocks []api.ContentBlock
		if err := json.Unmarshal(msg.Content, &blocks); err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == api.ContentTypeToolResult {
				results = append(results, b)
			}
		}
	}
	return results
}

// ToolResultContent returns the string content of a tool_result block.
// Tool results store their content as a JSON-encoded string in the Content field.
func ToolResultContent(block api.ContentBlock) string {
	if block.Type != api.ContentTypeToolResult {
		return ""
	}
	var s string
	if err := json.Unmarshal(block.Content, &s); err != nil {
		return string(block.Content)
	}
	return s
}

// NewBackend creates and starts a mock API backend with the given responder.
func NewBackend(responder Responder) *Backend {
	b := &Backend{responder: responder}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/messages", b.handleMessages)
	b.server = httptest.NewServer(mux)
	return b
}

// URL returns the base URL of the mock server (e.g. "http://127.0.0.1:PORT").
func (b *Backend) URL() string {
	return b.server.URL
}

// Close shuts down the mock server.
func (b *Backend) Close() {
	b.server.Close()
}

// Requests returns all captured requests in order.
func (b *Backend) Requests() []*CapturedRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	cp := make([]*CapturedRequest, len(b.requests))
	copy(cp, b.requests)
	return cp
}

// RequestCount returns how many requests have been captured.
func (b *Backend) RequestCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.requests)
}

// LastRequest returns the most recent request, or nil if none.
func (b *Backend) LastRequest() *CapturedRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.requests) == 0 {
		return nil
	}
	return b.requests[len(b.requests)-1]
}

// SetResponder replaces the responder at runtime. This is useful for
// changing mock behavior between test phases.
func (b *Backend) SetResponder(r Responder) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.responder = r
}

// Client creates an api.Client pre-configured to talk to this mock backend.
// It uses a StaticTokenSource so no authentication is needed.
func (b *Backend) Client(opts ...api.ClientOption) *api.Client {
	allOpts := append([]api.ClientOption{api.WithBaseURL(b.URL())}, opts...)
	return api.NewClient(&StaticTokenSource{Token: "mock-token"}, allOpts...)
}

func (b *Backend) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read and parse the request body.
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req api.CreateMessageRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Capture the request for assertions.
	captured := &CapturedRequest{
		Method:  r.Method,
		Path:    r.URL.Path,
		Headers: r.Header.Clone(),
		Body:    &req,
		RawBody: rawBody,
	}
	b.mu.Lock()
	b.requests = append(b.requests, captured)
	responder := b.responder
	b.mu.Unlock()

	// Get the response from the responder.
	resp := responder.Respond(&req)
	if resp == nil {
		http.Error(w, "responder returned nil", http.StatusInternalServerError)
		return
	}

	// Write the SSE stream.
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)

	if err := WriteSSEResponse(w, resp); err != nil {
		// Can't change status at this point, log to response.
		fmt.Fprintf(w, "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"mock_error\",\"message\":\"%s\"}}\n\n", err.Error())
	}

	// Flush if possible.
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
