package mcp

import (
	"context"
	"encoding/json"
	"testing"
)

func TestMCPToolWrapper_Name(t *testing.T) {
	wrapper := NewMCPToolWrapper("github", MCPToolDef{
		Name:        "create_issue",
		Description: "Create a GitHub issue",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, nil)

	if got := wrapper.Name(); got != "mcp__github__create_issue" {
		t.Errorf("Name() = %q, want %q", got, "mcp__github__create_issue")
	}
}

func TestMCPToolWrapper_Description(t *testing.T) {
	wrapper := NewMCPToolWrapper("github", MCPToolDef{
		Name:        "create_issue",
		Description: "Create a GitHub issue",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, nil)

	if got := wrapper.Description(); got != "Create a GitHub issue" {
		t.Errorf("Description() = %q, want %q", got, "Create a GitHub issue")
	}
}

func TestMCPToolWrapper_InputSchema(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"title":{"type":"string"}}}`)
	wrapper := NewMCPToolWrapper("github", MCPToolDef{
		Name:        "create_issue",
		Description: "Create a GitHub issue",
		InputSchema: schema,
	}, nil)

	if string(wrapper.InputSchema()) != string(schema) {
		t.Errorf("InputSchema() = %s, want %s", wrapper.InputSchema(), schema)
	}
}

func TestMCPToolWrapper_RequiresPermission(t *testing.T) {
	wrapper := NewMCPToolWrapper("github", MCPToolDef{
		Name:        "create_issue",
		Description: "Create a GitHub issue",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, nil)

	if !wrapper.RequiresPermission(nil) {
		t.Error("MCP tools should always require permission")
	}
}

func TestMCPToolWrapper_Execute(t *testing.T) {
	transport := newMockTransport()

	callResult, _ := json.Marshal(ToolCallResult{
		Content: []ToolResultContent{
			{Type: "text", Text: "Issue #42 created successfully"},
		},
	})
	transport.enqueue(&JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  callResult,
	})

	client := NewMCPClient("github", transport)
	wrapper := NewMCPToolWrapper("github", MCPToolDef{
		Name:        "create_issue",
		Description: "Create a GitHub issue",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, client)

	ctx := context.Background()
	result, err := wrapper.Execute(ctx, json.RawMessage(`{"title":"Bug report"}`))
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if result != "Issue #42 created successfully" {
		t.Errorf("result = %q, want %q", result, "Issue #42 created successfully")
	}
}

func TestMCPToolWrapper_ExecuteMultipleContent(t *testing.T) {
	transport := newMockTransport()

	callResult, _ := json.Marshal(ToolCallResult{
		Content: []ToolResultContent{
			{Type: "text", Text: "Line 1"},
			{Type: "text", Text: "Line 2"},
		},
	})
	transport.enqueue(&JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  callResult,
	})

	client := NewMCPClient("test", transport)
	wrapper := NewMCPToolWrapper("test", MCPToolDef{
		Name:        "multi",
		Description: "Multiple content blocks",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, client)

	ctx := context.Background()
	result, err := wrapper.Execute(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if result != "Line 1\nLine 2" {
		t.Errorf("result = %q, want %q", result, "Line 1\nLine 2")
	}
}

func TestMCPToolWrapper_ExecuteError(t *testing.T) {
	transport := newMockTransport()

	callResult, _ := json.Marshal(ToolCallResult{
		Content: []ToolResultContent{
			{Type: "text", Text: "Permission denied"},
		},
		IsError: true,
	})
	transport.enqueue(&JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  callResult,
	})

	client := NewMCPClient("test", transport)
	wrapper := NewMCPToolWrapper("test", MCPToolDef{
		Name:        "restricted",
		Description: "Restricted tool",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, client)

	ctx := context.Background()
	result, err := wrapper.Execute(ctx, json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for isError result")
	}
	if result != "Permission denied" {
		t.Errorf("result = %q, want %q", result, "Permission denied")
	}
}

func TestExtractTexts(t *testing.T) {
	tests := []struct {
		name    string
		content []ToolResultContent
		want    string
	}{
		{
			name:    "empty",
			content: nil,
			want:    "",
		},
		{
			name: "single text",
			content: []ToolResultContent{
				{Type: "text", Text: "hello"},
			},
			want: "hello",
		},
		{
			name: "multiple texts",
			content: []ToolResultContent{
				{Type: "text", Text: "hello"},
				{Type: "text", Text: "world"},
			},
			want: "hello\nworld",
		},
		{
			name: "mixed types",
			content: []ToolResultContent{
				{Type: "text", Text: "hello"},
				{Type: "image"},
				{Type: "text", Text: "world"},
			},
			want: "hello\nworld",
		},
		{
			name: "skip empty text",
			content: []ToolResultContent{
				{Type: "text", Text: ""},
				{Type: "text", Text: "hello"},
			},
			want: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTexts(tt.content)
			if got != tt.want {
				t.Errorf("extractTexts() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestListMcpResourcesTool_Schema(t *testing.T) {
	tool := NewListMcpResourcesTool(NewManager("/tmp"))

	if tool.Name() != "ListMcpResources" {
		t.Errorf("Name() = %q", tool.Name())
	}
	if tool.RequiresPermission(nil) {
		t.Error("ListMcpResources should not require permission")
	}

	// Verify schema is valid JSON.
	var schema map[string]interface{}
	if err := json.Unmarshal(tool.InputSchema(), &schema); err != nil {
		t.Fatalf("InputSchema() is not valid JSON: %v", err)
	}
}

func TestReadMcpResourceTool_Schema(t *testing.T) {
	tool := NewReadMcpResourceTool(NewManager("/tmp"))

	if tool.Name() != "ReadMcpResource" {
		t.Errorf("Name() = %q", tool.Name())
	}
	if tool.RequiresPermission(nil) {
		t.Error("ReadMcpResource should not require permission")
	}
}

func TestSubscribeMcpResourceTool_Schema(t *testing.T) {
	tool := NewSubscribeMcpResourceTool(NewManager("/tmp"))

	if tool.Name() != "SubscribeMcpResource" {
		t.Errorf("Name() = %q", tool.Name())
	}
	if tool.RequiresPermission(nil) {
		t.Error("SubscribeMcpResource should not require permission")
	}
}

func TestUnsubscribeMcpResourceTool_Schema(t *testing.T) {
	tool := NewUnsubscribeMcpResourceTool(NewManager("/tmp"))

	if tool.Name() != "UnsubscribeMcpResource" {
		t.Errorf("Name() = %q", tool.Name())
	}
}

func TestSubscribePollingTool_Schema(t *testing.T) {
	tool := NewSubscribePollingTool(NewManager("/tmp"))

	if tool.Name() != "SubscribePolling" {
		t.Errorf("Name() = %q", tool.Name())
	}
}

func TestUnsubscribePollingTool_Schema(t *testing.T) {
	tool := NewUnsubscribePollingTool(NewManager("/tmp"))

	if tool.Name() != "UnsubscribePolling" {
		t.Errorf("Name() = %q", tool.Name())
	}
}

func TestSubscriptionStore(t *testing.T) {
	store := &subscriptionStore{
		subs: make(map[string]subscription),
	}

	// Add subscriptions.
	id1 := store.add(subscription{server: "github", uri: "file://a", subType: "resource"})
	id2 := store.add(subscription{server: "github", uri: "file://b", subType: "resource"})
	id3 := store.add(subscription{server: "fs", uri: "file://c", subType: "polling"})

	if id1 == id2 || id2 == id3 {
		t.Error("subscription IDs should be unique")
	}

	// Find by server.
	matches := store.findByServerURI("github", "")
	if len(matches) != 2 {
		t.Errorf("findByServerURI(github, '') = %d matches, want 2", len(matches))
	}

	// Find by server + URI.
	matches = store.findByServerURI("github", "file://a")
	if len(matches) != 1 {
		t.Errorf("findByServerURI(github, file://a) = %d matches, want 1", len(matches))
	}

	// Remove by ID.
	sub, ok := store.remove(id1)
	if !ok {
		t.Error("expected to find subscription")
	}
	if sub.server != "github" {
		t.Errorf("removed sub server = %q", sub.server)
	}

	// Verify removed.
	_, ok = store.remove(id1)
	if ok {
		t.Error("should not find removed subscription")
	}

	// Find all.
	matches = store.findByServerURI("", "")
	if len(matches) != 2 {
		t.Errorf("remaining subscriptions = %d, want 2", len(matches))
	}
}
