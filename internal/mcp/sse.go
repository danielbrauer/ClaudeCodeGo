package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// SSETransport communicates with an MCP server over HTTP using
// Server-Sent Events for responses.
type SSETransport struct {
	baseURL    string
	client     *http.Client
	mu         sync.Mutex
	endpointCh chan string // receives the messages endpoint from the SSE stream
	endpoint   string     // resolved endpoint for sending messages
	cancel     context.CancelFunc
	closed     bool
}

// NewSSETransport creates an SSE transport that connects to the given URL.
// The URL should be the SSE endpoint of the MCP server.
func NewSSETransport(url string) *SSETransport {
	return &SSETransport{
		baseURL:    url,
		client:     &http.Client{},
		endpointCh: make(chan string, 1),
	}
}

// Connect establishes the SSE connection and discovers the messages endpoint.
func (t *SSETransport) Connect(ctx context.Context) error {
	connCtx, cancel := context.WithCancel(ctx)
	t.cancel = cancel

	req, err := http.NewRequestWithContext(connCtx, "GET", t.baseURL, nil)
	if err != nil {
		cancel()
		return fmt.Errorf("create SSE request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := t.client.Do(req)
	if err != nil {
		cancel()
		return fmt.Errorf("SSE connect: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		cancel()
		return fmt.Errorf("SSE connect: status %d", resp.StatusCode)
	}

	// Read SSE events in a goroutine. The first "endpoint" event tells us
	// where to POST JSON-RPC messages.
	go t.readSSEStream(resp.Body)

	// Wait for the endpoint event.
	select {
	case endpoint := <-t.endpointCh:
		t.endpoint = endpoint
		return nil
	case <-ctx.Done():
		cancel()
		resp.Body.Close()
		return ctx.Err()
	}
}

// readSSEStream reads SSE events from the response body.
// It looks for an "endpoint" event to resolve the messages URL.
func (t *SSETransport) readSSEStream(body io.ReadCloser) {
	defer body.Close()

	scanner := bufio.NewScanner(body)
	var eventType string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}

		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

			if eventType == "endpoint" {
				// Resolve relative URL against base.
				endpoint := data
				if !strings.HasPrefix(endpoint, "http") {
					// Build absolute URL from base URL.
					base := t.baseURL
					if idx := strings.LastIndex(base, "/"); idx > 8 { // after "https://"
						base = base[:idx]
					}
					endpoint = base + "/" + strings.TrimPrefix(endpoint, "/")
				}
				select {
				case t.endpointCh <- endpoint:
				default:
				}
			}
			eventType = ""
			continue
		}

		if line == "" {
			eventType = ""
		}
	}
}

// Send posts a JSON-RPC request to the server's messages endpoint
// and reads the response from the SSE stream or inline response.
func (t *SSETransport) Send(ctx context.Context, req *JSONRPCRequest) (*JSONRPCResponse, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.endpoint == "" {
		return nil, fmt.Errorf("SSE transport not connected (no endpoint)")
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", t.endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create POST request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("POST request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("POST response status %d: %s", httpResp.StatusCode, string(body))
	}

	// The response may come as a direct JSON body or via SSE.
	contentType := httpResp.Header.Get("Content-Type")

	if strings.Contains(contentType, "text/event-stream") {
		return t.readSSEResponse(httpResp.Body)
	}

	// Direct JSON response.
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	// Some servers return 202 Accepted with no body for async handling.
	if len(body) == 0 {
		return nil, fmt.Errorf("empty response body")
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &resp, nil
}

// readSSEResponse reads a single JSON-RPC response from an SSE stream.
func (t *SSETransport) readSSEResponse(body io.Reader) (*JSONRPCResponse, error) {
	scanner := bufio.NewScanner(body)
	var eventType string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}

		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

			if eventType == "message" || eventType == "" {
				var resp JSONRPCResponse
				if err := json.Unmarshal([]byte(data), &resp); err != nil {
					continue // Skip non-JSON data events.
				}
				return &resp, nil
			}
			eventType = ""
			continue
		}

		if line == "" {
			eventType = ""
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read SSE response: %w", err)
	}

	return nil, fmt.Errorf("SSE stream ended without response")
}

// Notify sends a JSON-RPC notification via POST.
func (t *SSETransport) Notify(ctx context.Context, req *JSONRPCRequest) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.endpoint == "" {
		return fmt.Errorf("SSE transport not connected (no endpoint)")
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", t.endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create POST request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := t.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("POST notification: %w", err)
	}
	httpResp.Body.Close()

	return nil
}

// Close shuts down the SSE transport.
func (t *SSETransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	if t.cancel != nil {
		t.cancel()
	}
	return nil
}
