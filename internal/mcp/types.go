// Package mcp implements a Model Context Protocol (MCP) client for connecting
// to external tool servers via JSON-RPC 2.0 over stdio or SSE transports.
package mcp

import "encoding/json"

// JSON-RPC 2.0 types.

// JSONRPCRequest is a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"` // nil for notifications
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse is a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError is the error object in a JSON-RPC 2.0 response.
type JSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *JSONRPCError) Error() string {
	return e.Message
}

// MCP server configuration types.

// ServerConfig describes how to connect to an MCP server.
type ServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"` // for SSE transport
}

// MCPConfig is the top-level .mcp.json structure.
type MCPConfig struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

// MCP protocol types.

// InitializeParams are sent by the client in the "initialize" request.
type InitializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      ClientInfo         `json:"clientInfo"`
}

// ClientCapabilities advertises what the client supports.
type ClientCapabilities struct{}

// ClientInfo identifies the client.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResult is the server's response to "initialize".
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
}

// ServerCapabilities advertises what the server supports.
type ServerCapabilities struct {
	Tools     *ToolsCapability     `json:"tools,omitempty"`
	Resources *ResourceCapability  `json:"resources,omitempty"`
}

// ToolsCapability indicates the server supports tools.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourceCapability indicates the server supports resources.
type ResourceCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// ServerInfo identifies the server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// ToolsListResult is the response to "tools/list".
type ToolsListResult struct {
	Tools []MCPToolDef `json:"tools"`
}

// MCPToolDef describes a tool provided by an MCP server.
type MCPToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToolCallParams are sent in a "tools/call" request.
type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// ToolCallResult is the response to "tools/call".
type ToolCallResult struct {
	Content []ToolResultContent `json:"content"`
	IsError bool                `json:"isError,omitempty"`
}

// ToolResultContent is a content block in a tool call result.
type ToolResultContent struct {
	Type string `json:"type"` // "text", "image", "resource"
	Text string `json:"text,omitempty"`
}

// ResourcesListResult is the response to "resources/list".
type ResourcesListResult struct {
	Resources []MCPResource `json:"resources"`
}

// MCPResource describes a resource provided by an MCP server.
type MCPResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	MIMEType    string `json:"mimeType,omitempty"`
	Description string `json:"description,omitempty"`
}

// ResourceReadParams are sent in a "resources/read" request.
type ResourceReadParams struct {
	URI string `json:"uri"`
}

// ResourceReadResult is the response to "resources/read".
type ResourceReadResult struct {
	Contents []MCPResourceContent `json:"contents"`
}

// MCPResourceContent is a content block in a resource read result.
type MCPResourceContent struct {
	URI      string `json:"uri"`
	MIMEType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

// ResourceSubscribeParams are sent in a "resources/subscribe" request.
type ResourceSubscribeParams struct {
	URI string `json:"uri"`
}

// ResourceUnsubscribeParams are sent in a "resources/unsubscribe" request.
type ResourceUnsubscribeParams struct {
	URI string `json:"uri"`
}

// MCP protocol version.
const ProtocolVersion = "2024-11-05"
