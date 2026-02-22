package mcp

import (
	"encoding/json"
	"testing"
)

func TestJSONRPCRequestMarshal(t *testing.T) {
	id := int64(1)
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion":"2024-11-05"}`),
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if parsed["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v, want 2.0", parsed["jsonrpc"])
	}
	if parsed["method"] != "initialize" {
		t.Errorf("method = %v, want initialize", parsed["method"])
	}
	if parsed["id"].(float64) != 1 {
		t.Errorf("id = %v, want 1", parsed["id"])
	}
}

func TestJSONRPCNotificationMarshal(t *testing.T) {
	// Notifications have no ID.
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if _, ok := parsed["id"]; ok {
		t.Error("notification should not have 'id' field")
	}
}

func TestJSONRPCResponseUnmarshal(t *testing.T) {
	data := `{
		"jsonrpc": "2.0",
		"id": 1,
		"result": {"tools": [{"name": "test_tool", "description": "A test", "inputSchema": {"type":"object"}}]}
	}`

	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if resp.Error != nil {
		t.Error("expected no error in response")
	}
	if resp.Result == nil {
		t.Fatal("expected result in response")
	}
}

func TestJSONRPCErrorResponse(t *testing.T) {
	data := `{
		"jsonrpc": "2.0",
		"id": 1,
		"error": {"code": -32601, "message": "Method not found"}
	}`

	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error in response")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("error code = %d, want -32601", resp.Error.Code)
	}
	if resp.Error.Message != "Method not found" {
		t.Errorf("error message = %q, want %q", resp.Error.Message, "Method not found")
	}
	if resp.Error.Error() != "Method not found" {
		t.Errorf("Error() = %q, want %q", resp.Error.Error(), "Method not found")
	}
}

func TestServerConfigMarshal(t *testing.T) {
	cfg := ServerConfig{
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-filesystem"},
		Env:     map[string]string{"HOME": "/tmp"},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed ServerConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if parsed.Command != "npx" {
		t.Errorf("Command = %q, want %q", parsed.Command, "npx")
	}
	if len(parsed.Args) != 2 {
		t.Errorf("Args len = %d, want 2", len(parsed.Args))
	}
}

func TestMCPConfigMarshal(t *testing.T) {
	cfg := MCPConfig{
		MCPServers: map[string]ServerConfig{
			"github": {
				Command: "npx",
				Args:    []string{"-y", "@modelcontextprotocol/server-github"},
				Env:     map[string]string{"GITHUB_TOKEN": "tok"},
			},
			"remote": {
				URL: "https://mcp.example.com/sse",
			},
		},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed MCPConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(parsed.MCPServers) != 2 {
		t.Errorf("MCPServers len = %d, want 2", len(parsed.MCPServers))
	}
	if parsed.MCPServers["github"].Command != "npx" {
		t.Errorf("github command = %q, want %q", parsed.MCPServers["github"].Command, "npx")
	}
	if parsed.MCPServers["remote"].URL != "https://mcp.example.com/sse" {
		t.Errorf("remote URL = %q, want %q", parsed.MCPServers["remote"].URL, "https://mcp.example.com/sse")
	}
}

func TestToolCallResultUnmarshal(t *testing.T) {
	data := `{
		"content": [
			{"type": "text", "text": "Issue #42 created"},
			{"type": "text", "text": "successfully"}
		]
	}`

	var result ToolCallResult
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(result.Content) != 2 {
		t.Fatalf("Content len = %d, want 2", len(result.Content))
	}
	if result.Content[0].Text != "Issue #42 created" {
		t.Errorf("Content[0].Text = %q", result.Content[0].Text)
	}
}

func TestInitializeParamsMarshal(t *testing.T) {
	params := InitializeParams{
		ProtocolVersion: ProtocolVersion,
		Capabilities:    ClientCapabilities{},
		ClientInfo: ClientInfo{
			Name:    "claude-code",
			Version: "1.0.0",
		},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if parsed["protocolVersion"] != ProtocolVersion {
		t.Errorf("protocolVersion = %v, want %v", parsed["protocolVersion"], ProtocolVersion)
	}

	clientInfo := parsed["clientInfo"].(map[string]interface{})
	if clientInfo["name"] != "claude-code" {
		t.Errorf("clientInfo.name = %v", clientInfo["name"])
	}
}
