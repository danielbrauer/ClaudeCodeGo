package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
)

// mockTransport is a test double for Transport.
type mockTransport struct {
	mu        sync.Mutex
	responses []*JSONRPCResponse // queued responses
	requests  []*JSONRPCRequest  // captured requests
	notifs    []*JSONRPCRequest  // captured notifications
	closed    bool
}

func newMockTransport() *mockTransport {
	return &mockTransport{}
}

func (m *mockTransport) enqueue(resp *JSONRPCResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = append(m.responses, resp)
}

func (m *mockTransport) Send(ctx context.Context, req *JSONRPCRequest) (*JSONRPCResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, req)
	if len(m.responses) == 0 {
		return nil, fmt.Errorf("no mock responses queued")
	}
	resp := m.responses[0]
	m.responses = m.responses[1:]

	// Copy the request ID to the response for proper matching.
	resp.ID = req.ID
	return resp, nil
}

func (m *mockTransport) Notify(ctx context.Context, req *JSONRPCRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifs = append(m.notifs, req)
	return nil
}

func (m *mockTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func TestMCPClient_Initialize(t *testing.T) {
	transport := newMockTransport()

	// Queue initialize response.
	initResult, _ := json.Marshal(InitializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities: ServerCapabilities{
			Tools:     &ToolsCapability{},
			Resources: &ResourceCapability{Subscribe: true},
		},
		ServerInfo: ServerInfo{
			Name:    "test-server",
			Version: "1.0.0",
		},
	})
	transport.enqueue(&JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  initResult,
	})

	client := NewMCPClient("test", transport)
	ctx := context.Background()

	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("Initialize error: %v", err)
	}

	// Check that initialize was called.
	if len(transport.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(transport.requests))
	}
	if transport.requests[0].Method != "initialize" {
		t.Errorf("method = %q, want %q", transport.requests[0].Method, "initialize")
	}

	// Check that initialized notification was sent.
	if len(transport.notifs) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(transport.notifs))
	}
	if transport.notifs[0].Method != "notifications/initialized" {
		t.Errorf("notification method = %q, want %q", transport.notifs[0].Method, "notifications/initialized")
	}

	// Check parsed capabilities.
	if client.Capabilities().Tools == nil {
		t.Error("expected tools capability")
	}
	if client.Capabilities().Resources == nil {
		t.Error("expected resources capability")
	}
	if client.ServerInfoResult().Name != "test-server" {
		t.Errorf("server name = %q, want %q", client.ServerInfoResult().Name, "test-server")
	}
}

func TestMCPClient_ListTools(t *testing.T) {
	transport := newMockTransport()

	toolsResult, _ := json.Marshal(ToolsListResult{
		Tools: []MCPToolDef{
			{
				Name:        "create_issue",
				Description: "Create a GitHub issue",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"title":{"type":"string"}}}`),
			},
			{
				Name:        "list_repos",
				Description: "List repositories",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			},
		},
	})
	transport.enqueue(&JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  toolsResult,
	})

	client := NewMCPClient("github", transport)
	ctx := context.Background()

	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools error: %v", err)
	}

	if len(tools) != 2 {
		t.Fatalf("tools len = %d, want 2", len(tools))
	}
	if tools[0].Name != "create_issue" {
		t.Errorf("tools[0].Name = %q, want %q", tools[0].Name, "create_issue")
	}
	if tools[1].Name != "list_repos" {
		t.Errorf("tools[1].Name = %q, want %q", tools[1].Name, "list_repos")
	}
}

func TestMCPClient_CallTool(t *testing.T) {
	transport := newMockTransport()

	callResult, _ := json.Marshal(ToolCallResult{
		Content: []ToolResultContent{
			{Type: "text", Text: "Issue #42 created"},
		},
	})
	transport.enqueue(&JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  callResult,
	})

	client := NewMCPClient("github", transport)
	ctx := context.Background()

	result, err := client.CallTool(ctx, "create_issue", json.RawMessage(`{"title":"Bug"}`))
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}

	// Verify the request.
	if len(transport.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(transport.requests))
	}
	if transport.requests[0].Method != "tools/call" {
		t.Errorf("method = %q, want %q", transport.requests[0].Method, "tools/call")
	}

	// Verify the result contains expected data.
	var parsed ToolCallResult
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(parsed.Content) != 1 || parsed.Content[0].Text != "Issue #42 created" {
		t.Errorf("unexpected result content: %v", parsed.Content)
	}
}

func TestMCPClient_ListResources(t *testing.T) {
	transport := newMockTransport()

	resResult, _ := json.Marshal(ResourcesListResult{
		Resources: []MCPResource{
			{URI: "file:///tmp/test.txt", Name: "test.txt", MIMEType: "text/plain"},
		},
	})
	transport.enqueue(&JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  resResult,
	})

	client := NewMCPClient("fs", transport)
	ctx := context.Background()

	resources, err := client.ListResources(ctx)
	if err != nil {
		t.Fatalf("ListResources error: %v", err)
	}

	if len(resources) != 1 {
		t.Fatalf("resources len = %d, want 1", len(resources))
	}
	if resources[0].URI != "file:///tmp/test.txt" {
		t.Errorf("URI = %q", resources[0].URI)
	}
}

func TestMCPClient_ReadResource(t *testing.T) {
	transport := newMockTransport()

	readResult, _ := json.Marshal(ResourceReadResult{
		Contents: []MCPResourceContent{
			{URI: "file:///tmp/test.txt", MIMEType: "text/plain", Text: "hello world"},
		},
	})
	transport.enqueue(&JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  readResult,
	})

	client := NewMCPClient("fs", transport)
	ctx := context.Background()

	contents, err := client.ReadResource(ctx, "file:///tmp/test.txt")
	if err != nil {
		t.Fatalf("ReadResource error: %v", err)
	}

	if len(contents) != 1 {
		t.Fatalf("contents len = %d, want 1", len(contents))
	}
	if contents[0].Text != "hello world" {
		t.Errorf("Text = %q", contents[0].Text)
	}
}

func TestMCPClient_ErrorResponse(t *testing.T) {
	transport := newMockTransport()

	transport.enqueue(&JSONRPCResponse{
		JSONRPC: "2.0",
		Error: &JSONRPCError{
			Code:    -32601,
			Message: "Method not found",
		},
	})

	client := NewMCPClient("test", transport)
	ctx := context.Background()

	_, err := client.ListTools(ctx)
	if err == nil {
		t.Fatal("expected error for error response")
	}
	if err.Error() != "tools/list: Method not found" {
		t.Errorf("error = %q", err.Error())
	}
}

func TestMCPClient_Close(t *testing.T) {
	transport := newMockTransport()
	client := NewMCPClient("test", transport)

	if err := client.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	if !transport.closed {
		t.Error("transport should be closed")
	}
}

func TestMCPClient_IDIncrement(t *testing.T) {
	transport := newMockTransport()

	// Queue two responses.
	transport.enqueue(&JSONRPCResponse{JSONRPC: "2.0", Result: json.RawMessage(`{"tools":[]}`)})
	transport.enqueue(&JSONRPCResponse{JSONRPC: "2.0", Result: json.RawMessage(`{"tools":[]}`)})

	client := NewMCPClient("test", transport)
	ctx := context.Background()

	client.ListTools(ctx)
	client.ListTools(ctx)

	if len(transport.requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(transport.requests))
	}

	id1 := *transport.requests[0].ID
	id2 := *transport.requests[1].ID

	if id1 >= id2 {
		t.Errorf("IDs should be incrementing: %d, %d", id1, id2)
	}
}
