# Phase 6: MCP — Integration Guide

Phase 6 adds Model Context Protocol (MCP) support, enabling the CLI to connect
to external tool servers. The new `internal/mcp/` package implements a JSON-RPC
2.0 client over stdio and SSE transports, discovers tools from MCP servers at
startup, and registers them in the existing tool registry so they participate in
the agentic loop identically to built-in tools.

**MCP is an additive layer.** It does not modify the agentic loop, the API
client, the streaming handler, the permission system, or the TUI. It plugs
into the existing `Tool` interface, `Registry`, and `LoopConfig.Tools`
pipeline. The model sees MCP tools alongside built-in tools in every API
request, and invokes them the same way.

---

## What exists today

### The Tool interface (`internal/tools/registry.go`, lines 14–29)

Every tool — built-in or MCP — must satisfy this:

```go
type Tool interface {
    Name() string
    Description() string
    InputSchema() json.RawMessage
    Execute(ctx context.Context, input json.RawMessage) (string, error)
    RequiresPermission(input json.RawMessage) bool
}
```

Key constraint: `Execute` returns `(string, error)`, not structured JSON. MCP
tool wrappers must marshal their result to a string.

### The Registry (`internal/tools/registry.go`)

```go
type Registry struct {
    tools      map[string]Tool
    order      []string
    permission PermissionHandler
}

func (r *Registry) Register(t Tool)
func (r *Registry) Execute(ctx context.Context, name string, input []byte) (string, error)
func (r *Registry) Definitions() []api.ToolDefinition
```

Execution pipeline:
1. Look up tool by name
2. If `tool.RequiresPermission(input)` → call `permission.RequestPermission()`
3. If allowed → call `tool.Execute(ctx, input)`
4. Return result string to the agentic loop

MCP tool wrappers register like any other tool. No special-casing needed.

### Tool definitions flow to the API (`internal/conversation/loop.go`)

```go
req := &api.CreateMessageRequest{
    Messages: l.history.Messages(),
    System:   l.system,
    Tools:    l.tools,  // ← registry.Definitions() result
}
```

The loop stores `tools []api.ToolDefinition` at construction. MCP tool
definitions must be included in this slice. Since `main.go` calls
`registry.Definitions()` after all tools (including MCP) are registered,
this works automatically.

### Sub-agent tool inheritance (`internal/tools/agent.go`, lines 52–67)

```go
type AgentTool struct {
    tools    []api.ToolDefinition      // snapshot of parent's definitions
    toolExec conversation.ToolExecutor  // parent's registry (shared)
}
```

Sub-agents receive the parent's tool definitions and registry reference.
MCP tools registered before the `AgentTool` is created will be visible to
sub-agents automatically, because `registry.Definitions()` is called at
agent creation time (line 167 of `main.go`).

### Configuration (`internal/config/settings.go`)

```go
type Settings struct {
    Permissions []PermissionRule  `json:"permissions,omitempty"`
    Model       string            `json:"model,omitempty"`
    Env         map[string]string `json:"env,omitempty"`
    Hooks       json.RawMessage   `json:"hooks,omitempty"`
    Sandbox     json.RawMessage   `json:"sandbox,omitempty"`
}
```

MCP server configuration lives in separate `.mcp.json` files (not in
`settings.json`). Phase 6 adds a loader for these files and a new
`MCPServers` field or a standalone config type.

### Permission rules (`internal/config/permissions.go`)

Permission rules use glob-like patterns:

```go
type PermissionRule struct {
    Tool    string `json:"tool"`
    Pattern string `json:"pattern,omitempty"`
    Action  string `json:"action"` // "allow", "deny", "ask"
}
```

MCP tools need a naming convention that works with existing pattern matching.
The convention is `mcp__<server>__<tool>` for tool names (matching the
official CLI), which allows rules like:

```json
{"tool": "mcp__github__*", "action": "allow"}
```

---

## MCP tool schemas from `sdk-tools.d.ts`

The official CLI defines these MCP-related tools:

### McpInput / McpOutput

```typescript
// Input: arbitrary — each MCP tool has its own schema
export interface McpInput {
  [k: string]: unknown;
}

// Output: string (MCP tool execution result)
export type McpOutput = string;
```

Each discovered MCP tool becomes a separate `ToolDefinition` with its own
`Name`, `Description`, and `InputSchema` from the server. The model calls
them by name like any other tool.

### ListMcpResources

```typescript
export interface ListMcpResourcesInput {
  server?: string;  // optional filter
}

export type ListMcpResourcesOutput = {
  uri: string;
  name: string;
  mimeType?: string;
  description?: string;
  server: string;
}[];
```

### ReadMcpResource

```typescript
export interface ReadMcpResourceInput {
  server: string;
  uri: string;
}

export interface ReadMcpResourceOutput {
  contents: {
    uri: string;
    mimeType?: string;
    text?: string;
  }[];
}
```

### SubscribeMcpResource / UnsubscribeMcpResource

```typescript
export interface SubscribeMcpResourceInput {
  server: string;
  uri: string;
  reason?: string;
}
export interface SubscribeMcpResourceOutput {
  subscribed: boolean;
  subscriptionId: string;
}

export interface UnsubscribeMcpResourceInput {
  server?: string;
  uri?: string;
  subscriptionId?: string;
}
export interface UnsubscribeMcpResourceOutput {
  unsubscribed: boolean;
}
```

### SubscribePolling / UnsubscribePolling

```typescript
export interface SubscribePollingInput {
  type: "tool" | "resource";
  server: string;
  toolName?: string;
  arguments?: { [k: string]: unknown };
  uri?: string;
  intervalMs: number;  // minimum 1000ms, default 5000ms
  reason?: string;
}
export interface SubscribePollingOutput {
  subscribed: boolean;
  subscriptionId: string;
}

export interface UnsubscribePollingInput {
  subscriptionId?: string;
  server?: string;
  target?: string;
}
export interface UnsubscribePollingOutput {
  unsubscribed: boolean;
}
```

---

## Architecture of `internal/mcp/`

### Proposed file structure

```
internal/mcp/
├── client.go        # MCPClient interface, JSON-RPC request/response
├── stdio.go         # stdio transport (subprocess management)
├── sse.go           # SSE transport (HTTP streaming)
├── types.go         # JSON-RPC 2.0 types, MCP protocol types
├── config.go        # .mcp.json loading from project and user dirs
├── manager.go       # Server lifecycle: init → discover → register → shutdown
└── tools.go         # MCPToolWrapper, ListMcpResources, ReadMcpResource, etc.
```

### Core types (`internal/mcp/types.go`)

```go
// JSON-RPC 2.0

type JSONRPCRequest struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      int64           `json:"id"`
    Method  string          `json:"method"`
    Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      int64           `json:"id"`
    Result  json.RawMessage `json:"result,omitempty"`
    Error   *JSONRPCError   `json:"error,omitempty"`
}

type JSONRPCError struct {
    Code    int             `json:"code"`
    Message string          `json:"message"`
    Data    json.RawMessage `json:"data,omitempty"`
}

// MCP protocol

type ServerConfig struct {
    Command string            `json:"command"`
    Args    []string          `json:"args,omitempty"`
    Env     map[string]string `json:"env,omitempty"`
    URL     string            `json:"url,omitempty"` // for SSE transport
}

type MCPToolDef struct {
    Name        string          `json:"name"`
    Description string          `json:"description,omitempty"`
    InputSchema json.RawMessage `json:"inputSchema"`
}

type MCPResource struct {
    URI         string `json:"uri"`
    Name        string `json:"name"`
    MIMEType    string `json:"mimeType,omitempty"`
    Description string `json:"description,omitempty"`
}

type MCPResourceContent struct {
    URI      string `json:"uri"`
    MIMEType string `json:"mimeType,omitempty"`
    Text     string `json:"text,omitempty"`
}
```

### The Transport interface (`internal/mcp/client.go`)

```go
type Transport interface {
    // Send sends a JSON-RPC request and returns the response.
    Send(ctx context.Context, req *JSONRPCRequest) (*JSONRPCResponse, error)

    // Close shuts down the transport.
    Close() error
}
```

Two implementations:

**StdioTransport** (`stdio.go`): Launches a subprocess via `os/exec.Cmd`,
writes JSON-RPC requests to stdin, reads responses from stdout. Each
request is a single line of JSON followed by `\n`. Responses are read
line-by-line. The subprocess's stderr is captured for error diagnostics.

**SSETransport** (`sse.go`): Connects to an HTTP endpoint. Sends requests
via POST. Reads streaming responses via Server-Sent Events. The event
stream carries JSON-RPC responses as `data:` fields.

### The MCPClient (`internal/mcp/client.go`)

```go
type MCPClient struct {
    transport  Transport
    serverName string
    nextID     int64
    mu         sync.Mutex

    // Capabilities negotiated during initialization.
    capabilities ServerCapabilities
}

func NewMCPClient(serverName string, transport Transport) *MCPClient

func (c *MCPClient) Initialize(ctx context.Context) error
func (c *MCPClient) ListTools(ctx context.Context) ([]MCPToolDef, error)
func (c *MCPClient) CallTool(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error)
func (c *MCPClient) ListResources(ctx context.Context) ([]MCPResource, error)
func (c *MCPClient) ReadResource(ctx context.Context, uri string) ([]MCPResourceContent, error)
func (c *MCPClient) Close() error
```

### The MCP lifecycle

```
1. Load .mcp.json config files
2. For each server config:
   a. Create transport (stdio or SSE based on config fields)
   b. Create MCPClient
   c. Call client.Initialize() — JSON-RPC "initialize" method
   d. Send "notifications/initialized" notification
   e. Call client.ListTools() — JSON-RPC "tools/list" method
   f. For each discovered tool, wrap in MCPToolWrapper
   g. Register wrapper in the tool registry
3. Also register the built-in MCP management tools:
   - ListMcpResources
   - ReadMcpResource
   - SubscribeMcpResource / UnsubscribeMcpResource
   - SubscribePolling / UnsubscribePolling
4. On CLI exit, call client.Close() for each server
```

---

## Integration with existing code

### MCPToolWrapper (`internal/mcp/tools.go`)

This is the bridge between MCP and the `tools.Tool` interface:

```go
type MCPToolWrapper struct {
    serverName  string
    toolName    string
    displayName string       // "mcp__<server>__<tool>"
    description string
    inputSchema json.RawMessage
    client      *MCPClient
}
```

Implementing the `Tool` interface:

```go
func (w *MCPToolWrapper) Name() string {
    return w.displayName  // e.g. "mcp__github__create_issue"
}

func (w *MCPToolWrapper) Description() string {
    return w.description
}

func (w *MCPToolWrapper) InputSchema() json.RawMessage {
    return w.inputSchema  // pass through from server's tools/list
}

func (w *MCPToolWrapper) RequiresPermission(_ json.RawMessage) bool {
    return true  // MCP tools always require permission
}

func (w *MCPToolWrapper) Execute(ctx context.Context, input json.RawMessage) (string, error) {
    result, err := w.client.CallTool(ctx, w.toolName, input)
    if err != nil {
        return "", err
    }
    // Result is JSON — marshal to string for the Tool interface.
    return string(result), nil
}
```

### Manager (`internal/mcp/manager.go`)

Coordinates server lifecycle and exposes a clean API for `main.go`:

```go
type Manager struct {
    clients map[string]*MCPClient  // keyed by server name
    mu      sync.Mutex
}

func NewManager() *Manager

// StartServers loads config, connects to servers, discovers tools,
// and registers them in the provided registry.
func (m *Manager) StartServers(ctx context.Context, configs map[string]ServerConfig, registry *tools.Registry) error

// Shutdown gracefully closes all server connections.
func (m *Manager) Shutdown()

// Servers returns the list of connected server names (for /mcp status).
func (m *Manager) Servers() []string

// Client returns the client for a named server (for resource tools).
func (m *Manager) Client(name string) (*MCPClient, bool)
```

### Config loading (`internal/mcp/config.go`)

```go
type MCPConfig struct {
    MCPServers map[string]ServerConfig `json:"mcpServers"`
}

// LoadMCPConfig loads and merges .mcp.json from project and user dirs.
func LoadMCPConfig(cwd string) (*MCPConfig, error)
```

Load order (lower to higher priority, higher wins per server name):
1. `~/.mcp.json` (user-level)
2. `.mcp.json` (project-level, in cwd)

The merge is per server name: project-level overrides user-level for the
same server name. Servers from both levels that don't conflict are included.

### Changes to `cmd/claude/main.go`

Insert MCP initialization **after** built-in tool registration and
**before** the `AgentTool` creation:

```go
// ... existing tool registration (lines 141–164) ...

// Phase 6: MCP server initialization.
mcpConfig, err := mcp.LoadMCPConfig(cwd)
if err != nil {
    fmt.Fprintf(os.Stderr, "Warning: MCP config error: %v\n", err)
}

var mcpManager *mcp.Manager
if mcpConfig != nil && len(mcpConfig.MCPServers) > 0 {
    mcpManager = mcp.NewManager()
    if err := mcpManager.StartServers(ctx, mcpConfig.MCPServers, registry); err != nil {
        fmt.Fprintf(os.Stderr, "Warning: MCP startup error: %v\n", err)
    }
    defer mcpManager.Shutdown()

    // Register MCP management tools (these need the manager reference).
    registry.Register(mcp.NewListMcpResourcesTool(mcpManager))
    registry.Register(mcp.NewReadMcpResourceTool(mcpManager))
    registry.Register(mcp.NewSubscribeMcpResourceTool(mcpManager))
    registry.Register(mcp.NewUnsubscribeMcpResourceTool(mcpManager))
    registry.Register(mcp.NewSubscribePollingTool(mcpManager))
    registry.Register(mcp.NewUnsubscribePollingTool(mcpManager))
}

// Agent tool registered last — now includes MCP tools in definitions.
agentTool := tools.NewAgentTool(client, system, registry.Definitions(), registry, bgStore)
registry.Register(agentTool)
```

**Ordering matters.** MCP tools must be registered before `AgentTool` so
that `registry.Definitions()` includes them when building the sub-agent's
tool list.

### Changes to `internal/tui/slash.go`

Add an `/mcp` command to the slash registry:

```go
r.register(SlashCommand{
    Name:        "mcp",
    Description: "Show MCP server status",
    Execute: func(m *model) string {
        // m would need a reference to the mcpManager
        // or this can be a simple status listing
        return "MCP server status (not yet implemented)"
    },
})
```

To make the MCP manager accessible to the TUI, add it to `AppConfig`:

```go
type AppConfig struct {
    Loop       *conversation.Loop
    Session    *session.Session
    SessStore  *session.Store
    Version    string
    Model      string
    PrintMode  bool
    MCPManager interface{}  // *mcp.Manager, interface{} to avoid import cycle
}
```

### Permission rules for MCP tools

MCP tools use the naming convention `mcp__<server>__<tool>`. Permission
rules can match these with glob patterns:

```json
[
  {"tool": "mcp__github__*", "action": "allow"},
  {"tool": "mcp__*", "action": "ask"}
]
```

The existing `RuleBasedPermissionHandler` in `config/permissions.go` already
supports glob matching via doublestar, so this works without modification.

---

## JSON-RPC 2.0 protocol details

### Initialize handshake

Request:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": "2024-11-05",
    "capabilities": {},
    "clientInfo": {
      "name": "claude-code",
      "version": "1.0.0"
    }
  }
}
```

Response:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": "2024-11-05",
    "capabilities": {
      "tools": {},
      "resources": {}
    },
    "serverInfo": {
      "name": "server-name",
      "version": "1.0.0"
    }
  }
}
```

Followed by the client sending a notification (no `id`):
```json
{
  "jsonrpc": "2.0",
  "method": "notifications/initialized"
}
```

### Tool discovery

Request:
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/list",
  "params": {}
}
```

Response:
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "tools": [
      {
        "name": "create_issue",
        "description": "Create a GitHub issue",
        "inputSchema": {
          "type": "object",
          "properties": {
            "title": {"type": "string"},
            "body": {"type": "string"}
          },
          "required": ["title"]
        }
      }
    ]
  }
}
```

### Tool execution

Request:
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "create_issue",
    "arguments": {"title": "Bug report", "body": "..."}
  }
}
```

Response:
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "content": [
      {"type": "text", "text": "Issue #42 created successfully"}
    ]
  }
}
```

The `tools/call` result contains a `content` array with text blocks.
`MCPToolWrapper.Execute` should extract and concatenate the text content.

### Resource operations

`resources/list` and `resources/read` follow the same request/response
pattern. These are exposed via the `ListMcpResources` and `ReadMcpResource`
built-in tools, not via MCP tool wrappers.

---

## Transport implementation details

### stdio transport (`internal/mcp/stdio.go`)

```go
type StdioTransport struct {
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    stdout *bufio.Scanner
    stderr bytes.Buffer
    mu     sync.Mutex
}

func NewStdioTransport(command string, args []string, env map[string]string) (*StdioTransport, error)
```

Implementation notes:
- Start the subprocess with `exec.CommandContext` so the process is killed
  on context cancellation
- Set the working directory to the project directory
- Merge server env vars with the current environment (server's `env` field
  overrides)
- Write requests as single JSON lines to stdin
- Read responses as single JSON lines from stdout (buffered scanner)
- Capture stderr for error diagnostics
- Use a mutex around send/receive to serialize requests (MCP servers may
  not support concurrent requests)
- Close() kills the subprocess gracefully (SIGTERM, then SIGKILL after
  timeout)

### SSE transport (`internal/mcp/sse.go`)

```go
type SSETransport struct {
    baseURL string
    client  *http.Client
    mu      sync.Mutex
}

func NewSSETransport(url string) *SSETransport
```

Implementation notes:
- POST requests to the server's endpoint
- Read SSE responses from a long-lived GET connection
- Parse SSE events the same way as `api/streaming.go` (can share or
  duplicate the parser)
- Handle reconnection on connection drop

### Choosing transport

The config determines which transport to use:

```go
func transportForConfig(cfg ServerConfig, cwd string) (Transport, error) {
    if cfg.URL != "" {
        return NewSSETransport(cfg.URL), nil
    }
    return NewStdioTransport(cfg.Command, cfg.Args, cfg.Env, cwd)
}
```

If `url` is set → SSE transport. Otherwise → stdio transport using
`command` and `args`.

---

## Built-in MCP management tools

These are standard `tools.Tool` implementations that live in
`internal/mcp/tools.go` and delegate to the `Manager`:

| Tool | Purpose | Needs |
|------|---------|-------|
| `ListMcpResources` | List resources across all servers | `Manager.Client()` for each server |
| `ReadMcpResource` | Read a resource by server + URI | `Manager.Client(server)` |
| `SubscribeMcpResource` | Subscribe to resource changes | `Manager.Client(server)` + subscription store |
| `UnsubscribeMcpResource` | Unsubscribe from changes | Subscription store |
| `SubscribePolling` | Poll a tool or resource on interval | Goroutine manager for periodic calls |
| `UnsubscribePolling` | Stop polling | Cancel the polling goroutine |

None of these tools require permission (they're resource/subscription
management, not arbitrary execution). The discovered MCP tools themselves
do require permission (set in `MCPToolWrapper.RequiresPermission`).

---

## Error handling

| Failure | Behavior |
|---------|----------|
| `.mcp.json` missing | No MCP servers loaded; silent (not an error) |
| `.mcp.json` parse error | Warning to stderr; continue without MCP |
| Server subprocess fails to start | Warning; skip that server, continue with others |
| `initialize` handshake fails | Warning; skip that server |
| `tools/list` fails | Warning; server registered but with no tools |
| Tool execution fails | Return error to agentic loop (same as built-in tools) |
| Server crashes mid-session | Tool execution returns error; no automatic restart |
| Context cancelled (Ctrl+C) | Subprocess killed via context; graceful shutdown |

---

## Dependencies

**No new external dependencies.** Everything uses the standard library:
- `encoding/json` for JSON-RPC marshaling
- `os/exec` for subprocess management
- `bufio` for line-based stdio I/O
- `net/http` for SSE transport
- `sync` for mutex around transport access
- `context` for cancellation

---

## Files changed

| File | Change |
|------|--------|
| `internal/mcp/types.go` | **New.** JSON-RPC 2.0 types, MCP protocol types |
| `internal/mcp/client.go` | **New.** Transport interface, MCPClient |
| `internal/mcp/stdio.go` | **New.** Subprocess-based stdio transport |
| `internal/mcp/sse.go` | **New.** HTTP SSE transport |
| `internal/mcp/config.go` | **New.** `.mcp.json` loader |
| `internal/mcp/manager.go` | **New.** Server lifecycle management |
| `internal/mcp/tools.go` | **New.** MCPToolWrapper + resource/subscription tools |
| `cmd/claude/main.go` | **Modify.** Add MCP initialization between tool registration and AgentTool creation |
| `internal/tui/slash.go` | **Modify.** Add `/mcp` command |
| `internal/tui/app.go` | **Modify.** Add MCPManager to AppConfig (optional) |

No changes needed to:
- `internal/tools/registry.go` — MCP tools use the existing `Tool` interface
- `internal/conversation/loop.go` — the loop sees MCP tools like any other
- `internal/api/` — no API changes
- `internal/config/permissions.go` — glob matching already handles MCP tool name patterns
- `internal/tui/model.go` — MCP tool calls render through existing stream handler

---

## How to verify

1. **stdio transport** — Create a test `.mcp.json` with a simple MCP server
   (e.g. `npx -y @modelcontextprotocol/server-filesystem`). Verify the CLI
   discovers its tools and can call them.

2. **SSE transport** — Start a local MCP server with HTTP SSE transport.
   Verify tool discovery and execution work.

3. **Tool visibility** — Verify MCP tools appear in the tool definitions
   sent to the API (check with `--dangerously-skip-permissions` and ask the
   model "what tools do you have?").

4. **Sub-agent inheritance** — Trigger an Agent tool call and verify the
   sub-agent can see and use MCP tools.

5. **Permission flow** — Verify MCP tool calls go through the permission
   handler (should show permission prompt in TUI).

6. **Permission rules** — Add a rule like `{"tool": "mcp__*", "action": "allow"}`
   and verify it auto-approves MCP tools.

7. **Server failure** — Start the CLI with a misconfigured MCP server.
   Verify it warns and continues without crashing.

8. **Graceful shutdown** — Verify MCP server subprocesses are cleaned up on
   Ctrl+C.

9. **Resource tools** — With a server that supports resources, verify
   `ListMcpResources` and `ReadMcpResource` work.

10. **/mcp command** — Verify the slash command shows connected servers and
    their tool counts.

When in doubt about protocol details, inspect the official CLI's traffic
with `mitmproxy` or add logging to the JSON-RPC exchange.
