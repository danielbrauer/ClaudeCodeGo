package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MCPToolWrapper bridges an MCP server tool to the tools.Tool interface.
// It is registered in the tool registry like any built-in tool.
type MCPToolWrapper struct {
	serverName  string
	toolName    string
	displayName string // "mcp__<server>__<tool>"
	description string
	inputSchema json.RawMessage
	client      *MCPClient
}

// NewMCPToolWrapper creates a wrapper for a discovered MCP tool.
func NewMCPToolWrapper(serverName string, def MCPToolDef, client *MCPClient) *MCPToolWrapper {
	return &MCPToolWrapper{
		serverName:  serverName,
		toolName:    def.Name,
		displayName: fmt.Sprintf("mcp__%s__%s", serverName, def.Name),
		description: def.Description,
		inputSchema: def.InputSchema,
		client:      client,
	}
}

func (w *MCPToolWrapper) Name() string             { return w.displayName }
func (w *MCPToolWrapper) Description() string       { return w.description }
func (w *MCPToolWrapper) InputSchema() json.RawMessage { return w.inputSchema }

func (w *MCPToolWrapper) RequiresPermission(_ json.RawMessage) bool {
	return true // MCP tools always require permission.
}

func (w *MCPToolWrapper) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	result, err := w.client.CallTool(ctx, w.toolName, input)
	if err != nil {
		return "", err
	}

	// Parse the tool call result and extract text content.
	var callResult ToolCallResult
	if err := json.Unmarshal(result, &callResult); err != nil {
		// If it's not a structured result, return the raw JSON as string.
		return string(result), nil
	}

	if callResult.IsError {
		texts := extractTexts(callResult.Content)
		return texts, fmt.Errorf("MCP tool error: %s", texts)
	}

	return extractTexts(callResult.Content), nil
}

// extractTexts concatenates all text content blocks into a single string.
func extractTexts(content []ToolResultContent) string {
	var parts []string
	for _, c := range content {
		if c.Type == "text" && c.Text != "" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// --- Built-in MCP management tools ---
// These implement the tools.Tool interface and delegate to the Manager.

// ListMcpResourcesTool lists resources across all connected MCP servers.
type ListMcpResourcesTool struct {
	manager *Manager
}

func NewListMcpResourcesTool(manager *Manager) *ListMcpResourcesTool {
	return &ListMcpResourcesTool{manager: manager}
}

func (t *ListMcpResourcesTool) Name() string { return "ListMcpResources" }

func (t *ListMcpResourcesTool) Description() string {
	return "List resources available from MCP servers."
}

func (t *ListMcpResourcesTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"server": {
				"type": "string",
				"description": "Optional server name to filter by."
			}
		}
	}`)
}

func (t *ListMcpResourcesTool) RequiresPermission(_ json.RawMessage) bool {
	return false // Resource listing is read-only.
}

func (t *ListMcpResourcesTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Server string `json:"server"`
	}
	if len(input) > 0 {
		json.Unmarshal(input, &params)
	}

	type resourceEntry struct {
		URI         string `json:"uri"`
		Name        string `json:"name"`
		MIMEType    string `json:"mimeType,omitempty"`
		Description string `json:"description,omitempty"`
		Server      string `json:"server"`
	}

	var allResources []resourceEntry

	servers := t.manager.Servers()
	for _, name := range servers {
		if params.Server != "" && params.Server != name {
			continue
		}

		client, ok := t.manager.Client(name)
		if !ok {
			continue
		}

		resources, err := client.ListResources(ctx)
		if err != nil {
			continue // Skip servers that don't support resources.
		}

		for _, r := range resources {
			allResources = append(allResources, resourceEntry{
				URI:         r.URI,
				Name:        r.Name,
				MIMEType:    r.MIMEType,
				Description: r.Description,
				Server:      name,
			})
		}
	}

	result, _ := json.Marshal(allResources)
	return string(result), nil
}

// ReadMcpResourceTool reads a specific resource from an MCP server.
type ReadMcpResourceTool struct {
	manager *Manager
}

func NewReadMcpResourceTool(manager *Manager) *ReadMcpResourceTool {
	return &ReadMcpResourceTool{manager: manager}
}

func (t *ReadMcpResourceTool) Name() string { return "ReadMcpResource" }

func (t *ReadMcpResourceTool) Description() string {
	return "Read a resource from an MCP server by server name and URI."
}

func (t *ReadMcpResourceTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"server": {
				"type": "string",
				"description": "The MCP server name."
			},
			"uri": {
				"type": "string",
				"description": "The resource URI to read."
			}
		},
		"required": ["server", "uri"]
	}`)
}

func (t *ReadMcpResourceTool) RequiresPermission(_ json.RawMessage) bool {
	return false // Resource reading is read-only.
}

func (t *ReadMcpResourceTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Server string `json:"server"`
		URI    string `json:"uri"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	client, ok := t.manager.Client(params.Server)
	if !ok {
		return "", fmt.Errorf("MCP server %q not found", params.Server)
	}

	contents, err := client.ReadResource(ctx, params.URI)
	if err != nil {
		return "", err
	}

	type outputContent struct {
		URI      string `json:"uri"`
		MIMEType string `json:"mimeType,omitempty"`
		Text     string `json:"text,omitempty"`
	}

	output := struct {
		Contents []outputContent `json:"contents"`
	}{}

	for _, c := range contents {
		output.Contents = append(output.Contents, outputContent{
			URI:      c.URI,
			MIMEType: c.MIMEType,
			Text:     c.Text,
		})
	}

	result, _ := json.Marshal(output)
	return string(result), nil
}

// SubscribeMcpResourceTool subscribes to resource changes.
type SubscribeMcpResourceTool struct {
	manager       *Manager
	subscriptions *subscriptionStore
}

func NewSubscribeMcpResourceTool(manager *Manager) *SubscribeMcpResourceTool {
	return &SubscribeMcpResourceTool{
		manager:       manager,
		subscriptions: globalSubscriptionStore,
	}
}

func (t *SubscribeMcpResourceTool) Name() string { return "SubscribeMcpResource" }

func (t *SubscribeMcpResourceTool) Description() string {
	return "Subscribe to changes on an MCP resource."
}

func (t *SubscribeMcpResourceTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"server": {
				"type": "string",
				"description": "The MCP server name."
			},
			"uri": {
				"type": "string",
				"description": "The resource URI to subscribe to."
			},
			"reason": {
				"type": "string",
				"description": "Optional reason for the subscription."
			}
		},
		"required": ["server", "uri"]
	}`)
}

func (t *SubscribeMcpResourceTool) RequiresPermission(_ json.RawMessage) bool {
	return false
}

func (t *SubscribeMcpResourceTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Server string `json:"server"`
		URI    string `json:"uri"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	client, ok := t.manager.Client(params.Server)
	if !ok {
		return "", fmt.Errorf("MCP server %q not found", params.Server)
	}

	if err := client.SubscribeResource(ctx, params.URI); err != nil {
		return "", err
	}

	subID := t.subscriptions.add(subscription{
		server: params.Server,
		uri:    params.URI,
		subType: "resource",
	})

	result, _ := json.Marshal(struct {
		Subscribed     bool   `json:"subscribed"`
		SubscriptionID string `json:"subscriptionId"`
	}{
		Subscribed:     true,
		SubscriptionID: subID,
	})

	return string(result), nil
}

// UnsubscribeMcpResourceTool unsubscribes from resource changes.
type UnsubscribeMcpResourceTool struct {
	manager       *Manager
	subscriptions *subscriptionStore
}

func NewUnsubscribeMcpResourceTool(manager *Manager) *UnsubscribeMcpResourceTool {
	return &UnsubscribeMcpResourceTool{
		manager:       manager,
		subscriptions: globalSubscriptionStore,
	}
}

func (t *UnsubscribeMcpResourceTool) Name() string { return "UnsubscribeMcpResource" }

func (t *UnsubscribeMcpResourceTool) Description() string {
	return "Unsubscribe from changes on an MCP resource."
}

func (t *UnsubscribeMcpResourceTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"subscriptionId": {
				"type": "string",
				"description": "The subscription ID to cancel."
			},
			"server": {
				"type": "string",
				"description": "Optional server name to match."
			},
			"uri": {
				"type": "string",
				"description": "Optional URI to match."
			}
		}
	}`)
}

func (t *UnsubscribeMcpResourceTool) RequiresPermission(_ json.RawMessage) bool {
	return false
}

func (t *UnsubscribeMcpResourceTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		SubscriptionID string `json:"subscriptionId"`
		Server         string `json:"server"`
		URI            string `json:"uri"`
	}
	if len(input) > 0 {
		json.Unmarshal(input, &params)
	}

	removed := false

	if params.SubscriptionID != "" {
		sub, ok := t.subscriptions.remove(params.SubscriptionID)
		if ok {
			if client, ok := t.manager.Client(sub.server); ok {
				client.UnsubscribeResource(ctx, sub.uri)
			}
			removed = true
		}
	} else {
		// Remove by server + URI match.
		for _, id := range t.subscriptions.findByServerURI(params.Server, params.URI) {
			sub, ok := t.subscriptions.remove(id)
			if ok {
				if client, ok := t.manager.Client(sub.server); ok {
					client.UnsubscribeResource(ctx, sub.uri)
				}
				removed = true
			}
		}
	}

	result, _ := json.Marshal(struct {
		Unsubscribed bool `json:"unsubscribed"`
	}{
		Unsubscribed: removed,
	})

	return string(result), nil
}

// SubscribePollingTool polls a tool or resource at a regular interval.
type SubscribePollingTool struct {
	manager       *Manager
	subscriptions *subscriptionStore
}

func NewSubscribePollingTool(manager *Manager) *SubscribePollingTool {
	return &SubscribePollingTool{
		manager:       manager,
		subscriptions: globalSubscriptionStore,
	}
}

func (t *SubscribePollingTool) Name() string { return "SubscribePolling" }

func (t *SubscribePollingTool) Description() string {
	return "Poll an MCP tool or resource at a regular interval."
}

func (t *SubscribePollingTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"type": {
				"type": "string",
				"enum": ["tool", "resource"],
				"description": "Whether to poll a tool or resource."
			},
			"server": {
				"type": "string",
				"description": "The MCP server name."
			},
			"toolName": {
				"type": "string",
				"description": "Tool name (when type is 'tool')."
			},
			"arguments": {
				"type": "object",
				"description": "Tool arguments (when type is 'tool')."
			},
			"uri": {
				"type": "string",
				"description": "Resource URI (when type is 'resource')."
			},
			"intervalMs": {
				"type": "integer",
				"description": "Polling interval in milliseconds (minimum 1000, default 5000)."
			},
			"reason": {
				"type": "string",
				"description": "Optional reason for polling."
			}
		},
		"required": ["type", "server", "intervalMs"]
	}`)
}

func (t *SubscribePollingTool) RequiresPermission(_ json.RawMessage) bool {
	return false
}

func (t *SubscribePollingTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Type       string          `json:"type"`
		Server     string          `json:"server"`
		ToolName   string          `json:"toolName"`
		Arguments  json.RawMessage `json:"arguments"`
		URI        string          `json:"uri"`
		IntervalMs int             `json:"intervalMs"`
		Reason     string          `json:"reason"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	if params.IntervalMs < 1000 {
		params.IntervalMs = 5000
	}

	_, ok := t.manager.Client(params.Server)
	if !ok {
		return "", fmt.Errorf("MCP server %q not found", params.Server)
	}

	// Create a cancellable context for the polling goroutine.
	pollCtx, cancel := context.WithCancel(context.Background())

	subID := t.subscriptions.add(subscription{
		server:  params.Server,
		uri:     params.URI,
		subType: "polling",
		cancel:  cancel,
	})

	// Start polling in a goroutine.
	go func() {
		ticker := time.NewTicker(time.Duration(params.IntervalMs) * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-pollCtx.Done():
				return
			case <-ticker.C:
				client, ok := t.manager.Client(params.Server)
				if !ok {
					return
				}

				switch params.Type {
				case "tool":
					client.CallTool(pollCtx, params.ToolName, params.Arguments)
				case "resource":
					client.ReadResource(pollCtx, params.URI)
				}
			}
		}
	}()

	result, _ := json.Marshal(struct {
		Subscribed     bool   `json:"subscribed"`
		SubscriptionID string `json:"subscriptionId"`
	}{
		Subscribed:     true,
		SubscriptionID: subID,
	})

	return string(result), nil
}

// UnsubscribePollingTool stops polling.
type UnsubscribePollingTool struct {
	subscriptions *subscriptionStore
}

func NewUnsubscribePollingTool(manager *Manager) *UnsubscribePollingTool {
	return &UnsubscribePollingTool{
		subscriptions: globalSubscriptionStore,
	}
}

func (t *UnsubscribePollingTool) Name() string { return "UnsubscribePolling" }

func (t *UnsubscribePollingTool) Description() string {
	return "Stop polling an MCP tool or resource."
}

func (t *UnsubscribePollingTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"subscriptionId": {
				"type": "string",
				"description": "The subscription ID to cancel."
			},
			"server": {
				"type": "string",
				"description": "Optional server name to match."
			},
			"target": {
				"type": "string",
				"description": "Optional target to match."
			}
		}
	}`)
}

func (t *UnsubscribePollingTool) RequiresPermission(_ json.RawMessage) bool {
	return false
}

func (t *UnsubscribePollingTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var params struct {
		SubscriptionID string `json:"subscriptionId"`
		Server         string `json:"server"`
		Target         string `json:"target"`
	}
	if len(input) > 0 {
		json.Unmarshal(input, &params)
	}

	removed := false

	if params.SubscriptionID != "" {
		sub, ok := t.subscriptions.remove(params.SubscriptionID)
		if ok {
			if sub.cancel != nil {
				sub.cancel()
			}
			removed = true
		}
	} else {
		// Remove by server match.
		for _, id := range t.subscriptions.findByServerURI(params.Server, params.Target) {
			sub, ok := t.subscriptions.remove(id)
			if ok {
				if sub.cancel != nil {
					sub.cancel()
				}
				removed = true
			}
		}
	}

	result, _ := json.Marshal(struct {
		Unsubscribed bool `json:"unsubscribed"`
	}{
		Unsubscribed: removed,
	})

	return string(result), nil
}

// --- Subscription store ---

type subscription struct {
	server  string
	uri     string
	subType string // "resource" or "polling"
	cancel  context.CancelFunc
}

type subscriptionStore struct {
	mu    sync.Mutex
	subs  map[string]subscription
	nextID atomic.Int64
}

var globalSubscriptionStore = &subscriptionStore{
	subs: make(map[string]subscription),
}

func (s *subscriptionStore) add(sub subscription) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := fmt.Sprintf("sub_%d", s.nextID.Add(1))
	s.subs[id] = sub
	return id
}

func (s *subscriptionStore) remove(id string) (subscription, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sub, ok := s.subs[id]
	if ok {
		delete(s.subs, id)
	}
	return sub, ok
}

func (s *subscriptionStore) findByServerURI(server, uri string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var ids []string
	for id, sub := range s.subs {
		if (server == "" || sub.server == server) && (uri == "" || sub.uri == uri) {
			ids = append(ids, id)
		}
	}
	return ids
}
