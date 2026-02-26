# Go Implementation Analysis

Deep analysis of the ClaudeCodeGo codebase. Catalogs every implemented feature
with implementation completeness levels per package.

**Codebase**: 32,735 lines of Go across 12 packages + entry point
**Test coverage**: Comprehensive (every package has tests, ~50% of lines are test code)

---

## Package Overview

| Package | Files | Purpose | Completeness |
|---------|-------|---------|-------------|
| `cmd/claude` | 1 | CLI entry point, flag parsing, initialization | 95% |
| `internal/api` | 3+3t | HTTP client, SSE streaming, API types | 85% |
| `internal/auth` | 6+4t | OAuth PKCE flow, token management, credentials | 98% |
| `internal/config` | 3+3t | Settings hierarchy, CLAUDE.md, permissions | 95% |
| `internal/conversation` | 6+4t | Agentic loop, history, compaction, system prompt | 90% |
| `internal/hooks` | 2+1t | Hook event system, command runner | 95% |
| `internal/mcp` | 7+4t | JSON-RPC client, transports, tool discovery | 85% |
| `internal/mock` | 5+2t | Test infrastructure, mock backend | 90% |
| `internal/session` | 1+1t | Session persistence, save/load/resume | 100% |
| `internal/skills` | 2+1t | Skill loading, YAML frontmatter parsing | 100% |
| `internal/tools` | 18+6t | Tool registry, 17 tool implementations | 95% |
| `internal/tui` | 45+20t | Bubble Tea TUI, slash commands, UI states | 92% |

**Overall: ~93% feature complete**

---

## 1. cmd/claude — CLI Entry Point (95%)

**File**: `cmd/claude/main.go`

### CLI Flags
| Flag | Description |
|------|-------------|
| `-model <model>` | Override model (opus, sonnet, haiku, or full ID) |
| `-p` | Print mode (non-interactive, exit after response) |
| `-c` | Continue most recent session |
| `-r <id>` | Resume specific session by ID |
| `-max-tokens <n>` | Max response tokens |
| `-version` | Print version and exit |
| `-login` | Trigger OAuth login flow |
| `-dangerously-skip-permissions` | Bypass all permission prompts |
| `-permission-mode <mode>` | Set mode: default, plan, acceptEdits, bypassPermissions |
| `-output-format <fmt>` | Output format: text, json, stream-json |

### Subcommands
- `claude login [--email <email>] [--sso]` — OAuth login
- `claude logout` — Clear credentials
- `claude status [--json|--text]` — Auth status
- `claude auth status` — Compound subcommand form

### Initialization Sequence
1. Subcommand dispatch (before flag parsing)
2. Auth check → trigger login if unauthenticated
3. Settings loading from hierarchy
4. Hook config parsing from settings
5. Skills loading from ~/.claude/skills/ + .claude/skills/
6. Model resolution (CLI flag > settings > default)
7. API client creation
8. System prompt building
9. Permission mode validation (no bypass under sudo)
10. Tool registry setup with all tools
11. MCP server initialization
12. Session management (resume/create)
13. Output format selection
14. Hook firing (SessionStart)
15. Conversation loop or TUI launch

### What's Implemented
- Full CLI flags and subcommands
- Proper auth flow with OAuth integration
- Both print mode (json, stream-json, text) and TUI mode
- Pipe/stdin support for Unix workflows
- Fast mode resolution (Opus 4.6 requirement)
- MCP server startup before tool registration

### Gaps
- No `claude update` self-update command
- No `claude mcp` management subcommand
- No `claude agents` subcommand

---

## 2. internal/api — HTTP Client & Streaming (85%)

**Files**: client.go, streaming.go, types.go

### Types
- `Client` — HTTP client for Messages API with token-based auth
- `CreateMessageRequest` — Full request body with model, messages, system, tools, stream, speed
- `MessageResponse` — Complete response with content blocks, usage, stop reason
- `ContentBlock` — Union type: text, image, tool_use, tool_result (with cache control)
- `ToolDefinition` — Tool name, description, JSON schema (with cache control)
- `StreamHandler` — Interface for processing SSE events (8 methods)
- `TokenSource` / `RefreshableTokenSource` — Token provider interfaces

### Model Constants
- `ModelClaude46Opus = "claude-opus-4-6"`
- `ModelClaude46Sonnet = "claude-sonnet-4-6"`
- `ModelClaude45Haiku = "claude-haiku-4-5-20251001"`
- Fast mode: `FastModeBeta = "fast-mode-2026-02-01"`

### Client Features
- Bearer token auth via `TokenSource` interface
- All required headers: Authorization, Content-Type, anthropic-version (2023-06-01), anthropic-beta, x-app, User-Agent, Accept
- `?beta=true` query parameter
- 401 auto-retry with token refresh (RefreshableTokenSource)
- Fast mode: conditional beta header + speed field
- Streaming (`CreateMessageStream`) and non-streaming (`CreateMessage`) methods

### SSE Streaming
- Complete SSE parser with 64KB initial buffer, 10MB max line
- All event types: message_start, content_block_start/delta/stop, message_delta, message_stop, ping, error
- Text delta assembly into complete blocks
- Tool input JSON assembly from incremental `input_json_delta` fragments
- Response assembler multiplexes to handler AND accumulates final response

### Gaps
- No advanced retry strategies (only 401 retried)
- No rate limiting / backoff
- No request middleware / logging hooks

---

## 3. internal/auth — OAuth & Credentials (98%)

**Files**: oauth.go, credentials.go, profile.go, status.go, filelock_unix.go, filelock_other.go

### OAuth PKCE Flow
1. Generate code verifier (32 random bytes), challenge (SHA256 + base64url), state (32 random bytes)
2. Start local callback server on random port
3. Build authorization URL with PKCE params, scopes, client ID
4. Open browser (`open`/`xdg-open`/`rundll32`) with manual fallback
5. Wait for callback (5-min timeout) with code#state format for manual
6. Exchange authorization code at token endpoint
7. Fetch profile info, roles, create API key (non-fatal failures)

### Token Management
- **Storage**: `~/.claude/.credentials.json` (mode 0600)
- **Format**: JSON with `claudeAiOauth` (tokens), `oauthAccount` (metadata), `apiKey`
- **Refresh**: Automatic when <5 min to expiration
- **File locking**: fcntl flock on Unix (non-blocking, exclusive), no-op on Windows
- **Cross-process safety**: Lock file + re-read after acquire + jittered backoff (up to 5 retries)
- **Token hierarchy**: CLAUDE_CODE_OAUTH_TOKEN > CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR > stored credentials

### Auth Status Detection (Priority Order)
1. Third-party providers (Bedrock/Vertex/Foundry via env vars)
2. CLAUDE_CODE_OAUTH_TOKEN env var
3. ANTHROPIC_API_KEY env var
4. Stored claude.ai OAuth credentials
5. None

### Environment Variable Overrides
- `CLAUDE_CODE_OAUTH_TOKEN` — Direct token
- `CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR` — Read from FD
- `CLAUDE_CONFIG_DIR` — Override ~/.claude/
- `CLAUDE_CODE_OAUTH_CLIENT_ID` — Custom client ID
- `CLAUDE_CODE_CUSTOM_OAUTH_URL` — Custom OAuth URL (allowlisted)
- `CLAUDE_CODE_USE_BEDROCK/VERTEX/FOUNDRY` — Third-party providers

### What's Implemented
- Full OAuth PKCE with browser + manual fallback
- Token refresh with file locking
- Profile, roles, API key fetching
- Account metadata storage
- Auth status (JSON + text output)
- All environment variable overrides
- 78 test cases, zero TODOs

### Gaps
- Windows file locking is best-effort only
- API key created but not used by HTTP client

---

## 4. internal/config — Settings & Permissions (95%)

**Files**: settings.go, claudemd.go, permissions.go

### Settings Hierarchy (highest → lowest priority)
1. **Managed** — `/etc/claude/settings.json`
2. **CLI flags** — Applied after loading
3. **Local** — `.claude/settings.local.json` (gitignored)
4. **Project** — `.claude/settings.json` (committed)
5. **User** — `~/.claude/settings.json`

### Settings Fields
- `Permissions []PermissionRule` — Tool permission rules
- `Model string` — Default model
- `Env map[string]string` — Environment variables
- `Hooks json.RawMessage` — Hook definitions (parsed in Phase 7)
- `Sandbox json.RawMessage` — Sandbox config (stored, not parsed)
- `AutoCompactEnabled`, `Verbose`, `ThinkingEnabled`, `FastMode` — Bool pointers
- `EditorMode` — "normal" or "vim"
- `DiffTool` — "terminal" or "auto"
- `NotifChannel`, `Theme`, `DefaultPermissionMode`, `DisableBypassPermissions` — Strings
- `StatusLine *StatusLineConfig` — Custom status line command

### Merge Semantics
- Permissions: concatenated (overlay first = higher priority)
- Env: deep merge (overlay overrides per key)
- Booleans: overlay wins if non-nil
- Strings: overlay wins if non-empty
- DisableBypassPermissions: "once disabled, sticks"

### CLAUDE.md Loading
1. `~/.claude/CLAUDE.md` + `~/.claude/rules/` (user-level)
2. Walk from root to CWD, loading `CLAUDE.md` at each level
3. `.claude/CLAUDE.md` + `.claude/rules/` (project-level)
4. `@path` imports (relative, with cycle detection)
5. Rules directories: all `.md` files, sorted alphabetically

### Permission System
- **5 modes**: default, plan, acceptEdits, bypassPermissions, dontAsk
- **Decision priority** (8 levels):
  1. Permission mode (bypass/dontAsk → allow all; plan → read-only only)
  2. Session deny rules
  3. Session allow rules
  4. Settings-based rules (deny > ask > allow)
  5. Read-only command auto-allow (Bash only)
  6. Session ask rules
  7. Mode-specific (acceptEdits → allow edit tools)
  8. Fallback with suggestions

### Pattern Matching
- Two formats: Go `[{tool, pattern, action}]` and JS `{allow: [], deny: [], ask: []}`
- Patterns: `Bash(npm:*)`, `Read(./.env)`, `WebFetch(domain:example.com)`
- Wildcard matching: `*` matches any sequence, `?` matches single char
- Bash prefix matching: `npm` matches `npm install`, `npm run build`
- File glob: doublestar library for `**` support
- Domain substring matching

### Bash Security Checks
- Detects dangerous pipes: `curl|sh`, `wget|bash`, etc.
- Read-only command classification (20+ commands, git subcommand checks)
- Suggestion generation for user-friendly permission prompts

### Gaps
- Hooks stored as json.RawMessage, parsed elsewhere
- Sandbox not actively used

---

## 5. internal/conversation — Agentic Loop (90%)

**Files**: loop.go, history.go, compaction.go, system_prompt.go, cache.go, json_handlers.go

### Agentic Loop (loop.go)
```
SendMessage(userMessage)
  → RunUserPromptSubmit hook (can block/modify)
  → Add user message to history
  → run() loop:
      → Apply prompt caching
      → Apply fast mode if Opus 4.6
      → CreateMessageStream()
      → Add assistant response to history
      → Auto-compaction check
      → If stop_reason != tool_use: RunStop hook, return
      → If tool_use:
          → For each tool_use block:
              → Validate tool exists
              → RunPreToolUse hook
              → Execute tool
              → RunPostToolUse hook
          → Add tool results to history
          → Loop back
```

### History Management
- Simple `[]api.Message` slice
- Operations: AddUserMessage, AddAssistantResponse, AddToolResults, ReplaceRange, SetMessages
- `MakeToolResult()` helper for tool_result content blocks

### Context Compaction
- Trigger: `InputTokens >= 150,000` (configurable)
- Preserves last 4 messages (configurable)
- API-based summarization of older messages
- Summary preserves: key decisions, files modified, command outputs, task state

### System Prompt Construction (Two-Block Architecture)
**Block 1 — Core (stable, cached):**
1. Identity: Claude Code CLI
2. Security guardrails
3. Task philosophy (avoid over-engineering, simple solutions)
4. Action care (reversibility checks)
5. Environment (CWD, OS, date)

**Block 2 — Project (volatile):**
1. CLAUDE.md content
2. Active skills
3. Permission rules summary

Extensible via `RegisterCoreSection()` / `RegisterProjectSection()`

### Prompt Caching
- System blocks: all get `ephemeral` cache control
- Tools: only last tool gets cache control
- Messages: last 2 messages get cache control (skipping thinking blocks)
- Configurable via env vars: `DISABLE_PROMPT_CACHING`, per-model variants

### JSON Output Handlers
- `JSONStreamHandler` — Buffers full response, emits single JSON (--output-format json)
- `StreamJSONStreamHandler` — Emits one JSON line per event (--output-format stream-json)

### Gaps
- Hook implementations deferred to Phase 7 (interfaces defined, nil-guarded)
- No agent/sub-agent loop isolation (handled in tools package)

---

## 6. internal/hooks — Hook Event System (95%)

**Files**: types.go, runner.go

### Hook Events
| Event | When | Can Block |
|-------|------|-----------|
| PreToolUse | Before tool execution | Yes |
| PostToolUse | After tool execution | No |
| UserPromptSubmit | User sends message | Yes (can modify) |
| SessionStart | Session begins | No |
| PermissionRequest | Permission needed | No |
| Stop | Conversation ends | No |

### Implementation
- `HookConfig` holds hook definitions by event type
- `HookDef` has Type (command/prompt/agent), Command, Prompt fields
- `Runner` implements `conversation.HookRunner` interface
- Command hooks execute shell commands with env vars:
  `HOOK_EVENT`, `TOOL_NAME`, `TOOL_INPUT`, `TOOL_OUTPUT`, `TOOL_IS_ERROR`
- PostToolUse truncates output at 10KB for env var
- 17 test cases covering all events

### Gaps
- Prompt hooks return content but don't inject into conversation
- Agent hooks not spawning sub-agents (treated as no-ops)

---

## 7. internal/mcp — Model Context Protocol (85%)

**Files**: client.go, config.go, manager.go, sse.go, stdio.go, tools.go, types.go

### Protocol
- JSON-RPC 2.0 client with Initialize/ListTools/CallTool/ListResources/ReadResource/Subscribe/Unsubscribe
- Protocol version: `2024-11-05`

### Transports
- **StdioTransport**: Subprocess via stdin/stdout (10MB buffer, 5s graceful shutdown)
- **SSETransport**: HTTP Server-Sent Events (URL discovery, relative path resolution)

### Manager
- `StartServers()` — Connect, initialize, discover tools, register in tool registry
- `Shutdown()` — Close all servers gracefully
- `ServerStatus()` — Human-readable status per server

### MCP Tools (Wrapped for Tool Registry)
- `MCPToolWrapper` — Wraps server tool as `tools.Tool` interface
- `ListMcpResourcesTool` — Lists resources across all servers
- `ReadMcpResourceTool` — Reads resource by URI
- `SubscribeMcpResourceTool` / `UnsubscribeMcpResourceTool`
- `SubscribePollingTool` / `UnsubscribePollingTool` (interval-based)
- Global subscription store with `sub_<N>` IDs

### Config
- Loads ~/.mcp.json + <cwd>/.mcp.json (project overrides user)

### Gaps
- No resource update notification streaming
- Polling results not fed back to Claude
- No transport error recovery / reconnection

---

## 8. internal/session — Session Persistence (100%)

**Files**: session.go

### Implementation
- Sessions stored as JSON: `~/.claude/projects/<cwd-sha256-hash>/sessions/<id>.json`
- `Store.Save/Load/List/MostRecent` operations
- `GenerateID()` — nanosecond-precision timestamp
- Permissions: 0700 (dir), 0600 (files)
- Malformed files gracefully skipped during List
- 9 test cases, fully interoperable with official CLI format

---

## 9. internal/skills — Skill Loading (100%)

**Files**: loader.go, types.go

### Implementation
- Discovers skills from `~/.claude/skills/` + `<cwd>/.claude/skills/`
- Parses YAML frontmatter: `name`, `description`, `trigger` (slash command)
- Fallback to filename if no name in frontmatter
- `ActiveSkillContent()` formats combined markdown for system prompt
- Project skills override user skills (by name)
- 9 test cases

---

## 10. internal/tools — Tool Registry & Implementations (95%)

**Files**: registry.go, permission.go, background.go + 15 tool files

### Tool Interface
```go
type Tool interface {
    Name() string
    Description() string
    InputSchema() json.RawMessage
    Execute(ctx context.Context, input json.RawMessage) (string, error)
    RequiresPermission(input json.RawMessage) bool
}
```

### Registry
- Thread-safe with mutex, preserves registration order
- Two permission handler interfaces: simple (bool) and rich (PermissionResult)
- Rich handler decision flow: allow → execute, deny → error, ask/passthrough → interactive prompt

### Tool Inventory

| Tool | Permission | Status | Notes |
|------|-----------|--------|-------|
| **Bash** | Yes | Complete | Timeout (120s default, 600s max), env vars, 100KB output truncation |
| **FileRead** | No | Complete | cat -n format, images (base64), PDFs (pdftotext), Jupyter notebooks, 2000-line default |
| **FileEdit** | Yes | Complete | String replacement, replace_all, uniqueness validation, permission preservation |
| **FileWrite** | Yes | Complete | Absolute path validation, parent dir creation |
| **Glob** | No | Complete | doublestar library for `**`, sorted by mod time (newest first) |
| **Grep** | No | Complete | ripgrep primary + grep fallback, all context opts, offset/limit, multiline |
| **Agent** | No | ~95% | Background execution, resume, isolated Loop; max_turns accepted but not enforced |
| **TodoWrite** | No | Complete | Dual TUI/print modes, status tracking |
| **WebFetch** | Yes | Complete | HTTP GET, HTML→text, 15-min cache, 10MB body limit, 100KB truncation |
| **WebSearch** | No | Stub | Server-side tool by design; returns placeholder |
| **AskUserQuestion** | No | Complete | Multi-select, free text "Other", TUI/print modes |
| **NotebookEdit** | Yes | Complete | Replace/insert/delete modes, cell ID lookup, metadata preservation |
| **Config** | No | Complete | Nested dot-notation get/set, directory creation |
| **EnterWorktree** | Yes | Complete | git worktree creation, branch naming |
| **ExitPlanMode** | No | Complete | Plan completion signaling |
| **TaskOutput** | No | Complete | Blocking/polling with timeout |
| **TaskStop** | No | Complete | Context cancellation, 5s graceful shutdown |

### Background Task Management
- `BackgroundTaskStore`: thread-safe map with ID, context, cancel func, done channel, result
- Agent tool creates tasks for background execution
- TaskOutput polls/blocks on done channel
- TaskStop cancels context

### Gaps
- Agent tool: max_turns parameter accepted but ignored
- WebSearch: stub (by design, server-side in official CLI)

---

## 11. internal/tui — Terminal UI (92%)

**Files**: 45+ source files, 20+ test files

### Framework
Bubble Tea (charmbracelet/bubbletea) with glamour (markdown), lipgloss (styling), textarea (input)

### UI Modes
| Mode | Description |
|------|-------------|
| `modeInput` | Waiting for user text (default) |
| `modeStreaming` | Receiving API response |
| `modePermission` | Permission y/n prompt |
| `modeAskUser` | Structured question prompt |
| `modeResume` | Session picker |
| `modeModelPicker` | Model selection |
| `modeDiff` | Diff dialog |
| `modeConfig` | Config panel |
| `modeHelp` | Help screen |

### Slash Commands (20 built-in)
| Command | Aliases | Description |
|---------|---------|-------------|
| `/help` | | Show help and available commands |
| `/clear` | `/reset`, `/new` | Clear conversation history |
| `/resume` | | Resume previous session (picker) |
| `/continue` | | Resume most recent session |
| `/login` | | Sign in (exits to re-auth) |
| `/logout` | | Sign out |
| `/version` | | Show version |
| `/cost` | | Show token usage and cost |
| `/context` | | Show context window usage |
| `/mcp` | | Show MCP server status |
| `/fast` | | Toggle fast mode |
| `/config` | `/settings` | Open config panel |
| `/model` | | Show/switch model |
| `/diff` | | View uncommitted changes |
| `/memory` | | Edit CLAUDE.md (opens editor) |
| `/compact` | | Compact conversation history |
| `/init` | | Initialize project CLAUDE.md |
| `/review` | | Review a pull request |
| `/quit` | `/exit` | Exit the program |

Skills with triggers also registered as dynamic slash commands.

### Key Bindings
**Input mode**: Ctrl-C (double-press exit), Tab (fuzzy complete / accept suggestion), Shift+Tab (cycle permission modes), Escape (clear), Enter (submit), ? (help when empty)

**Streaming**: Ctrl-C (cancel), Enter (queue message), Escape (clear/dequeue)

**Permission**: y (allow), n (deny), a (always allow), Ctrl-C (deny+cancel)

### Features
- **Markdown rendering** via glamour with dark/light detection
- **Inline diff display** for FileEdit (red/green +/- lines)
- **Streaming text** accumulated during streaming, rendered on block stop
- **Status bar**: model name, token counts, fast mode, permission mode
- **Custom status line**: configurable via settings (external command)
- **Dynamic prompt suggestions**: API-generated, shown as placeholder, Tab to accept
- **Fuzzy completion**: slash commands, prefix + subsequence matching
- **Config panel**: bool toggles, enum selectors, search filtering
- **Memory editing**: opens $VISUAL/$EDITOR with CLAUDE.md
- **Diff dialog**: list/detail views, git diff parsing, colored hunks
- **Session resume**: picker with time, message count, first message preview
- **Model picker**: interactive selection from available models
- **Permission prompt**: y/n/a with rule suggestions
- **AskUser prompt**: multi-select, custom text, question-by-question
- **Todo list**: colored status icons, activeForm during in_progress
- **Message queue**: FIFO during streaming, auto-submit on turn complete

### Gaps
- Cost tracking shows `TotalCostUSD: 0` (not yet implemented)
- Thinking mode: config option present, rendering depends on API
- Verbose output: config option present, logging not implemented

---

## 12. internal/mock — Test Infrastructure (90%)

**Files**: backend.go, responder.go, sse.go, token.go

### Components
- `Backend` — Mock HTTP server wrapping httptest, captures requests
- `Responder` interface with implementations:
  - `StaticResponder` — Always same response
  - `ScriptedResponder` — Plays sequence (repeats last)
  - `EchoResponder` — Echoes last user message
  - `ResponderFunc` — Function adapter
- `WriteSSEResponse()` — Generates complete SSE stream from MessageResponse
- `StaticTokenSource` — Fixed token for testing
- `CapturedRequest` — Records method, path, headers, parsed body
  - `ToolResults()` / `AllToolResults()` for assertion

---

## Identified Gaps Summary

### Not Implemented
1. `claude update` self-update command
2. `claude mcp` management subcommand (server add/remove/list)
3. `claude agents` subcommand
4. Cost tracking (token usage displayed, USD cost not calculated)
5. API key authentication path (subscription-only currently)
6. Bedrock/Vertex/Foundry backend integration (env vars detected but not routed)
7. `/bug` feedback submission command
8. `/doctor` diagnostic command
9. Git checkpoint system (snapshot before edits)
10. Agent tool max_turns enforcement

### Partially Implemented
1. Hook system: command hooks working; prompt/agent hooks are no-ops
2. MCP subscriptions: subscribe/unsubscribe work; update notifications not streaming
3. WebSearch: stub by design (server-side tool)
4. Verbose mode: config toggle present, no verbose logging
5. Thinking mode: config toggle present, rendering untested

### Complete
1. OAuth PKCE flow with browser + manual fallback
2. Token refresh with cross-process file locking
3. Settings hierarchy (5 levels) with full merge semantics
4. CLAUDE.md loading with @path imports and rules directories
5. Permission system (5 modes, 8-level priority, pattern matching)
6. Agentic loop with parallel tool calls
7. SSE streaming with tool JSON assembly
8. Context compaction via API summarization
9. Prompt caching (system, tools, messages)
10. Session persistence (save, load, resume, list)
11. All 17 tools (15 complete, 1 stub by design, 1 at 95%)
12. Full TUI with 9 UI modes and 20 slash commands
13. Skill loading and registration
14. MCP client with stdio + SSE transports
15. JSON and stream-JSON output formats
16. Print mode (non-interactive) support
