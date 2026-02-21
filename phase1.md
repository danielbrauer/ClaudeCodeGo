# Phase 1: Foundation

Completed: 2026-02-21

## Goal

Get a basic conversation working end-to-end: OAuth login, API call, streamed response displayed in the terminal.

## What was built

### Project setup
- Go module `github.com/anthropics/claude-code-go` (Go 1.24.7)
- Directory structure: `cmd/claude/`, `internal/{api,auth,config,conversation,hooks,mcp,session,tools,tui}/`

### `internal/api` — Messages API client

**`types.go`** — Request/response structs matching the Messages API:
- `CreateMessageRequest` with model, max_tokens, messages, system, tools, stream
- `Message` with role and flexible content (string or `[]ContentBlock`)
- `ContentBlock` union type covering text, image, tool_use, and tool_result
- `ToolDefinition` for sending tool schemas to the API
- `MessageResponse` with content blocks, stop_reason, usage
- Model constants for Claude 4 Opus, Sonnet, and 3.5 Haiku

**`streaming.go`** — SSE stream parser:
- Line-by-line parser that handles `event:` and `data:` fields per the SSE spec
- Typed event structs for all message lifecycle events: `message_start`, `content_block_start`, `content_block_delta` (text_delta + input_json_delta), `content_block_stop`, `message_delta`, `message_stop`
- `StreamHandler` interface — callers implement this to receive events as they arrive
- 10MB line buffer to handle large tool call JSON deltas

**`client.go`** — HTTP client:
- `TokenSource` interface for pluggable auth
- Streaming POST to `/v1/messages` with required headers:
  - `Authorization: Bearer {token}`
  - `anthropic-version: 2023-06-01`
  - `anthropic-beta: oauth-2025-04-20`
  - `x-app: cli`
- `responseAssembler` wraps a `StreamHandler` and collects all events into a final `MessageResponse`, including assembling incremental `input_json_delta` chunks into complete tool call JSON
- Configurable via `ClientOption` functions (model, max tokens, base URL, HTTP client)

### `internal/auth` — OAuth authentication

**`oauth.go`** — PKCE browser login flow:
- Reverse-engineered from the official `cli.js` (v2.1.50)
- OAuth configuration extracted:
  - Client ID: `9d1c250a-e61b-44d9-88ed-5944d1962f5e`
  - Authorize URL: `https://claude.ai/oauth/authorize`
  - Token URL: `https://platform.claude.com/v1/oauth/token`
  - Scopes: `user:profile user:inference user:sessions:claude_code user:mcp_servers org:create_api_key`
- Flow: generate PKCE verifier/challenge (S256) → start local HTTP server on random port → build authorize URL → open browser → wait for `/callback` with auth code → exchange code for tokens at token endpoint
- Browser open: `open` (macOS), `xdg-open` (Linux), `rundll32` (Windows)
- 5-minute timeout waiting for browser callback

**`credentials.go`** — Token storage and refresh:
- Reads/writes `~/.claude/.credentials.json` with `0600` permissions
- Matches the official CLI's file format: tokens stored under the `claudeAiOauth` key with fields `accessToken`, `refreshToken`, `expiresAt`, `scopes`, `subscriptionType`, `rateLimitTier`
- `TokenProvider` implements `api.TokenSource` and handles:
  - Loading tokens from disk (or `CLAUDE_CODE_OAUTH_TOKEN` env var)
  - Automatic refresh when token expires within 5 minutes
  - Persisting refreshed tokens back to disk
  - Thread-safe access via mutex

### `internal/conversation` — Agentic loop

**`system_prompt.go`** — System prompt assembly:
- Loads CLAUDE.md from multiple locations (matching official behavior):
  - `~/.claude/CLAUDE.md` (user-level)
  - Every directory from filesystem root to CWD
  - `.claude/CLAUDE.md` (project-level)
- Injects environment info (working directory, platform, date)

**`history.go`** — Message history:
- Append user messages, assistant responses, and tool results
- Helper to create `tool_result` content blocks

**`loop.go`** — Core agentic loop:
- Sends messages to the API with system prompt and tool definitions
- Streams the response via `StreamHandler`
- If stop_reason is `tool_use`: extracts tool calls, executes them via `ToolExecutor` interface, appends results, loops back
- If stop_reason is `end_turn`: returns (conversation turn complete)
- `PrintStreamHandler` prints streamed text deltas to stdout

### `cmd/claude` — CLI entry point

**`main.go`** — Flag parsing and REPL:
- Flags: `--model`, `-p` (print mode), `-c` (continue session), `-r` (resume session), `--max-tokens`, `--login`, `--version`
- Auto-login: if no credentials found, triggers the OAuth flow automatically
- Interactive REPL with prompt (`> `), reads from stdin
- Slash commands: `/help`, `/model`, `/quit`, `/exit`, `/version`
- Ctrl+C handling via context cancellation
- Accepts initial prompt as positional arguments (`claude "what is 2+2"`)

### Tests

**`internal/api/streaming_test.go`**:
- Text response: verifies all events are dispatched correctly for a simple text reply
- Tool use: verifies input_json_delta assembly and tool_use stop_reason
- Ping: verifies keepalive pings are ignored

**`internal/auth/credentials_test.go`**:
- Save/load round-trip with file permission verification (0600)
- Code challenge determinism (same verifier → same challenge)
- Code verifier uniqueness (two calls → different values)

## Key decisions

- **No external dependencies** — Phase 1 uses only the Go standard library. The `net/http` server handles OAuth callbacks, `crypto/sha256` handles PKCE, `bufio.Scanner` handles SSE parsing.
- **Interface-driven design** — `TokenSource`, `StreamHandler`, and `ToolExecutor` interfaces allow clean separation. Phase 2 tools plug into `ToolExecutor` without changing the loop.
- **Match official CLI formats** — Credentials file location (`~/.claude/.credentials.json`), JSON structure (`claudeAiOauth` key), and CLAUDE.md loading order all match the official implementation for interoperability.

## What's next (Phase 2)

The `ToolExecutor` interface is wired into the loop but has no implementations. Phase 2 adds:
- Tool interface and registry
- Tool schema generation (JSON schemas matching `sdk-tools.d.ts`)
- Bash, FileRead, FileEdit, FileWrite, Glob, Grep tools
- Permission prompts before tool execution
