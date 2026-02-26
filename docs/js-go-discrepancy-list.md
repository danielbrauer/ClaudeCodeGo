# JS vs Go: Prioritized Discrepancy List

Comparison of the official Claude Code CLI (JS, v2.1.50) against the Go reimplementation.
Ranked from most to least severe.

---

## #1 — CRITICAL: System Prompt Does Not Match JS Version

**Severity: P0 — Must fix first**

The Go system prompt construction differs fundamentally from the JS CLI in structure, content injection mechanism, and context placement.

### Differences

| Aspect | JS CLI | Go CLI | Impact |
|--------|--------|--------|--------|
| **Context injection** | Git status, date, CLAUDE.md injected as `<system-reminder>` blocks in user messages via `userContext`/`systemContext` | All context embedded directly in system prompt blocks | Model sees different message structure |
| **Git status** | Explicitly included: branch, recent commits, working tree status | **Not included at all** | Model lacks repo awareness |
| **System prompt assembly** | Dynamic `owq()` function merges system prompt + systemContext at query time; `TN1()` injects userContext into messages | Static two-block architecture (core + project) built once | Different caching and freshness behavior |
| **Tool descriptions** | Generated dynamically, sent as separate `tools` array with descriptions in the tool definitions | Same pattern but descriptions may differ in wording | Behavioral differences |
| **CLAUDE.md location** | Injected into userContext as `claudeMd` key, appears in user messages | Injected into system prompt Block 2 as "Project Instructions" | Different prompt position affects model behavior |
| **Date injection** | Added to userContext as `currentDate` key | Added to system prompt environment section | Minor positional difference |

### Required Fix

1. Add git status collection and injection (branch, recent commits, clean/dirty state)
2. Move CLAUDE.md and date from system prompt to user message context (matching `<system-reminder>` pattern)
3. Match the exact system prompt text from cli.js (identity, security, philosophy sections)
4. Ensure tool descriptions match word-for-word

---

## #2 — CRITICAL: Missing Beta Headers (8+ conditional betas)

**Severity: P0**

The Go client sends only 2 static beta headers. The JS CLI sends up to 15 conditional beta headers that control access to critical model features.

### Missing Betas

| Beta Header | Condition in JS | Feature Blocked |
|-------------|-----------------|-----------------|
| `interleaved-thinking-2025-05-14` | Model supports thinking + env not disabled | Extended thinking / interleaved thinking |
| `context-1m-2025-08-07` | Model has `[1m]` suffix | 1M token context window |
| `context-management-2025-06-27` | `USE_API_CONTEXT_MANAGEMENT` env var | API-level context management |
| `structured-outputs-2025-12-15` | `tengu_tool_pear` feature flag | Structured output support |
| `web-search-2025-03-05` | Vertex/Foundry backends | Web search tool |
| `tool-examples-2025-10-29` | Experimental betas enabled + model >= 4.0 | Tool examples in prompts |
| `advanced-tool-use-2025-11-20` | Unknown condition | Advanced tool use features |
| `tool-search-tool-2025-10-19` | Unknown condition | Tool search/deferred loading |
| `effort-2025-11-24` | Unknown condition | Effort level control |
| `adaptive-thinking-2026-01-28` | Unknown condition | Adaptive thinking |
| `prompt-caching-scope-2026-01-05` | Experimental betas enabled | Prompt caching scope |
| `files-api-2025-04-14` | Unknown condition | Files API support |
| `skills-2025-10-02` | Skills active | Skills system |

### Also Missing

- `ANTHROPIC_BETAS` environment variable parsing (user-specified custom betas)
- `betas` array in request body (JS sends betas both as header AND in request body)
- `?beta=true` query parameter is hardcoded in Go but not used in JS (may cause issues)

### Present in Go

- `claude-code-20250219` ✓
- `oauth-2025-04-20` ✓
- `fast-mode-2026-02-01` ✓ (conditional on fast mode)

---

## #3 — HIGH: Missing CLI Flags (~28 flags absent)

**Severity: P1**

Go implements 10 CLI flags. JS has ~40. Missing flags prevent usage in SDK/scripting scenarios and limit interactive configuration.

### Missing Flags by Category

**Session management (6 missing):**
- `--session-id <uuid>` — Specify session UUID
- `--fork-session` — New session ID when resuming
- `--from-pr [value]` — Resume session linked to PR
- `--no-session-persistence` — Don't save sessions (print mode)
- `--resume-session-at <id>` — Resume to specific message
- `--rewind-files <id>` — Restore files at message

**Model/thinking control (5 missing):**
- `--effort <level>` — low, medium, high, max
- `--thinking <mode>` — enabled, adaptive, disabled
- `--max-thinking-tokens <n>` — Max thinking tokens
- `--betas <betas...>` — Additional beta headers
- `--fallback-model <model>` — Fallback on overload

**Permission control (5 missing):**
- `--allow-dangerously-skip-permissions` — Enable bypass option
- `--allowedTools <tools...>` — Allow specific tools
- `--disallowedTools <tools...>` — Deny specific tools
- `--tools <tools...>` — Specify available tools
- `--permission-prompt-tool <tool>` — MCP tool for permission prompts

**System prompt overrides (4 missing):**
- `--system-prompt <prompt>` — Custom system prompt
- `--system-prompt-file <file>` — System prompt from file
- `--append-system-prompt <prompt>` — Append to default
- `--append-system-prompt-file <file>` — Append from file

**Output/input format (4 missing):**
- `--input-format <fmt>` — text or stream-json
- `--json-schema <schema>` — Structured output schema
- `--include-partial-messages` — Include partial chunks
- `--replay-user-messages` — Re-emit user messages

**Agent control (3 missing):**
- `--agent <agent>` — Agent for session
- `--max-turns <n>` — Max agentic turns (print mode)
- `--max-budget-usd <amount>` — Max spend (print mode)

**Debug/development (4 missing):**
- `-d/--debug [filter]` — Debug mode
- `--verbose` — Verbose setting
- `--debug-file <path>` — Write debug logs to file
- `--mcp-debug` — MCP debug (deprecated)

**Other (4 missing):**
- `--settings <file-or-json>` — Additional settings
- `--add-dir <dirs...>` — Additional directories
- `--mcp-config <configs...>` — MCP server config
- `--strict-mcp-config` — Only use specified servers

---

## #4 — HIGH: Tool Schema Discrepancies

**Severity: P1**

Multiple tool schemas don't match `sdk-tools.d.ts`. This causes the model to generate parameters the Go code ignores or rejects.

### Agent Tool (Most Critical)

Missing 4 parameters from schema:
- `name` — Name for spawned agent
- `team_name` — Team name for spawning
- `mode` — Permission mode enum: `acceptEdits | bypassPermissions | default | dontAsk | plan`
- `isolation` — Isolation mode enum: `worktree`

### Bash Tool

Missing 2 parameters:
- `run_in_background` — Run command in background (parameter exists but not in schema)
- `dangerouslyDisableSandbox` — Override sandbox mode

### Config Tool

Type mismatch:
- SDK restricts `value` to `string | boolean | number`
- Go accepts any `interface{}`

### TaskOutput Tool

Requiredness mismatch:
- SDK: `block` and `timeout` are required
- Go: marked `omitempty` (optional)

### Grep Tool

JSON tag issue:
- Fields like `-B`, `-A`, `-C`, `-n`, `-i` use hyphen-prefixed JSON tags
- Standard Go `encoding/json` treats `-` as "skip field"
- Custom unmarshaling works around this, but schema definition may confuse

---

## #5 — HIGH: Missing Slash Commands (7 commands)

**Severity: P1**

### Missing from Go

| Command | Purpose | Impact |
|---------|---------|--------|
| `/doctor` | Diagnose configuration/auth issues | User troubleshooting |
| `/theme` | Change color theme | UX customization |
| `/vim` | Toggle vim mode | Editor preference |
| `/permissions` | View/edit permission rules | Security management |
| `/hooks` | View configured hooks | Hook introspection |
| `/agents` | Configure subagents | Agent management |
| `/add-dir` | Add additional directories | Multi-directory support |
| `/install` | Install skills/plugins | Plugin ecosystem |
| `/status` | Show session status | Session monitoring |

### Go Has Extra Commands Not In JS

| Command | Purpose |
|---------|---------|
| `/version` | Show version (JS uses `claude --version`) |
| `/resume` | Resume session (JS uses `-r` flag) |
| `/continue` | Continue session (JS uses `-c` flag) |
| `/quit` | Exit program |
| `/diff` | View uncommitted changes |

---

## #6 — HIGH: Missing Subcommands

**Severity: P1**

### Missing from Go

| Subcommand | Purpose |
|------------|---------|
| `claude update` | Self-update mechanism |
| `claude mcp [subcommand]` | MCP server management (add, remove, list) |
| `claude agents` | List configured agents |

---

## #7 — MEDIUM: Missing HTTP Headers & Request Body Fields

**Severity: P2**

### Missing Headers

| Header | Purpose |
|--------|---------|
| `X-Stainless-Retry-Count` | Retry tracking for Stainless SDK |
| `X-Stainless-Timeout` | Timeout tracking |
| `x-claude-remote-container-id` | Remote container identification |
| `x-claude-remote-session-id` | Remote session identification |
| `x-client-app` | Client application identifier |
| `x-anthropic-additional-protection` | Additional protection flag |
| Custom headers from `ANTHROPIC_CUSTOM_HEADERS` | User-specified headers |

### Missing Request Body Fields

| Field | Purpose |
|-------|---------|
| `betas` array | JS sends beta list in body AND header |
| `thinking` config | Extended thinking budget tokens |
| `output_config` | Structured output format |
| `tool_choice` | Tool selection constraint |

---

## #8 — MEDIUM: Hook System Incomplete

**Severity: P2**

### Status

| Feature | Status |
|---------|--------|
| Command hooks (PreToolUse, PostToolUse, etc.) | ✓ Implemented |
| Prompt hooks | ✗ Return content but don't inject into conversation |
| Agent hooks | ✗ No-ops (don't spawn sub-agents) |
| PostToolUseFailure event | ✗ Missing (only PostToolUse exists) |

---

## #9 — MEDIUM: MCP Gaps

**Severity: P2**

| Feature | Status |
|---------|--------|
| stdio transport | ✓ |
| SSE transport | ✓ |
| Tool discovery + execution | ✓ |
| Resource listing + reading | ✓ |
| Subscribe/unsubscribe | ✓ |
| Resource update notification streaming | ✗ Missing |
| Polling results not fed back to Claude | ✗ Missing |
| Transport error recovery / reconnection | ✗ Missing |
| `--mcp-config` CLI flag | ✗ Missing |
| `--strict-mcp-config` CLI flag | ✗ Missing |
| MCP proxy support (`mcp-proxy.anthropic.com`) | ✗ Missing |

---

## #10 — MEDIUM: Cost Tracking Not Implemented

**Severity: P2**

The `/cost` command exists but shows `TotalCostUSD: 0`. Token-to-USD pricing calculations are not implemented.

---

## #11 — MEDIUM: Git Checkpoint System Missing

**Severity: P2**

JS CLI creates automatic git snapshots before file modifications, allowing revert to pre-edit state. Go does not implement this.

---

## #12 — LOW: Thinking/Effort Mode Support

**Severity: P3**

- Config toggle present for thinking mode
- No `--effort` or `--thinking` CLI flags
- No thinking budget token support in API requests
- Thinking delta events not rendered in TUI

---

## #13 — LOW: Missing Environment Variable Support

**Severity: P3**

Several env vars recognized by JS but not Go:

| Variable | Purpose |
|----------|---------|
| `CLAUDE_CODE_DISABLE_AUTO_MEMORY` | Disable auto memory |
| `CLAUDE_CODE_REMOTE` | Remote mode flag |
| `CLAUDE_CODE_REMOTE_MEMORY_DIR` | Remote memory directory |
| `CLAUDE_CODE_USE_VERTEX` | Vertex AI backend |
| `CLAUDE_CODE_USE_FOUNDRY` | Foundry backend |
| `DISABLE_COMPACT` | Disable compaction |
| `ANTHROPIC_BETAS` | Custom beta headers |
| `ANTHROPIC_CUSTOM_HEADERS` | Custom HTTP headers |

---

## #14 — LOW: Verbose/Debug Mode Not Implemented

**Severity: P3**

Config toggle exists but no verbose logging output is produced. Debug mode flags (`-d`, `--debug-file`) are missing.

---

## #15 — LOW: Telemetry Not Implemented

**Severity: P3**

JS CLI sends telemetry events to `/api/event_logging/batch` and OpenTelemetry metrics to `/api/claude_code/metrics`. Go does not implement telemetry. This is intentionally skipped per CLAUDE.md.

---

## Summary Statistics

| Category | JS | Go | Coverage |
|----------|----|----|----------|
| CLI flags | ~40 | 10 | 25% |
| Slash commands | 24 | 19 | 79% |
| Tools (schemas correct) | 24 | ~17 correct | 71% |
| Beta headers | 15 | 3 | 20% |
| Hook events | 6 | 5 | 83% |
| Subcommands | 5+ | 3 | 60% |

**Overall feature parity estimate: ~75%** (the Go analysis estimated 93% but that was before detailed schema comparison revealed many partial implementations)
