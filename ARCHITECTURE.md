# Architecture

ClaudeCodeGo is a Go reimplementation of the Claude Code CLI (`@anthropic-ai/claude-code`). This document describes how every component fits together, how data flows through the system, and where the Go implementation diverges from the JavaScript original.

## High-level overview

```
                          ┌──────────────────────┐
                          │   cmd/claude/main.go  │
                          │   (CLI entry point)   │
                          └──────────┬───────────┘
                                     │
                 ┌───────────────────┤───────────────────┐
                 │                   │                   │
          ┌──────▼──────┐    ┌───────▼───────┐   ┌──────▼──────┐
          │  Print mode │    │   TUI (app)   │   │  Pipe mode  │
          │  -p flag    │    │  Bubble Tea   │   │  stdin→-p   │
          └──────┬──────┘    └───────┬───────┘   └──────┬──────┘
                 │                   │                   │
                 └───────────────────┤───────────────────┘
                                     │
                          ┌──────────▼───────────┐
                          │   Agentic Loop       │
                          │  (conversation/loop)  │
                          ├──────────────────────┤
                          │  Hooks ──────────────│──→ internal/hooks
                          │  Tools ──────────────│──→ internal/tools
                          │  Compaction ─────────│──→ conversation/compaction
                          │  History ────────────│──→ conversation/history
                          └──────────┬───────────┘
                                     │
                          ┌──────────▼───────────┐
                          │     API Client       │
                          │    (api/client)       │
                          │    SSE streaming      │
                          └──────────┬───────────┘
                                     │
                          ┌──────────▼───────────┐
                          │    OAuth / Tokens     │
                          │   (auth/oauth,        │
                          │    auth/credentials)  │
                          └──────────────────────┘
```

The binary has three modes of operation:

1. **Interactive mode** (default) — launches a Bubble Tea TUI with rich markdown rendering, permission prompts, and slash commands.
2. **Print mode** (`-p`) — non-interactive. Sends a single prompt, streams the response to stdout, exits.
3. **Pipe mode** (stdin is not a terminal) — reads prompt from stdin, forces print mode.

All three modes share the same agentic loop, tool registry, and API client.

---

## Package map

```
cmd/claude/main.go              Entry point, flag parsing, component wiring
internal/
  api/
    client.go                   HTTP client, streaming request/response
    types.go                    Messages API types (requests, responses, content blocks)
    streaming.go                SSE line parser, StreamHandler interface
  auth/
    oauth.go                    PKCE OAuth flow (browser, callback server, code exchange)
    credentials.go              Token storage (~/.claude/.credentials.json), auto-refresh
  config/
    settings.go                 Five-level settings hierarchy, merge logic
    permissions.go              Rule-based permission matching (glob patterns)
    claudemd.go                 CLAUDE.md loader (multi-location, @path imports, rules dirs)
  conversation/
    loop.go                     Agentic loop, HookRunner interface, stream handlers
    history.go                  Message list management
    compaction.go               Context window summarization
    system_prompt.go            System prompt assembly (identity, env, CLAUDE.md, skills, perms)
    json_handlers.go            JSON and stream-JSON output handlers for --output-format
  hooks/
    types.go                    HookConfig, HookDef, event constants
    runner.go                   Hook execution engine (shell commands, prompts)
  skills/
    types.go                    Skill struct
    loader.go                   Skill discovery and frontmatter parsing
  session/
    session.go                  Session persistence (~/.claude/projects/<hash>/sessions/)
  tools/
    registry.go                 Tool interface, registry, permission-checked dispatch
    permission.go               TerminalPermissionHandler, AlwaysAllowPermissionHandler
    background.go               BackgroundTaskStore (shared by Agent, TaskOutput, TaskStop)
    agent.go                    Agent/Task tool (sub-agents with isolated loops)
    bash.go                     Shell command execution
    fileread.go                 File reading (text, images, PDFs, notebooks)
    fileedit.go                 String replacement editing
    filewrite.go                File creation/overwrite
    glob.go                     Pattern-based file discovery
    grep.go                     Content search (ripgrep or grep fallback)
    todo.go                     Structured task list
    askuser.go                  Structured questions with options
    webfetch.go                 URL fetching with HTML-to-text and caching
    websearch.go                Web search (stub — handled server-side)
    notebook.go                 Jupyter notebook cell editing
    config_tool.go              Runtime config get/set
    worktree.go                 Git worktree creation
    planmode.go                 ExitPlanMode signal
    taskoutput.go               Read background task output
    taskstop.go                 Stop background tasks
  mcp/
    manager.go                  MCP server lifecycle management
    client.go                   JSON-RPC 2.0 client for MCP protocol
    stdio.go                    Subprocess transport (stdin/stdout)
    sse.go                      HTTP SSE transport
    config.go                   .mcp.json loading and merging
    types.go                    MCP protocol types
    tools.go                    MCPToolWrapper, resource tools, subscription tools
  tui/
    app.go                      Top-level TUI application, wiring
    model.go                    Bubble Tea model (state machine, Update/View)
    msg.go                      All BT message types
    stream.go                   TUIStreamHandler (loop events → BT messages)
    permission.go               TUIPermissionHandler (modal prompts with channel handshake)
    slash.go                    Slash command registry, skill command registration
    input.go                    Text input configuration
    output.go                   Markdown rendering, diff display, tool summaries
    status.go                   Token tracking, status bar
    progress.go                 Spinner configuration
    theme.go                    Lipgloss style definitions
    todo.go                     Todo list rendering
```

---

## The agentic loop

The loop lives in `conversation/loop.go` and is the core of the application. It orchestrates the conversation between the user, the Claude API, and tools.

### Flow

```
 SendMessage(ctx, text)
        │
        │  ← UserPromptSubmit hook (can modify or reject message)
        │
        ▼
  Add user message to history
        │
        ▼
  ┌─────────────────────────────────────────┐
  │  Build request (messages, system, tools) │
  │  Call API with streaming                 │──→ StreamHandler callbacks
  │  Assemble response from SSE events       │
  │  Add assistant response to history       │
  │  Check auto-compaction                   │
  │                                          │
  │  if stop_reason == "end_turn"            │
  │    → Stop hook                           │
  │    → notify turn complete                │
  │    → return (done)                       │
  │                                          │
  │  if stop_reason == "tool_use"            │
  │    for each tool_use block:              │
  │      → PreToolUse hook (can block)       │
  │      → registry.Execute (+ perms)        │
  │      → PostToolUse hook                  │
  │      → collect result                    │
  │    add tool results to history           │
  │    notify turn complete                  │
  │    → loop back                           │
  └─────────────────────────────────────────┘
```

### Key types

- **`LoopConfig`** — everything the loop needs: client, system prompt, tool definitions, tool executor, stream handler, history, compactor, hooks, turn-complete callback.
- **`ToolExecutor`** interface — `Execute(ctx, name, input) → (string, error)` and `HasTool(name) → bool`. Implemented by `tools.Registry`.
- **`HookRunner`** interface — six methods matching lifecycle events. Implemented by `hooks.Runner`. Nil means no hooks.
- **`StreamHandler`** interface — eight callbacks for SSE events. Five implementations exist (see below).

### Stream handlers

| Handler | Location | Used when |
|---------|----------|-----------|
| `PrintStreamHandler` | `conversation/loop.go` | Print mode with `text` output format |
| `ToolAwareStreamHandler` | `conversation/loop.go` | Fallback; shows tool summaries on stdout |
| `TUIStreamHandler` | `tui/stream.go` | Interactive mode; forwards events to Bubble Tea |
| `JSONStreamHandler` | `conversation/json_handlers.go` | `--output-format json`; buffers, emits single JSON |
| `StreamJSONStreamHandler` | `conversation/json_handlers.go` | `--output-format stream-json`; one JSON line per event |

---

## Authentication

The CLI authenticates via Claude subscription OAuth (Pro/Team/Enterprise), not API keys.

### OAuth flow (`auth/oauth.go`)

1. Generate PKCE code verifier + challenge.
2. Start an ephemeral local HTTP server on a random port.
3. Open the browser to `https://claude.ai/oauth/authorize` with client ID, redirect URI, PKCE challenge, and scopes.
4. User logs in and consents in the browser.
5. Browser redirects to `http://localhost:<port>/oauth/callback?code=...`.
6. CLI exchanges the authorization code for access + refresh tokens at `https://platform.claude.com/v1/oauth/token`.
7. Tokens stored to `~/.claude/.credentials.json`.

### Token management (`auth/credentials.go`)

- `TokenProvider` implements the `TokenSource` interface used by the API client.
- Checks `CLAUDE_CODE_OAUTH_TOKEN` environment variable first.
- Auto-refreshes when less than 5 minutes until expiration.
- Caches tokens in memory after first load.
- File permissions: directory 0700, file 0600.

---

## API client

`api/client.go` sends requests to the Claude Messages API with streaming.

### Request flow

1. Build `CreateMessageRequest` with messages, system prompt, tools, and `stream: true`.
2. POST to the API endpoint with `Authorization: Bearer <token>` and `anthropic-beta: oauth-2025-01-01`.
3. Read the SSE response line by line via `ParseSSEStream`.
4. A `responseAssembler` collects events and builds the final `MessageResponse` — assembling text deltas into text blocks and `input_json_delta` fragments into complete tool call JSON.
5. Simultaneously, the `StreamHandler` receives every event for live display.

### SSE event types

| Event | Purpose |
|-------|---------|
| `message_start` | Initial message metadata and input token usage |
| `content_block_start` | Start of a text or tool_use block |
| `content_block_delta` | Incremental text or JSON fragment |
| `content_block_stop` | Block complete |
| `message_delta` | Stop reason and output token usage |
| `message_stop` | Stream finished |

---

## Configuration

### Settings hierarchy (`config/settings.go`)

Loaded from lowest to highest priority (higher overrides lower):

1. **User** — `~/.claude/settings.json`
2. **Project** — `.claude/settings.json`
3. **Local** — `.claude/settings.local.json` (gitignored)
4. **Managed** — `/etc/claude/settings.json`
5. **CLI flags** — applied after loading (in `main.go`)

Merge rules:
- Scalar fields: higher priority wins.
- `permissions`: concatenated, higher-priority rules first (first match wins).
- `env`: deep merge, higher priority wins per key.
- `hooks`, `sandbox`: higher priority wins if non-nil.

### CLAUDE.md loading (`config/claudemd.go`)

Content loaded and concatenated from:

1. `~/.claude/CLAUDE.md` (user-level)
2. Every `CLAUDE.md` found walking from filesystem root to CWD
3. `.claude/CLAUDE.md` (project-level)
4. `~/.claude/rules/*.md` (user rules, sorted)
5. `.claude/rules/*.md` (project rules, sorted)

Supports `@path` import directives (with cycle detection).

### Permission rules (`config/permissions.go`)

Rules are glob patterns that match tool calls:

```
Bash                       → all bash commands
Bash(npm run *)            → specific command patterns
FileRead(./.env)           → specific file paths
WebFetch(domain:example.com) → domain restrictions
```

Evaluated by `RuleBasedPermissionHandler` in order; first match determines action (`allow`, `deny`, or `ask`). Falls back to the underlying handler (terminal prompt or TUI modal) if no rule matches.

---

## System prompt assembly

`conversation/system_prompt.go` builds the system prompt from:

1. **Core identity** — "You are Claude Code..."
2. **Environment info** — CWD, platform, date
3. **CLAUDE.md content** — loaded via `config.LoadClaudeMD`
4. **Active skills** — injected if skills are loaded
5. **Permission rules summary** — if rules are configured

---

## Tool system

### Interface (`tools/registry.go`)

```go
type Tool interface {
    Name() string
    Description() string
    InputSchema() json.RawMessage
    Execute(ctx context.Context, input json.RawMessage) (string, error)
    RequiresPermission(input json.RawMessage) bool
}
```

The `Registry` holds all registered tools and dispatches execution. Before executing a tool that requires permission, it calls the current `PermissionHandler`.

### Permission flow

Two built-in handlers:
- **`TerminalPermissionHandler`** — prompts stdin for y/n (print mode fallback)
- **`AlwaysAllowPermissionHandler`** — auto-approves (`--dangerously-skip-permissions`)

In TUI mode, `TUIPermissionHandler` sends a `PermissionRequestMsg` to the Bubble Tea event loop and blocks on a channel until the user presses y/n.

### Built-in tools

| Tool | Permission | Notes |
|------|-----------|-------|
| Bash | Yes | Timeout support (default 120s, max 600s), output truncation (100K chars) |
| FileRead | No | Text files with cat -n format, images (base64), PDFs (via pdftotext), notebooks |
| FileEdit | Yes | Exact string replacement, uniqueness check, replace_all mode |
| FileWrite | Yes | Creates parent dirs, absolute paths only |
| Glob | No | doublestar patterns, sorted by mtime |
| Grep | No | Wraps ripgrep (falls back to grep), three output modes |
| Agent | No | Spawns sub-agents with isolated conversation loops |
| TodoWrite | No | Updates structured task list, integrates with TUI |
| AskUserQuestion | No | Multi-choice questions with "Other" option |
| WebFetch | Yes | HTTP fetch, HTML-to-text, 15-min cache, 10MB limit |
| WebSearch | No | Stub (server-side capability) |
| NotebookEdit | Yes | Jupyter cell replace/insert/delete |
| Config | No | Get/set runtime settings |
| EnterWorktree | Yes | Git worktree creation |
| ExitPlanMode | No | Signals plan completion |
| TaskOutput | No | Read background agent output |
| TaskStop | No | Cancel background agents |

### Sub-agents (`tools/agent.go`)

The Agent tool creates isolated conversation loops with their own history but sharing the same API client, tool registry, and permission handler. Sub-agents inherit hooks from the parent. They can run synchronously (blocking) or in the background (tracked by `BackgroundTaskStore`).

---

## Hooks system

### Configuration

Defined in `settings.json` under the `hooks` key:

```json
{
  "hooks": {
    "PreToolUse":        [{"type": "command", "command": "./pre-tool.sh"}],
    "PostToolUse":       [{"type": "command", "command": "echo done"}],
    "UserPromptSubmit":  [{"type": "prompt",  "prompt": "Check for sensitive data"}],
    "SessionStart":      [{"type": "command", "command": "./setup.sh"}],
    "Stop":              [{"type": "command", "command": "./cleanup.sh"}],
    "PermissionRequest": [{"type": "command", "command": "./log-perm.sh"}]
  }
}
```

### Execution (`hooks/runner.go`)

Command hooks run via `sh -c` with environment variables set:

| Variable | Events | Content |
|----------|--------|---------|
| `HOOK_EVENT` | All | Event name |
| `TOOL_NAME` | PreToolUse, PostToolUse, PermissionRequest | Tool being called |
| `TOOL_INPUT` | PreToolUse, PostToolUse, PermissionRequest | Tool input JSON |
| `TOOL_OUTPUT` | PostToolUse | Tool result (truncated to 10K) |
| `TOOL_IS_ERROR` | PostToolUse | "true" or "false" |
| `USER_MESSAGE` | UserPromptSubmit | User's message text |

Exit code semantics:
- **0** — continue normally
- **non-zero** — block the action (PreToolUse blocks tool execution; UserPromptSubmit rejects the message)

For UserPromptSubmit, stdout from the hook replaces the user's message (message modification).

### Firing points

| Event | Where | Semantics |
|-------|-------|-----------|
| `SessionStart` | `main.go` (print mode) or `tui/app.go` (TUI mode) | Before first interaction |
| `UserPromptSubmit` | `Loop.SendMessage()` before adding to history | Can modify or reject |
| `PreToolUse` | `Loop.run()` before `toolExec.Execute()` | Can block tool execution |
| `PostToolUse` | `Loop.run()` after `toolExec.Execute()` | Observational; errors logged |
| `Stop` | `Loop.run()` when `stop_reason != "tool_use"` | Fires on conversation end |
| `PermissionRequest` | Available via `RunPermissionRequest()` | Currently informational |

---

## Skills system

### Discovery (`skills/loader.go`)

Skills are markdown files loaded from:
1. `.claude/skills/` (project-level, higher priority)
2. `~/.claude/skills/` (user-level)

Project skills with the same name override user skills.

### Format

```markdown
---
name: commit
description: Create a git commit
trigger: /commit
---

Instructions for the model when this skill is activated...
```

Frontmatter is optional. Without it, the filename (minus `.md`) becomes the name.

### Integration

- **System prompt** — all skill content is injected under `# Active Skills`.
- **Slash commands** — skills with a `trigger` field register as slash commands. When invoked, the skill's body is sent as a user message to the agentic loop.

---

## MCP (Model Context Protocol)

### Architecture

```
                    ┌─────────────┐
                    │   Manager   │
                    └──────┬──────┘
                           │ manages
              ┌────────────┼────────────┐
              │            │            │
        ┌─────▼─────┐ ┌───▼─────┐ ┌───▼─────┐
        │ MCPClient │ │MCPClient│ │MCPClient│
        └─────┬─────┘ └───┬─────┘ └───┬─────┘
              │            │            │
        ┌─────▼─────┐ ┌───▼─────┐ ┌───▼─────┐
        │   Stdio   │ │   SSE   │ │  Stdio  │
        │ Transport │ │Transport│ │Transport│
        └───────────┘ └─────────┘ └─────────┘
```

The `Manager` starts MCP servers from `.mcp.json` config, discovers their tools via `tools/list`, wraps them as `MCPToolWrapper` objects, and registers them in the tool registry. MCP tool names are prefixed: `mcp__<server>__<tool>`.

### Transports

- **Stdio** — launches a subprocess, communicates via stdin/stdout JSON-RPC. Line-based protocol with 10MB scanner buffer.
- **SSE** — connects to an HTTP endpoint, reads SSE events for endpoint discovery, then POSTs JSON-RPC messages.

### Config

Loaded from `~/.mcp.json` (user) and `.mcp.json` (project), merged with project overriding per server name.

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

---

## TUI

### Framework

Built on [Bubble Tea](https://github.com/charmbracelet/bubbletea), the Elm-architecture TUI framework for Go. Markdown rendered with [Glamour](https://github.com/charmbracelet/glamour). Styling with [Lipgloss](https://github.com/charmbracelet/lipgloss).

### State machine

The TUI model has four modes:

| Mode | Active when |
|------|-------------|
| `modeInput` | Waiting for user text input |
| `modeStreaming` | API response in progress (spinner, streaming text) |
| `modePermission` | Waiting for y/n on a permission prompt |
| `modeAskUser` | Waiting for structured question response |

### Event flow

The agentic loop runs in a goroutine. Events flow to the TUI via three bridges:

1. **`TUIStreamHandler`** — converts SSE events into BT messages (`TextDeltaMsg`, `ContentBlockStopMsg`, etc.) via `program.Send()`.
2. **`TUIPermissionHandler`** — sends `PermissionRequestMsg` with a result channel. The goroutine blocks until the TUI sends y/n back.
3. **AskUser** — same pattern: `AskUserRequestMsg` with a response channel.

### Slash commands

Built-in: `/help`, `/model`, `/version`, `/cost`, `/context`, `/mcp`, `/compact`, `/quit`, `/exit`.

Skills with `trigger` frontmatter register additional slash commands at startup.

### View layout (live region)

```
┌─ Streaming text (markdown) ──────────────────────┐
│                                                   │
├─ Active tool spinner ─────────────────────────────┤
│  ⣾ Bash  $ npm test                              │
├─ Permission prompt ───────────────────────────────┤
│  Allow Bash: $ rm -rf tmp? [y/n]                  │
├─ AskUser prompt ──────────────────────────────────┤
│  [Auth] Which method? > OAuth  JWT  Other         │
├─ Todo list ───────────────────────────────────────┤
│  [x] Parse config                                 │
│  [~] Implementing auth flow                       │
│  [ ] Write tests                                  │
├─ Text input ──────────────────────────────────────┤
│  > _                                              │
├─ Status bar ──────────────────────────────────────┤
│  claude-sonnet-4-20250514  ↓1.2k ↑0.3k           │
└───────────────────────────────────────────────────┘
```

---

## Session management

Sessions are stored in `~/.claude/projects/<sha256(cwd)>/sessions/<id>.json`. Each session records:

- Session ID (timestamp-based)
- Model name
- Working directory
- Full message history
- Creation and update timestamps

Flags:
- `-c` resumes the most recent session (by `UpdatedAt`).
- `-r <id>` resumes a specific session.

Auto-save happens after every agentic turn via the `OnTurnComplete` callback.

---

## Context compaction

When input tokens exceed 150,000 (configurable), the `Compactor` summarizes older messages:

1. Preserve the 4 most recent messages.
2. Send older messages to the API with a summarization prompt.
3. Replace them with a single summary message prefixed `[Conversation Summary]`.

Manual compaction available via `/compact`.

---

## Differences from the JavaScript original

This section flags intentional simplifications, partial implementations, and behavioral differences compared to the official `@anthropic-ai/claude-code` v2.1.50.

### Authentication

| Aspect | JS original | Go implementation |
|--------|------------|-------------------|
| API key auth | Supported | **Not implemented** — subscription OAuth only |
| Bedrock/Vertex/Foundry | Supported | **Not implemented** |
| Token storage format | `claudeAiOauth` key in `~/.claude/.credentials.json` | Same format — interoperable |

### Hooks

| Aspect | JS original | Go implementation |
|--------|------------|-------------------|
| Hook types | command, prompt, agent | command and prompt implemented; **agent hooks treated as commands** |
| Hook config merging | Deep merge across settings levels | **Overlay wins** — higher-priority settings replace entire hooks config |
| PermissionRequest hook | Fires in the permission handler | **Not wired into the permission handler** — `RunPermissionRequest` exists but isn't called from `RuleBasedPermissionHandler` |

### Skills / Plugins

| Aspect | JS original | Go implementation |
|--------|------------|-------------------|
| Plugin bundles | Installable from GitHub, bundles of skills+hooks+MCP | **Not implemented** — only standalone skill files |
| Auto-trigger skills | Pattern-based automatic activation | **Not implemented** — trigger-based slash commands only |
| Skill file format | Full YAML frontmatter with many fields | **Simplified** — only `name`, `description`, `trigger` parsed |

### Tools

| Aspect | JS original | Go implementation |
|--------|------------|-------------------|
| WebSearch | Full server-side integration | **Stub** — returns placeholder response |
| Git checkpoints | Automatic snapshots before edits | **Not implemented** |
| FileRead PDF | Built-in PDF parsing | **Requires `pdftotext`** (poppler-utils) installed externally |
| Parallel tool calls | Concurrent execution | **Sequential** — tools in a single response executed one at a time |

### MCP

| Aspect | JS original | Go implementation |
|--------|------------|-------------------|
| Server-sent notifications | Handled asynchronously | **Not implemented** — SSE transport reads endpoint event only |
| Capability negotiation | Full capabilities exchange | **Simplified** — sends client capabilities, stores server capabilities |
| Error recovery | Reconnect on transport failure | **No reconnection** — server failure is permanent for the session |

### TUI

| Aspect | JS original | Go implementation |
|--------|------------|-------------------|
| Multi-line input | Shift+Enter for newlines | **Enter submits immediately** — no multi-line editing |
| Syntax highlighting | Token-level highlighting in code blocks | **Glamour-rendered** — block-level highlighting only |
| `/init` command | Interactive CLAUDE.md creation | **Not implemented** |
| `/doctor` command | Diagnostic checks | **Not implemented** |
| `/fast` command | Toggle fast mode | **Not implemented** |
| `/memory` command | Edit persistent memories | **Not implemented** |
| `/hooks` command | View configured hooks | **Not implemented** |
| `/agents` command | Configure sub-agents | **Not implemented** |
| Image display | Inline image rendering | **Not displayed** — base64 encoded and returned as JSON to the API |

### CLI

| Aspect | JS original | Go implementation |
|--------|------------|-------------------|
| `claude update` | Self-update mechanism | **Not implemented** |
| `claude mcp` | MCP management subcommands | **Not implemented** |
| `claude agents` | List configured agents | **Not implemented** |
| Telemetry | Usage analytics | **Not implemented** (intentionally) |
| `/bug` command | Feedback submission | **Not implemented** (intentionally) |
| IDE integration | VS Code, JetBrains protocols | **Not implemented** |

### Output formats

| Aspect | JS original | Go implementation |
|--------|------------|-------------------|
| `--output-format json` | Full message object | Implemented — single JSON object on message complete |
| `--output-format stream-json` | One JSON line per event | Implemented — matches event types |
| `--output-format text` | Default text output | Implemented |

### Session interoperability

The Go implementation uses the same `~/.claude/` directory structure and JSON formats. Sessions saved by the Go binary should be loadable by the JS original and vice versa, though this has not been exhaustively tested.

---

## Concurrency model

- **Main goroutine** — runs the Bubble Tea event loop (TUI mode) or waits for completion (print mode).
- **Agentic loop goroutine** — runs `loop.SendMessage()` in a goroutine started by BT commands.
- **Stream handler** — called from the agentic loop goroutine, sends BT messages via `program.Send()` (thread-safe).
- **Permission handler** — sends a message to BT and blocks on a channel; the BT Update loop writes the response.
- **Background agents** — each runs in its own goroutine with an isolated context.
- **MCP servers** — each stdio transport runs a subprocess with a goroutine monitoring its exit.

Context cancellation (Ctrl+C) propagates from the root context to all goroutines.

---

## Dependencies

Minimal external dependencies (no Anthropic SDK — direct HTTP):

| Dependency | Purpose |
|-----------|---------|
| `charmbracelet/bubbletea` | TUI framework |
| `charmbracelet/bubbles` | TUI components (textarea, spinner) |
| `charmbracelet/glamour` | Terminal markdown rendering |
| `charmbracelet/lipgloss` | Terminal styling |
| `bmatcuk/doublestar` | Glob pattern matching with `**` support |
| `golang.org/x/term` | Terminal detection and size |

Everything else uses the Go standard library.

---

## Build

```
go build ./cmd/claude
```

Produces a single static binary named `claude`. Cross-compilation:

```
GOOS=darwin  GOARCH=arm64  go build ./cmd/claude
GOOS=darwin  GOARCH=amd64  go build ./cmd/claude
GOOS=linux   GOARCH=amd64  go build ./cmd/claude
GOOS=linux   GOARCH=arm64  go build ./cmd/claude
GOOS=windows GOARCH=amd64  go build ./cmd/claude
```
