package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

// Transport is the interface for sending JSON-RPC messages to an MCP server.
type Transport interface {
	// Send sends a JSON-RPC request and returns the response.
	Send(ctx context.Context, req *JSONRPCRequest) (*JSONRPCResponse, error)

	// Notify sends a JSON-RPC notification (no response expected).
	Notify(ctx context.Context, req *JSONRPCRequest) error

	// Close shuts down the transport.
	Close() error
}

// MCPClient communicates with a single MCP server over a Transport.
type MCPClient struct {
	transport  Transport
	serverName string
	nextID     atomic.Int64
	mu         sync.Mutex

	// Capabilities negotiated during initialization.
	capabilities ServerCapabilities
	serverInfo   ServerInfo
}

// NewMCPClient creates a new MCP client for the named server.
func NewMCPClient(serverName string, transport Transport) *MCPClient {
	c := &MCPClient{
		transport:  transport,
		serverName: serverName,
	}
	c.nextID.Store(1)
	return c
}

// ServerName returns the configured name of this server.
func (c *MCPClient) ServerName() string {
	return c.serverName
}

// ServerInfo returns the server's self-reported info after initialization.
func (c *MCPClient) ServerInfoResult() ServerInfo {
	return c.serverInfo
}

// Capabilities returns the negotiated server capabilities.
func (c *MCPClient) Capabilities() ServerCapabilities {
	return c.capabilities
}

// Initialize performs the MCP initialization handshake.
func (c *MCPClient) Initialize(ctx context.Context) error {
	params := InitializeParams{
		ProtocolVersion: ProtocolVersion,
		Capabilities:    ClientCapabilities{},
		ClientInfo: ClientInfo{
			Name:    "claude-code",
			Version: "1.0.0",
		},
	}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal initialize params: %w", err)
	}

	resp, err := c.call(ctx, "initialize", paramsJSON)
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	var result InitializeResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("unmarshal initialize result: %w", err)
	}

	c.capabilities = result.Capabilities
	c.serverInfo = result.ServerInfo

	// Send initialized notification.
	notif := &JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	if err := c.transport.Notify(ctx, notif); err != nil {
		return fmt.Errorf("send initialized notification: %w", err)
	}

	return nil
}

// ListTools discovers tools from the server.
func (c *MCPClient) ListTools(ctx context.Context) ([]MCPToolDef, error) {
	paramsJSON, _ := json.Marshal(struct{}{})

	resp, err := c.call(ctx, "tools/list", paramsJSON)
	if err != nil {
		return nil, fmt.Errorf("tools/list: %w", err)
	}

	var result ToolsListResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("unmarshal tools/list result: %w", err)
	}

	return result.Tools, nil
}

// CallTool executes a tool on the server.
func (c *MCPClient) CallTool(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	params := ToolCallParams{
		Name:      name,
		Arguments: args,
	}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal tool call params: %w", err)
	}

	resp, err := c.call(ctx, "tools/call", paramsJSON)
	if err != nil {
		return nil, fmt.Errorf("tools/call %s: %w", name, err)
	}

	return resp, nil
}

// ListResources lists resources from the server.
func (c *MCPClient) ListResources(ctx context.Context) ([]MCPResource, error) {
	paramsJSON, _ := json.Marshal(struct{}{})

	resp, err := c.call(ctx, "resources/list", paramsJSON)
	if err != nil {
		return nil, fmt.Errorf("resources/list: %w", err)
	}

	var result ResourcesListResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("unmarshal resources/list result: %w", err)
	}

	return result.Resources, nil
}

// ReadResource reads a resource from the server.
func (c *MCPClient) ReadResource(ctx context.Context, uri string) ([]MCPResourceContent, error) {
	params := ResourceReadParams{URI: uri}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal resource read params: %w", err)
	}

	resp, err := c.call(ctx, "resources/read", paramsJSON)
	if err != nil {
		return nil, fmt.Errorf("resources/read: %w", err)
	}

	var result ResourceReadResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("unmarshal resources/read result: %w", err)
	}

	return result.Contents, nil
}

// SubscribeResource subscribes to changes on a resource.
func (c *MCPClient) SubscribeResource(ctx context.Context, uri string) error {
	params := ResourceSubscribeParams{URI: uri}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal subscribe params: %w", err)
	}

	_, err = c.call(ctx, "resources/subscribe", paramsJSON)
	if err != nil {
		return fmt.Errorf("resources/subscribe: %w", err)
	}

	return nil
}

// UnsubscribeResource unsubscribes from changes on a resource.
func (c *MCPClient) UnsubscribeResource(ctx context.Context, uri string) error {
	params := ResourceUnsubscribeParams{URI: uri}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal unsubscribe params: %w", err)
	}

	_, err = c.call(ctx, "resources/unsubscribe", paramsJSON)
	if err != nil {
		return fmt.Errorf("resources/unsubscribe: %w", err)
	}

	return nil
}

// Close shuts down the transport.
func (c *MCPClient) Close() error {
	return c.transport.Close()
}

// call sends a JSON-RPC request and returns the result payload.
func (c *MCPClient) call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := c.nextID.Add(1) - 1
	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  params,
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	return resp.Result, nil
}
