# ClaudeCodeGo

A drop-in Go reimplementation of the Claude Code CLI (`@anthropic-ai/claude-code`).

## Project Goal

Reimplement the official Claude Code CLI in Go with full feature parity. The binary should be a complete replacement: same commands, same tools, same config file locations, same behavior. Users should be able to swap it in without changing their workflows.

Key constraint: this targets **Claude subscription auth** (Pro/Team/Enterprise via OAuth), not API key auth. We talk to the same backend the official CLI uses, which is distinct from the public `api.anthropic.com` endpoint.

## Reference Source

The `claude-code-source/` directory contains the official CLI extracted from npm (`@anthropic-ai/claude-code` v2.1.50):

- **`cli.js`** (587K lines prettified) -- the entire bundled application
- **`sdk-tools.d.ts`** (87KB) -- TypeScript type definitions for all tool inputs/outputs
- **`package.json`** -- version and dependency metadata

`sdk-tools.d.ts` is the authoritative reference for tool interfaces. All Go tool implementations must match these schemas exactly.

## Reverse Engineering Guide

The official CLI is a bundled JS application. Understanding its internals is necessary for protocol-level compatibility.

### Studying cli.js

The file is prettified (not minified) so it's searchable. Key areas to investigate:

**Authentication / OAuth flow:**
- Search for `oauth`, `authorize`, `token`, `refresh`, `login`, `claude.ai`, `session`
- Look for browser-open logic (the CLI opens a browser for OAuth consent)
- Find the token storage location (likely `~/.claude/` somewhere)
- Identify the OAuth client ID, redirect URI, and token endpoint
- Understand token refresh logic and expiration handling

**API endpoints and base URLs:**
- Search for `api.`, `console.`, `/v1/`, `messages`, `completions`
- The subscription path likely uses a different base URL or auth header format than the public API
- Look for feature flags or conditional paths based on auth type
- Find how the CLI determines which endpoint to use based on the auth method

**Request/response format:**
- Search for `messages.create`, `stream`, `content_block`, `tool_use`, `tool_result`
- Understand how tool calls and results are serialized in the conversation
- Look for system prompt construction logic
- Find how CLAUDE.md content is injected into the system prompt

**Streaming / SSE:**
- Search for `EventSource`, `text/event-stream`, `data:`, `event:`
- Understand the SSE event types: `message_start`, `content_block_start`, `content_block_delta`, `content_block_stop`, `message_delta`, `message_stop`
- Find how partial JSON tool calls are assembled from deltas

**Session persistence:**
- Search for `session`, `conversation`, `history`, `save`, `resume`, `compact`
- Find the session storage format and location (likely `~/.claude/sessions/`)
- Understand how context compaction works (summarization when context fills up)

**Permission system:**
- Search for `permission`, `allow`, `deny`, `ask`, `sandbox`
- Understand how permission rules are matched against tool calls
- Find the permission prompt UI logic

**Tool dispatch:**
- Search for tool names: `Bash`, `Read`, `Write`, `Edit`, `Glob`, `Grep`, `Agent`, `WebFetch`, etc.
- Understand how the tool registry works
- Find tool execution, timeout, and error handling patterns

### Network Inspection

For protocol-level understanding, intercept actual CLI traffic:

1. Use `mitmproxy` or Charles Proxy with HTTPS interception
2. Set `HTTPS_PROXY` and trust the CA cert
3. Run the official CLI and capture:
   - The OAuth flow (browser redirect, token exchange)
   - API request format (headers, body structure)
   - Streaming response format
   - Any non-obvious API calls (telemetry, session sync, etc.)

### Key Strings to Search in cli.js

```
"Authorization"       -- how auth headers are set
"Bearer"              -- token format
"x-api-key"           -- API key header (for reference, though we target subscription)
"anthropic-version"   -- API version header
"claude-"             -- model identifiers
"/v1/messages"        -- messages API path
"system"              -- system prompt construction
"tool_use"            -- tool call handling
"tool_result"         -- tool result handling
"checkpoint"          -- git checkpoint system
".claude/"            -- config/data directory references
"settings.json"       -- config file handling
"CLAUDE.md"           -- memory file loading
"mcp"                 -- MCP protocol handling
"hook"                -- hooks system
"skill"               -- skills system
"worktree"            -- git worktree handling
```

## Architecture

### High-Level Components

```
┌─────────────────────────────────────────────────────┐
│                    cmd/claude                         │
│                  (CLI entry point)                    │
├─────────────────────────────────────────────────────┤
│                     TUI Layer                        │
│         (terminal UI, markdown rendering,            │
│          diff display, input handling)               │
├─────────────────────────────────────────────────────┤
│                   Agentic Loop                       │
│        (conversation orchestration, tool             │
│         dispatch, context management)                │
├──────────┬──────────┬───────────┬───────────────────┤
│  Tools   │  Config  │  Session  │   Permissions     │
│ (Bash,   │ (settings│ (persist, │   (rules,         │
│  Read,   │  hierarchy│ resume,  │    ask/allow/     │
│  Edit,   │  CLAUDE.md│ compact) │    deny)          │
│  Write,  │  loading)│          │                    │
│  Glob,   │         │          │                    │
│  Grep,   │         │          │                    │
│  etc.)   │         │          │                    │
├──────────┴──────────┴───────────┴───────────────────┤
│                    API Client                        │
│         (HTTP, SSE streaming, auth)                  │
├─────────────────────────────────────────────────────┤
│                   Auth (OAuth)                       │
│        (subscription login, token storage,           │
│         refresh, browser flow)                       │
└─────────────────────────────────────────────────────┘
```

### The Agentic Loop

The core of Claude Code is a loop:

1. **Build messages** -- assemble system prompt (with CLAUDE.md, context), conversation history, and any new user input
2. **Call API** -- send messages to the Claude API with tool definitions, stream the response
3. **Process response** -- handle text output (display to user) and tool_use blocks
4. **Execute tools** -- run each requested tool, collect results
5. **Append results** -- add assistant message and tool results to conversation history
6. **Repeat** -- if the assistant used tools, loop back to step 1; if it produced only text (stop reason "end_turn"), wait for next user input

The loop must handle:
- **Streaming**: display text tokens as they arrive, show tool calls in progress
- **Parallel tool calls**: the API may request multiple tools in one response
- **Interruption**: user can cancel mid-stream (Ctrl+C) or mid-tool-execution
- **Context limits**: when conversation history approaches the model's context window, trigger compaction (summarize older messages)
- **Error recovery**: API errors, tool failures, timeouts

### Tool System

Tools are defined as JSON schemas sent to the API. The API responds with `tool_use` content blocks. Each tool has:
- **Input schema** -- JSON schema describing parameters (see `sdk-tools.d.ts`)
- **Output schema** -- structure of the result sent back to the API
- **Execution logic** -- the Go implementation
- **Permission requirements** -- whether user approval is needed

Tool registry pattern:
```go
type Tool interface {
    Name() string
    Description() string
    InputSchema() json.RawMessage
    Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
    RequiresPermission(input json.RawMessage) bool
}
```

### Built-in Tools

From `sdk-tools.d.ts`, the complete tool set:

| Tool | Purpose |
|------|---------|
| **Bash** | Execute shell commands with timeout, background support |
| **FileRead** (Read) | Read files with offset/limit, images, PDFs, notebooks |
| **FileEdit** (Edit) | String replacement in files, with replace_all |
| **FileWrite** (Write) | Create or overwrite files |
| **Glob** | File pattern matching (like `find` by name) |
| **Grep** | Content search via ripgrep-compatible regex |
| **Agent** (Task) | Spawn sub-agents with isolated context |
| **TodoWrite** | Manage a structured task list |
| **WebFetch** | Fetch URL content and process with a prompt |
| **WebSearch** | Web search |
| **AskUserQuestion** | Ask the user structured questions |
| **NotebookEdit** | Edit Jupyter notebook cells |
| **ExitPlanMode** | Signal completion of a plan |
| **TaskOutput** | Read output from background tasks |
| **TaskStop** | Stop background tasks |
| **Config** | Get/set configuration values |
| **EnterWorktree** | Create isolated git worktree |
| **MCP tools** | ListMcpResources, McpInput, ReadMcpResource, Subscribe/Unsubscribe |

## Authentication

### OAuth Subscription Flow

The CLI authenticates via Claude subscription (Pro/Team/Enterprise):

1. User runs `claude` for the first time
2. CLI opens browser to Claude's OAuth authorization endpoint
3. User logs in / consents
4. Browser redirects to a local callback (CLI runs a temporary HTTP server)
5. CLI exchanges the authorization code for access + refresh tokens
6. Tokens are stored locally (likely `~/.claude/credentials` or similar)
7. Subsequent runs use the stored tokens, refreshing as needed

**To reverse-engineer**: examine cli.js for the OAuth client configuration, endpoint URLs, scopes, and token storage format. Network inspection of an actual login flow will confirm the details.

### Token Management

- Store tokens securely in `~/.claude/`
- Implement automatic refresh before expiration
- Handle refresh failure gracefully (re-prompt login)
- Match the official CLI's storage format for interoperability

## Configuration System

### Settings Hierarchy (highest priority first)

1. **Managed** -- `/etc/claude/` or system-level (deployed by IT)
2. **Command-line flags** -- temporary session overrides
3. **Local** -- `.claude/settings.local.json` (personal, gitignored)
4. **Project** -- `.claude/settings.json` (team-shared, committed)
5. **User** -- `~/.claude/settings.json` (all projects)

Each level can set:
- `permissions` -- allow/deny/ask rules for tool access
- `model` -- default model
- `env` -- environment variables to inject
- `hooks` -- lifecycle event scripts
- `sandbox` -- sandboxing configuration

### CLAUDE.md Loading

CLAUDE.md files are loaded from multiple locations and merged:
- `~/.claude/CLAUDE.md` -- user-level (all projects)
- Walk from filesystem root to CWD, loading any `CLAUDE.md` found
- `.claude/CLAUDE.md` -- project-level
- Support `@path` imports for modular rules
- Support `.claude/rules/` directory for rule files

Content is injected into the system prompt.

### Permission Rules

Rules match tool calls with glob-like patterns:
```
Bash                          # all bash commands
Bash(npm run *)               # specific patterns
Read(./.env)                  # specific file paths
WebFetch(domain:example.com)  # domain restrictions
```

## Session Management

### Persistence

- Sessions stored in `~/.claude/sessions/` (or wherever the official CLI stores them)
- Each session: conversation history, tool results, metadata
- Support `claude -c` (continue last) and `claude -r <id>` (resume specific)
- Match the official format so sessions are interoperable

### Context Compaction

When conversation history approaches the context window limit:
1. Identify older messages that can be summarized
2. Call the API to generate a summary
3. Replace the detailed messages with the summary
4. Preserve recent messages and any active tool state

### Git Checkpoints

Before file modifications, create automatic git snapshots:
- Stash or commit current state
- Allow reverting to pre-edit state
- Integrate with the permission system (checkpoint before approved edits)

## Streaming

### SSE Parsing

The Messages API streams responses as Server-Sent Events:

```
event: message_start
data: {"type":"message_start","message":{...}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{...}}

event: message_stop
data: {"type":"message_stop"}
```

Key implementation details:
- Parse SSE line-by-line from the HTTP response body
- Handle `event:` and `data:` fields
- Assemble text deltas into complete text blocks
- Assemble `input_json_delta` into complete tool call JSON
- Handle multiple content blocks (text + tool_use interleaved)
- Track token usage from `message_start` and `message_delta`

### Tool Call Assembly

Tool calls arrive as incremental JSON string deltas:
```
content_block_start: {"type":"tool_use","id":"...","name":"Bash","input":{}}
content_block_delta: {"type":"input_json_delta","partial_json":"{\"comma"}
content_block_delta: {"type":"input_json_delta","partial_json":"nd\":\"ls\"}"}
content_block_stop: (assemble and parse complete JSON)
```

## MCP (Model Context Protocol)

### Overview

MCP enables connecting to external tool servers. The CLI acts as an MCP client.

### Transport

Two transport modes:
- **stdio**: launch a subprocess, communicate via stdin/stdout JSON-RPC
- **SSE**: connect to an HTTP server streaming JSON-RPC over SSE

### Configuration

MCP servers configured in:
- `.mcp.json` (project-level)
- `~/.mcp.json` (user-level)

Format:
```json
{
  "mcpServers": {
    "server-name": {
      "command": "npx",
      "args": ["-y", "@some/mcp-server"],
      "env": { "API_KEY": "..." }
    }
  }
}
```

### Implementation

- JSON-RPC 2.0 client over stdio or SSE
- Tool discovery: call `tools/list` on connected servers
- Tool execution: call `tools/call` with arguments
- Resource management: `resources/list`, `resources/read`, subscriptions
- Lifecycle: initialize, negotiate capabilities, shutdown

## Hooks System

Hooks run shell commands or Claude-driven prompts on lifecycle events:

| Event | When |
|-------|------|
| SessionStart | Session begins |
| UserPromptSubmit | User sends a message |
| PreToolUse | Before a tool executes |
| PostToolUse | After a tool executes |
| PermissionRequest | When permission is needed |
| Stop | Conversation ends |

Hooks can be:
- **Command hooks**: run a shell command, use exit code / stdout
- **Prompt hooks**: inject additional context into the conversation
- **Agent hooks**: spawn a Claude-driven sub-process

## Skills and Plugins

### Skills

Markdown files with frontmatter defining:
- Trigger conditions (slash command name, auto-trigger patterns)
- Instructions/prompts
- Supporting files

Located in:
- `~/.claude/skills/` (user-level)
- `.claude/skills/` (project-level)

### Plugins

Bundles of skills, hooks, subagents, and MCP servers. Installable from GitHub or local paths.

## Directory Structure

Proposed Go project layout:

```
ClaudeCodeGo/
├── cmd/
│   └── claude/
│       └── main.go              # Entry point, CLI flag parsing
├── internal/
│   ├── api/
│   │   ├── client.go            # HTTP client, request building
│   │   ├── messages.go          # Messages API types and methods
│   │   ├── streaming.go         # SSE parser and event handling
│   │   └── types.go             # API request/response types
│   ├── auth/
│   │   ├── oauth.go             # OAuth flow (browser, callback server)
│   │   ├── token.go             # Token storage, refresh
│   │   └── credentials.go       # Credential file management
│   ├── config/
│   │   ├── settings.go          # Settings hierarchy, merging
│   │   ├── claudemd.go          # CLAUDE.md loading and import resolution
│   │   └── permissions.go       # Permission rules parsing and matching
│   ├── conversation/
│   │   ├── history.go           # Message history management
│   │   ├── compaction.go        # Context compaction / summarization
│   │   └── system_prompt.go     # System prompt assembly
│   ├── hooks/
│   │   ├── hooks.go             # Hook registry and dispatch
│   │   └── events.go            # Event types
│   ├── mcp/
│   │   ├── client.go            # MCP JSON-RPC client
│   │   ├── stdio.go             # stdio transport
│   │   ├── sse.go               # SSE transport
│   │   └── types.go             # MCP protocol types
│   ├── session/
│   │   ├── session.go           # Session lifecycle
│   │   ├── persistence.go       # Save/load sessions
│   │   └── checkpoint.go        # Git checkpoint management
│   ├── tools/
│   │   ├── registry.go          # Tool registration and dispatch
│   │   ├── bash.go              # Bash tool
│   │   ├── fileread.go          # FileRead tool
│   │   ├── fileedit.go          # FileEdit tool
│   │   ├── filewrite.go         # FileWrite tool
│   │   ├── glob.go              # Glob tool
│   │   ├── grep.go              # Grep tool
│   │   ├── agent.go             # Agent/Task tool
│   │   ├── todo.go              # TodoWrite tool
│   │   ├── webfetch.go          # WebFetch tool
│   │   ├── websearch.go         # WebSearch tool
│   │   ├── askuser.go           # AskUserQuestion tool
│   │   ├── notebook.go          # NotebookEdit tool
│   │   ├── config_tool.go       # Config tool
│   │   ├── worktree.go          # EnterWorktree tool
│   │   ├── planmode.go          # ExitPlanMode tool
│   │   ├── taskoutput.go        # TaskOutput tool
│   │   └── taskstop.go          # TaskStop tool
│   └── tui/
│       ├── tui.go               # Main TUI loop
│       ├── input.go             # User input handling
│       ├── output.go            # Output rendering (markdown, diffs)
│       ├── progress.go          # Progress indicators, spinners
│       └── slash.go             # Slash command handling
├── go.mod
├── go.sum
├── CLAUDE.md                    # This file
├── LICENSE
├── .gitignore
└── claude-code-source/          # Reference JS source (not compiled)
```

## Implementation Phases

### Phase 1: Foundation

Get a basic conversation working end-to-end.

- [ ] Go module init (`go mod init`)
- [ ] Reverse-engineer auth flow from cli.js and network inspection
- [ ] Implement OAuth login (browser open, local callback server, token exchange)
- [ ] Token storage and refresh
- [ ] HTTP client for Messages API (correct endpoint, headers, auth)
- [ ] SSE streaming parser
- [ ] Basic stdin/stdout conversation loop (no tools yet)
- [ ] Verify: can send a message and stream a response

### Phase 2: Core Tool System

Make the agentic loop functional with essential tools.

- [ ] Tool interface and registry
- [ ] Tool schema generation (matching sdk-tools.d.ts)
- [ ] Tool dispatch in the agentic loop
- [ ] Tool result formatting and conversation threading
- [ ] Bash tool (command execution, timeout, output capture)
- [ ] FileRead tool (text files with offset/limit, line numbers)
- [ ] FileEdit tool (string replacement, replace_all)
- [ ] FileWrite tool (create/overwrite)
- [ ] Glob tool (file pattern matching)
- [ ] Grep tool (ripgrep-compatible regex search)
- [ ] Basic permission prompts (ask user before executing tools)
- [ ] Verify: Claude can read files, run commands, and edit code

### Phase 3: Session and Config

Make it persistent and configurable.

- [ ] Session save/load (match official format)
- [ ] `-c` (continue last) and `-r` (resume by ID) flags
- [ ] Settings hierarchy (managed → CLI → local → project → user)
- [ ] Settings file parsing (JSON)
- [ ] CLAUDE.md loading (multi-location, `@path` imports, `.claude/rules/`)
- [ ] System prompt assembly (injecting CLAUDE.md, context, permissions)
- [ ] Permission rules (pattern matching, allow/deny/ask)
- [ ] Context compaction (summarize when nearing limit)
- [ ] Verify: sessions persist, config files are respected

### Phase 4: Remaining Tools

Complete the tool set.

- [ ] Agent/Task tool (spawn sub-agents with isolated context)
- [ ] TodoWrite tool
- [ ] AskUserQuestion tool (structured questions with options)
- [ ] WebFetch tool (URL fetch + prompt processing)
- [ ] WebSearch tool
- [ ] NotebookEdit tool (Jupyter .ipynb manipulation)
- [ ] Config tool (get/set settings programmatically)
- [ ] EnterWorktree tool (git worktree creation)
- [ ] ExitPlanMode tool
- [ ] TaskOutput / TaskStop tools (background task management)
- [ ] FileRead extensions (images, PDFs, notebooks)
- [ ] Git checkpoint system (snapshot before edits)
- [ ] Verify: all tools from sdk-tools.d.ts are implemented

### Phase 5: TUI

Rich terminal experience.

- [ ] Choose and integrate TUI library (Bubble Tea vs tcell vs other)
- [ ] Markdown rendering in terminal
- [ ] Syntax-highlighted code blocks
- [ ] Diff display for file edits
- [ ] Streaming text display (token by token)
- [ ] Tool execution progress indicators
- [ ] Permission prompt UI
- [ ] Slash command input and dispatch (`/help`, `/model`, `/cost`, `/compact`, etc.)
- [ ] Cost/token tracking display
- [ ] Multi-line input editing
- [ ] Verify: matches the visual experience of the official CLI

### Phase 6: MCP

External tool server support.

- [ ] JSON-RPC 2.0 client implementation
- [ ] stdio transport (subprocess management)
- [ ] SSE transport (HTTP streaming)
- [ ] MCP server configuration loading (`.mcp.json`, `~/.mcp.json`)
- [ ] Tool discovery and registration from MCP servers
- [ ] Tool execution via MCP
- [ ] Resource listing, reading, subscription
- [ ] Server lifecycle management (init, capabilities, shutdown)
- [ ] Verify: can connect to standard MCP servers (e.g., GitHub MCP)

### Phase 7: Hooks, Skills, and Advanced Features

Full feature parity.

- [ ] Hooks system (command hooks, prompt hooks, agent hooks)
- [ ] All hook events (SessionStart, PreToolUse, PostToolUse, etc.)
- [ ] Skills loading and slash command registration
- [ ] Plugin system (install, load bundles)
- [ ] Agent teams (multi-session coordination)
- [ ] Worktree support (parallel isolated sessions)
- [ ] `claude update` self-update mechanism
- [ ] `claude mcp` management commands
- [ ] `claude agents` listing
- [ ] Print mode (`-p`) for scripting
- [ ] JSON and stream-JSON output formats
- [ ] Pipe/stdin support for Unix workflows
- [ ] Verify: full behavioral parity with official CLI

## CLI Interface

### Command Line Flags

Match the official CLI's interface:

```
claude                          # Interactive REPL
claude "prompt"                 # Start with initial prompt
claude -p "prompt"              # Print mode (non-interactive, exit after response)
claude -c                       # Continue most recent session
claude -r "session-id"          # Resume specific session
claude --model <model>          # Override model
claude --output-format <fmt>    # text | json | stream-json
claude update                   # Self-update
claude mcp [subcommand]         # MCP server management
claude agents                   # List configured agents
```

### Slash Commands (Interactive Mode)

```
/help                           # Show help
/model                          # Switch model
/cost                           # Show token usage and cost
/context                        # Show context usage breakdown
/compact                        # Trigger context compaction
/memory                         # Edit persistent memories
/hooks                          # View configured hooks
/agents                         # Configure subagents
/mcp                            # Manage MCP servers
/init                           # Initialize CLAUDE.md for project
/doctor                         # Diagnose issues
/fast                           # Toggle fast mode
```

## Go Conventions

### Error Handling

- Return errors, don't panic (except truly unrecoverable situations)
- Use `fmt.Errorf("context: %w", err)` for error wrapping
- Define sentinel errors for expected failure modes (e.g., `ErrTokenExpired`, `ErrPermissionDenied`)

### Concurrency

- Use goroutines for: SSE streaming, background tool execution, MCP server communication
- Use `context.Context` for cancellation (Ctrl+C propagation)
- Use channels for streaming events from API to TUI
- Protect shared state with mutexes or use channel-based designs

### Testing

- Unit tests for each package (`_test.go` files alongside source)
- Table-driven tests for tool input/output validation
- Integration tests that verify against recorded API responses
- Test tool implementations against the schemas in `sdk-tools.d.ts`
- Use `testdata/` directories for fixture files

### Dependencies

Keep dependencies minimal. Expected core dependencies:
- Standard library for HTTP, JSON, crypto, os, exec
- A TUI library (TBD)
- A markdown terminal renderer (e.g., `charmbracelet/glamour`)
- A glob library if `filepath.Glob` is insufficient (e.g., `doublestar`)
- No Anthropic SDK -- we use direct HTTP

### Build

- `go build ./cmd/claude` produces a single static binary
- Cross-compile for darwin/arm64, darwin/amd64, linux/amd64, linux/arm64, windows/amd64
- Binary name: `claude` (matching the official CLI)

## Compatibility Notes

### Interoperability with Official CLI

Where possible, use the same file formats and locations as the official CLI:
- `~/.claude/` for all user data
- Same session format so sessions can be resumed across implementations
- Same settings file format and locations
- Same CLAUDE.md loading behavior
- Same permission rule syntax

### Behavioral Parity

The Go implementation should produce identical API requests and handle responses identically. Key areas:
- System prompt must match (same CLAUDE.md injection, same tool descriptions)
- Tool schemas must match `sdk-tools.d.ts` exactly
- Tool output format must match so the model gets the same context
- Conversation threading (user/assistant/tool_result message ordering) must match

### What We Intentionally Skip (For Now)

- API key authentication (can be added later)
- AWS Bedrock / Google Vertex / Microsoft Foundry backends
- IDE integration protocols (VS Code, JetBrains extensions)
- Telemetry / analytics reporting
- The `/bug` command's feedback submission
