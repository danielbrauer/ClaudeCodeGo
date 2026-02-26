# JS CLI Feature Inventory

Structured inventory of the official Claude Code CLI (`@anthropic-ai/claude-code` v2.1.50) extracted from `cli.js` (587K lines) and `sdk-tools.d.ts` (87KB).

---

## 1. Authentication & OAuth

### OAuth Configuration (Production)

| Key | Value |
|-----|-------|
| BASE_API_URL | `https://api.anthropic.com` |
| CONSOLE_AUTHORIZE_URL | `https://platform.claude.com/oauth/authorize` |
| CLAUDE_AI_AUTHORIZE_URL | `https://claude.ai/oauth/authorize` |
| TOKEN_URL | `https://platform.claude.com/v1/oauth/token` |
| API_KEY_URL | `https://api.anthropic.com/api/oauth/claude_cli/create_api_key` |
| ROLES_URL | `https://api.anthropic.com/api/oauth/claude_cli/roles` |
| CONSOLE_SUCCESS_URL | `https://platform.claude.com/buy_credits?returnUrl=/oauth/code/success%3Fapp%3Dclaude-code` |
| CLAUDEAI_SUCCESS_URL | `https://platform.claude.com/oauth/code/success?app=claude-code` |
| MANUAL_REDIRECT_URL | `https://platform.claude.com/oauth/code/callback` |
| CLIENT_ID | `9d1c250a-e61b-44d9-88ed-5944d1962f5e` |
| MCP_PROXY_URL | `https://mcp-proxy.anthropic.com` |
| MCP_PROXY_PATH | `/v1/mcp/{server_id}` |

### OAuth Configuration (Staging/Local)

| Key | Value |
|-----|-------|
| CLIENT_ID | `22422756-60c9-4084-8eb7-27705fd5cf9a` |
| BASE_API_URL | `http://localhost:3000` |
| CLAUDE_AI_AUTHORIZE_URL | `http://localhost:4000/oauth/authorize` |
| OAUTH_FILE_SUFFIX | `-local-oauth` |

### OAuth Scopes

- Console scopes: `org:create_api_key`, `user:profile`
- Claude.ai scopes: `user:profile`, `user:inference`, `user:sessions:claude_code`, `user:mcp_servers`
- Combined scopes for full access: union of both sets

### OAuth Beta Header

`oauth-2025-04-20` (constant `KG`)

### PKCE Flow

- Code verifier: 32 random bytes, base64url-encoded
- Code challenge: SHA-256 of code verifier, base64url-encoded
- State parameter: 32 random bytes, base64url-encoded
- Base64url encoding: replace `+` with `-`, `/` with `_`, strip `=`

### Local Callback Server

- HTTP server on `localhost` (dynamic port, or specified)
- Callback path: `/callback`
- Validates state parameter, extracts authorization code
- Redirects to success URL after successful auth
- Class: `_V8` in cli.js

### Token Storage

- Config directory: `~/.claude/` (or `$CLAUDE_CONFIG_DIR`)
- `OA()` function returns config dir path
- On macOS: uses keychain storage (`FY4` function with platform-specific backends)
- OAuth file suffix can be customized (e.g., `-custom-oauth`, `-local-oauth`)

### Token Refresh

- `_M1` function wraps API calls with automatic 401 retry
- On 401: calls `gh()` to refresh token, then retries original request
- Telemetry event: `tengu_grove_oauth_401_received`

### Auth Methods

- **OAuth subscription**: Primary (Pro/Team/Enterprise via `claude.ai` or `platform.claude.com`)
- **API key**: Via `x-api-key` header and `anthropic-beta` header
- **Custom OAuth URL**: Via `CLAUDE_CODE_CUSTOM_OAUTH_URL` env var (restricted to approved endpoints)
- **Custom client ID**: Via `CLAUDE_CODE_OAUTH_CLIENT_ID` env var

### Auth-Related API Endpoints

| Endpoint | Purpose |
|----------|---------|
| `/api/oauth/profile` | Fetch user profile |
| `/api/claude_cli_profile` | Fetch CLI-specific profile |
| `/api/oauth/account/settings` | Get/update account settings |
| `/api/oauth/account/grove_notice_viewed` | Mark grove notice as viewed |
| `/api/oauth/claude_cli/client_data` | Get client configuration data |
| `/api/oauth/usage` | Get usage data |
| `/api/oauth/organizations/{org}/code/sessions` | List code sessions |
| `/api/oauth/organizations/{org}/admin_requests` | Admin request management |
| `/api/oauth/organizations/{org}/admin_requests/eligibility` | Check request eligibility |
| `/api/oauth/organizations/{org}/referral/eligibility` | Check referral eligibility |
| `/api/oauth/organizations/{org}/referral/redemptions` | Get referral redemptions |

---

## 2. API Client

### Base URL Selection

- Production: `https://api.anthropic.com`
- Staging: `https://api-staging.anthropic.com`
- Custom: via `ANTHROPIC_BASE_URL` env var
- Vertex AI and AWS Bedrock backends also supported (separate paths)

### API Version Header

```
anthropic-version: 2023-06-01
```

### Beta Headers (`anthropic-beta`)

| Beta Flag | Purpose |
|-----------|---------|
| `claude-code-20250219` | Main CLI beta |
| `interleaved-thinking-2025-05-14` | Interleaved thinking |
| `context-1m-2025-08-07` | 1M context window |
| `context-management-2025-06-27` | Context management |
| `structured-outputs-2025-12-15` | Structured outputs |
| `web-search-2025-03-05` | Web search |
| `tool-examples-2025-10-29` | Tool examples |
| `advanced-tool-use-2025-11-20` | Advanced tool use |
| `tool-search-tool-2025-10-19` | Tool search |
| `effort-2025-11-24` | Effort control |
| `adaptive-thinking-2026-01-28` | Adaptive thinking |
| `prompt-caching-scope-2026-01-05` | Prompt caching scope |
| `fast-mode-2026-02-01` | Fast mode |
| `files-api-2025-04-14` | Files API |
| `skills-2025-10-02` | Skills |

### Non-Messages API Endpoints

| Endpoint | Purpose |
|----------|---------|
| `/api/hello` | Connectivity check |
| `/api/claude_code/policy_limits` | Policy/rate limits |
| `/api/claude_code/settings` | Remote settings |
| `/api/claude_code/user_settings` | User settings |
| `/api/claude_code/metrics` | OpenTelemetry metrics export |
| `/api/claude_code/link_vcs_account` | Link VCS account |
| `/api/claude_cli_feedback` | Bug report submission |
| `/api/event_logging/batch` | Telemetry events |
| `/api/web/domain_info` | Domain fetch allowlist check |
| `/api/claude_code_grove` | Grove (extended context) config |
| `/api/claude_code_penguin_mode` | Penguin mode check |
| `/api/claude_code/organizations/metrics_enabled` | Org metrics check |

### HTTP Client

- Uses `axios` (aliased as `l8`) for all HTTP requests
- User-Agent header included on all requests
- Timeout defaults vary by endpoint (typically 5000ms for metadata, 30000ms for feedback)

---

## 3. SSE Streaming

### Event Types

| Event | Purpose |
|-------|---------|
| `message_start` | New message begins, contains message metadata |
| `content_block_start` | New content block (text or tool_use) |
| `content_block_delta` | Incremental update to content block |
| `content_block_stop` | Content block complete |
| `message_delta` | Message-level update (stop_reason, usage) |
| `message_stop` | Message complete |

### Delta Types (in `content_block_delta`)

| Delta Type | Content |
|------------|---------|
| `text_delta` | Incremental text: `{"type": "text_delta", "text": "..."}` |
| `input_json_delta` | Incremental tool input JSON: `{"type": "input_json_delta", "partial_json": "..."}` |
| `thinking_delta` | Thinking text (interleaved thinking) |
| `signature_delta` | Thinking signature |

### Tool Call Assembly

1. `content_block_start` arrives with `type: "tool_use"`, `id`, `name`, `input: {}`
2. Multiple `content_block_delta` events with `input_json_delta` containing partial JSON strings
3. `content_block_stop` signals completion
4. Partial JSON strings are concatenated and parsed

### Event Validation

- Events must arrive in order: `message_start` first
- `message_stop` before next `message_start`
- Content blocks must start before deltas
- Errors logged for unexpected event ordering

### Processed Events (lines ~153590-155070, ~156240-156310)

Two separate SSE processing pipelines exist (likely for different API backends), both handling the same event types.

---

## 4. Tool System

### Tool Registry

- 24 built-in tools defined in `sdk-tools.d.ts`
- Tools registered with `get inputSchema()` pattern (31 occurrences in cli.js)
- Tool dispatch via `toolName` field
- MCP tools dynamically added with `mcp__<server>__<tool>` naming

### Built-in Tools (from sdk-tools.d.ts)

#### Core Execution Tools

| Tool | API Name | Description |
|------|----------|-------------|
| Bash | `Bash` | Execute shell commands with timeout, background support |
| Agent/Task | `Task` | Spawn sub-agents with isolated context |
| TaskOutput | `TaskOutput` | Read output from background tasks |
| TaskStop | `TaskStop` | Stop background tasks |
| ExitPlanMode | `ExitPlanMode` | Signal completion of a plan |

#### File Operation Tools

| Tool | API Name | Description |
|------|----------|-------------|
| FileRead | `Read` | Read files with offset/limit, images, PDFs, notebooks |
| FileEdit | `Edit` | String replacement in files with replace_all |
| FileWrite | `Write` | Create or overwrite files |
| Glob | `Glob` | File pattern matching (returns up to 100 files) |
| Grep | `Grep` | Content search via ripgrep-compatible regex |
| NotebookEdit | `NotebookEdit` | Edit Jupyter notebook cells |

#### User Interaction Tools

| Tool | API Name | Description |
|------|----------|-------------|
| AskUserQuestion | `AskUserQuestion` | Structured questions with 2-4 options per question, 1-4 questions |
| TodoWrite | `TodoWrite` | Manage structured task list (pending/in_progress/completed) |

#### Web Tools

| Tool | API Name | Description |
|------|----------|-------------|
| WebFetch | `WebFetch` | Fetch URL content, process with prompt |
| WebSearch | `WebSearch` | Web search with domain filtering |

#### Configuration Tools

| Tool | API Name | Description |
|------|----------|-------------|
| Config | `Config` | Get/set configuration values |
| EnterWorktree | `EnterWorktree` | Create isolated git worktree |

#### MCP Tools

| Tool | API Name | Description |
|------|----------|-------------|
| McpInput | dynamic | Dynamic schema per MCP tool |
| ListMcpResources | `ListMcpResources` | List MCP resources |
| ReadMcpResource | `ReadMcpResource` | Read MCP resource content |
| SubscribeMcpResource | `SubscribeMcpResource` | Subscribe to resource changes |
| UnsubscribeMcpResource | `UnsubscribeMcpResource` | Unsubscribe from resources |
| SubscribePolling | `SubscribePolling` | Poll tool/resource at intervals (min 1000ms) |
| UnsubscribePolling | `UnsubscribePolling` | Stop polling |

### Tool Input/Output Schemas

See `sdk-tools.d.ts` for complete type definitions. Key schema details:

- **Bash**: `command` (required), `timeout` (max 600000ms), `description`, `run_in_background`, `dangerouslyDisableSandbox`
- **FileRead**: `file_path` (required, absolute), `offset`, `limit`, `pages` (PDF only, max 20). Output variants: text, image, notebook, pdf, parts
- **FileEdit**: `file_path`, `old_string`, `new_string` (must differ), `replace_all` (default false)
- **FileWrite**: `file_path` (required, absolute), `content` (required)
- **Glob**: `pattern` (required), `path` (optional). Max 100 results.
- **Grep**: `pattern` (required), `path`, `glob`, `output_mode` (files_with_matches/content/count), `-A/-B/-C/-n/-i`, `type`, `head_limit`, `offset`, `multiline`
- **Agent**: `description`, `prompt`, `subagent_type` (all required), `model` (sonnet/opus/haiku), `resume`, `run_in_background`, `max_turns`, `name`, `team_name`, `mode`, `isolation` (worktree)
- **AskUserQuestion**: 1-4 questions, each with 2-4 options, header (max 12 chars), multiSelect boolean
- **TodoWrite**: Array of `{content, status, activeForm}` items
- **WebFetch**: `url`, `prompt` (both required)
- **WebSearch**: `query` (required), `allowed_domains`, `blocked_domains`

### Tool Permission System

- Permission rules: `{toolName: string, ruleContent?: string}`
- Rule format: `ToolName(pattern)` — e.g., `Bash(npm run *)`, `Read(./.env)`, `WebFetch(domain:example.com)`
- Tool names must start with uppercase
- MCP tools: `mcp__<server>__<tool>` prefix
- Rule matching supports glob patterns via picomatch
- File-based rules match against absolute file paths
- Bash rules match against command patterns
- WebFetch rules match against `domain:` patterns

### Tool Deferred Loading / ToolSearch

- Some tools can be deferred (not loaded until needed)
- `ToolSearch` tool (line ~358006) enables fuzzy search for deferred tools
- Scoring system: exact name match (10pts), partial match (5pts), description match (2pts)
- MCP tools get slightly higher scores (12/6pts)

---

## 5. System Prompt Construction

### Assembly Function

`le()` function (line ~411524) combines:
1. Override system prompt (if set, replaces everything)
2. Agent definition's system prompt (if agent is active)
3. Custom system prompt (if provided)
4. Default system prompt (generated by `sG()`)
5. Append system prompt (additional instructions)

### System Prompt Components

The default system prompt includes:
- Tool definitions and descriptions
- CLAUDE.md content (merged from all locations)
- User context (from `C_()`)
- System context (from `jH()`)
- Permission rules
- Current date
- Model information
- Environment details (platform, shell, git status)

### Context Injection

- `systemReminders`: Extracted from `<system-reminder>` tags in user messages
- Regex pattern: `/^<system-reminder>\n?([\s\S]*?)\n?<\/system-reminder>$/`
- Context parts separated from system reminders in message processing

---

## 6. CLAUDE.md Loading

### Load Locations (in priority/merge order)

1. **User-level**: `~/.claude/CLAUDE.md`
2. **Walk from root to CWD**: Any `CLAUDE.md` found in parent directories
3. **Project-level**: `.claude/CLAUDE.md`
4. **Rules directory**: `.claude/rules/` (all files)

### Features

- `@path` imports for modular rules
- Exclusion patterns: glob patterns matched against absolute file paths using picomatch
- Exclusion only applies to User, Project, and Local types (Managed/policy files cannot be excluded)
- Example exclusions: `"/home/user/monorepo/CLAUDE.md"`, `"**/code/CLAUDE.md"`, `"**/some-dir/.claude/rules/**"`

---

## 7. Configuration / Settings

### Config Directory

- Default: `~/.claude/` (or `$CLAUDE_CONFIG_DIR`)
- Function: `OA()` returns the config directory path
- Teams config: `~/.claude/teams/`

### Settings Hierarchy (highest priority first)

1. **Managed** — `/etc/claude/` or system-level
2. **Remote settings** — fetched from `/api/claude_code/settings`
3. **Command-line flags** — temporary session overrides
4. **Local** — `.claude/settings.local.json`
5. **Project** — `.claude/settings.json`
6. **User** — `~/.claude/settings.json`

### Feature Flags

- Feature flag system via `qA()` function (e.g., `qA("tengu_oboe", false)`)
- Controls features like auto-memory, marble lantern (1M context), etc.

### Environment Variables

| Variable | Purpose |
|----------|---------|
| `CLAUDE_CONFIG_DIR` | Override config directory |
| `CLAUDE_CODE_ENVIRONMENT_KIND` | Environment type (`byoc`, `anthropic_cloud`) |
| `CLAUDE_CODE_DISABLE_AUTO_MEMORY` | Disable auto memory |
| `CLAUDE_CODE_REMOTE` | Remote mode flag |
| `CLAUDE_CODE_REMOTE_MEMORY_DIR` | Remote memory directory |
| `CLAUDE_CODE_CUSTOM_OAUTH_URL` | Custom OAuth endpoint |
| `CLAUDE_CODE_OAUTH_CLIENT_ID` | Custom OAuth client ID |
| `CLAUDE_CODE_USE_VERTEX` | Use Vertex AI backend |
| `CLAUDE_CODE_USE_FOUNDRY` | Use Foundry backend |
| `ANTHROPIC_BASE_URL` | Custom API base URL |
| `HTTPS_PROXY` | HTTP proxy |
| `DISABLE_COMPACT` | Disable compaction command |

---

## 8. Session Management

### Session Storage

- Sessions stored in `~/.claude/` directory hierarchy
- Session files read via `session file` references (line ~496048)
- Session ID: UUID format
- Supports `--session-id <uuid>` to specify a session ID

### Session Resume

- `-c, --continue`: Continue most recent conversation in current directory
- `-r, --resume [id]`: Resume by session ID or open interactive picker
- `--fork-session`: Create new session ID when resuming
- `--from-pr [value]`: Resume session linked to a PR
- `--no-session-persistence`: Disable session saving (print mode only)
- `--resume-session-at <message-id>`: Resume to specific message
- `--rewind-files <user-message-id>`: Restore files to state at message

### Session Persistence

- Sessions saved to disk automatically
- Can be disabled with `--no-session-persistence`
- Session file read telemetry: `tengu_session_file_read`

---

## 9. Context Compaction

### Compaction Function

`FbY` (line ~411650) handles compaction:

1. If no custom instructions: attempts `sW1()` (auto-compaction)
2. Otherwise: filters messages via `Dg()`, then calls `TZ6()` for full compaction
3. Clears caches and resets state after compaction
4. Returns `{type: "compact", compactionResult, displayText}`

### Summarization Prompts

Two summarization prompt generators (lines ~357706-357915):
- **Recent summary** (`yC4`): Summarizes recent messages only
- **Full summary** (`yC4` variant): Summarizes entire conversation

Both produce structured output:
```
<analysis>
[Thought process]
</analysis>
<summary>
1. Primary Request and Intent
2. Key Technical Concepts
3. Files and Code Sections
4. Errors and fixes
5. Problem Solving
6. All user messages
7. Pending Tasks
8. Current Work
9. Optional Next Step
</summary>
```

### Session Continuation

`ZQ6()` function (line ~357946) constructs continuation message:
- Includes summary from previous conversation
- Optional link to full transcript
- Optional preserved recent messages
- Instruction to continue without asking questions

### 1M Context Mode

- Opus and Sonnet have `[1m]` variants with 5x context multiplier
- Feature gate: `cc()` / `lc()` functions check `tengu_marble_lantern_disabled` flag
- Model aliases: `opus[1m]`, `sonnet[1m]`
- Billed as extra usage for subscription users

---

## 10. Models

### Supported Models

| Alias | Model ID | Notes |
|-------|----------|-------|
| opus | claude-opus-4-6 | Latest |
| sonnet | claude-sonnet-4-6 | Latest |
| haiku | claude-haiku-4-5 | |
| opus[1m] | claude-opus-4-6 (1M) | 5x context |
| sonnet[1m] | claude-sonnet-4-6 (1M) | 5x context |

### Model Display Names (from cli.js line ~69306)

- `claude-opus-4-6[1m]` → "Opus 4.6 (with 1M context)"
- `claude-opus-4-6` → "Opus 4.6"
- Plus older models: claude-opus-4-1, claude-opus-4-5, claude-sonnet-4-5, claude-sonnet-4, claude-3-7-sonnet, claude-3-5-sonnet, claude-3-5-haiku

### Model Capability Detection

- `q.includes("claude-opus-4") || q.includes("claude-sonnet-4")`: Extended thinking capable
- `q.includes("claude-haiku-4")`: Haiku family detection
- Separate region mappings for Vertex AI

---

## 11. Hooks System

### Hook Events

| Event | When |
|-------|------|
| `PreToolUse` | Before tool execution |
| `PostToolUse` | After successful tool execution |
| `PostToolUseFailure` | After failed tool execution |
| `UserPromptSubmit` | User sends a message |
| `SessionStart` | Session begins |
| `Stop` | Conversation ends |

### Hook Format

```json
{
  "PostToolUse": [{
    "matcher": {"tools": ["BashTool"]},
    "hooks": [{"type": "command", "command": "echo Done"}]
  }]
}
```

### Hook Types

- **Command hooks**: Run shell commands, use exit code/stdout
- **Prompt hooks**: Inject additional context
- **Agent hooks**: Spawn Claude-driven sub-processes

### Hook Dispatch

- Hook names follow `EventType:ToolName` format (e.g., `PreToolUse:Bash`, `PostToolUse:Read`)
- PreToolUse hooks can block tool execution (via `blockingError`)
- PostToolUse hooks can stop execution (via `stopReason`)

---

## 12. Skills System

### Skill Definition

Skills are markdown files with frontmatter defining:
- Trigger conditions (slash command name, auto-trigger patterns)
- Instructions/prompts
- Supporting files

### Skill Locations

- User-level: `~/.claude/skills/`
- Project-level: `.claude/skills/`

### Skills Beta Header

`skills-2025-10-02`

### Install Command

`/install` slash command for installing skills/plugins.

---

## 13. MCP (Model Context Protocol)

### Transport Modes

- **stdio**: Launch subprocess, communicate via stdin/stdout JSON-RPC
- **SSE**: Connect to HTTP server streaming JSON-RPC over SSE

### MCP Configuration

```json
{
  "mcpServers": {
    "server-name": {
      "command": "npx",
      "args": ["-y", "@some/mcp-server"],
      "env": {"API_KEY": "..."}
    }
  }
}
```

### MCP Config Files

- `.mcp.json` (project-level)
- `~/.mcp.json` (user-level)
- `--mcp-config <configs...>` CLI flag
- `--strict-mcp-config`: Only use servers from --mcp-config

### MCP Proxy

- Proxy URL: `https://mcp-proxy.anthropic.com`
- Proxy path: `/v1/mcp/{server_id}`
- OAuth for MCP servers: `/api/organizations/{org}/mcp/start-auth/{server}`

### MCP Tool Naming

- MCP tools registered as `mcp__<serverName>__<toolName>`
- Server name sanitized via `f_()` function
- Permission rules support MCP pattern matching

---

## 14. Slash Commands

### Registered Commands

| Command | Description | Aliases |
|---------|-------------|---------|
| `/clear` | Clear conversation history and free up context | `/reset`, `/new` |
| `/compact` | Clear history but keep summary in context | |
| `/config` | Configuration management | |
| `/context` | Show context usage breakdown | |
| `/cost` | Show token usage and cost | |
| `/doctor` | Diagnose issues | |
| `/memory` | Edit Claude memory files | |
| `/help` | Show help | |
| `/init` | Initialize CLAUDE.md for project | |
| `/login` | Log in | |
| `/logout` | Log out | |
| `/mcp` | Manage MCP servers | |
| `/review` | Review code/PR | |
| `/status` | Show status | |
| `/theme` | Change color theme | |
| `/vim` | Toggle vim mode | |
| `/permissions` | View/edit permission rules | |
| `/fast` | Toggle fast mode | |
| `/hooks` | View configured hooks | |
| `/agents` | Configure subagents | |
| `/model` | Switch between Claude models | |
| `/add-dir` | Add additional directories | |
| `/install` | Install skills/plugins | |

### Command Properties

- `type`: `"local"` for built-in commands
- `isEnabled`: Function returning boolean
- `isHidden`: Whether to show in help
- `supportsNonInteractive`: Whether usable in --print mode
- `argumentHint`: Hint for command arguments
- `load`: Lazy loader returning command module

---

## 15. CLI Flags

### Primary Flags

| Flag | Description |
|------|-------------|
| `[prompt]` | Initial prompt (positional argument) |
| `-p, --print` | Print response and exit (non-interactive) |
| `-c, --continue` | Continue most recent conversation |
| `-r, --resume [id]` | Resume by session ID or picker |
| `--model <model>` | Model for session (alias or full name) |
| `-d, --debug [filter]` | Debug mode with optional category filter |
| `--verbose` | Override verbose setting |
| `-h, --help` | Display help |

### Output Flags

| Flag | Description |
|------|-------------|
| `--output-format <fmt>` | `text` (default), `json`, `stream-json` |
| `--input-format <fmt>` | `text` (default), `stream-json` |
| `--json-schema <schema>` | JSON Schema for structured output |
| `--include-partial-messages` | Include partial chunks (stream-json) |
| `--replay-user-messages` | Re-emit user messages for ack |

### Session Flags

| Flag | Description |
|------|-------------|
| `--fork-session` | New session ID when resuming |
| `--from-pr [value]` | Resume session linked to PR |
| `--no-session-persistence` | Don't save sessions (print mode) |
| `--resume-session-at <id>` | Resume to specific message |
| `--rewind-files <id>` | Restore files at message |
| `--session-id <uuid>` | Specific session UUID |

### Permission Flags

| Flag | Description |
|------|-------------|
| `--dangerously-skip-permissions` | Bypass all permission checks |
| `--allow-dangerously-skip-permissions` | Enable bypass as option |
| `--permission-mode <mode>` | Permission mode for session |
| `--allowedTools, --allowed-tools <tools...>` | Allow specific tools |
| `--disallowedTools, --disallowed-tools <tools...>` | Deny specific tools |
| `--tools <tools...>` | Specify available tools (`""`, `"default"`, or names) |
| `--permission-prompt-tool <tool>` | MCP tool for permission prompts |

### Model Flags

| Flag | Description |
|------|-------------|
| `--effort <level>` | Effort level: low, medium, high, max |
| `--thinking <mode>` | Thinking: enabled, adaptive, disabled |
| `--max-thinking-tokens <n>` | Max thinking tokens (deprecated) |
| `--betas <betas...>` | Beta headers for API requests |
| `--fallback-model <model>` | Fallback on overload (print mode) |

### Agent/Tool Flags

| Flag | Description |
|------|-------------|
| `--agent <agent>` | Agent for session |
| `--max-turns <n>` | Max agentic turns (print mode) |
| `--max-budget-usd <amount>` | Max spend (print mode, must be > 0) |

### Config Flags

| Flag | Description |
|------|-------------|
| `--system-prompt <prompt>` | Custom system prompt |
| `--system-prompt-file <file>` | System prompt from file |
| `--append-system-prompt <prompt>` | Append to default system prompt |
| `--append-system-prompt-file <file>` | Append system prompt from file |
| `--settings <file-or-json>` | Additional settings file or JSON |
| `--add-dir <dirs...>` | Additional allowed directories |
| `--mcp-config <configs...>` | MCP server config files/strings |
| `--strict-mcp-config` | Only use --mcp-config servers |

### Other Flags

| Flag | Description |
|------|-------------|
| `--ide` | Auto-connect to IDE on startup |
| `--mcp-debug` | Deprecated: use --debug |
| `--debug-file <path>` | Write debug logs to file |
| `--init` | Run Setup hooks with init trigger |
| `--init-only` | Run setup hooks then exit |
| `--maintenance` | Run Setup hooks with maintenance trigger |
| `--enable-auth-status` | Auth status in SDK mode |

---

## 16. Agentic Loop

### Loop Structure

1. **Build messages**: Assemble system prompt, conversation history, user input
2. **Call API**: Send messages with tool definitions, stream response
3. **Process response**: Handle text output and tool_use blocks
4. **Execute tools**: Run each requested tool, collect results
5. **Append results**: Add assistant message and tool results to history
6. **Repeat**: If tools were used, loop; if `end_turn`, wait for user input

### Termination Conditions

- Stop reason: `end_turn` (model finished)
- Stop reason: `max_tokens` (context limit)
- User interruption (Ctrl+C)
- Max turns reached (--max-turns)
- Max budget exceeded (--max-budget-usd)
- Hook-triggered stop (PostToolUse with stopReason)

### Sub-Agent Spawning

- Agent/Task tool spawns isolated sub-agents
- Sub-agents can run in background (`run_in_background: true`)
- Isolation modes: default (shared working dir) or `worktree` (git worktree)
- Sub-agent types defined by `subagent_type` parameter
- Models can be overridden per sub-agent

---

## 17. Git Checkpoints

### Checkpoint System

- Automatic git snapshots before file modifications
- Stash or commit current state
- Allows reverting to pre-edit state
- Integrated with permission system

---

## 18. TUI Behavior

### Terminal UI Framework

- Built with React (Ink) for terminal rendering
- Uses `a96` (React/Ink state management)
- Tabbed interface with keyboard navigation (←/→ or Tab)

### Rendering

- Markdown rendering in terminal
- Syntax-highlighted code blocks
- Diff display for file edits (structured patches)
- Streaming text display (token by token)
- Progress indicators and spinners
- Color themes (configurable via `/theme`)

### Input

- Multi-line input editing
- Vim mode toggle (`/vim`)
- Slash command input and dispatch
- Ctrl+C for interruption
- Ctrl+B for backgrounding commands
- Ctrl+O for transcript toggle

### Keybindings

- Tab/arrow keys for tab navigation
- Custom keybindings via `~/.claude/keybindings.json`

---

## 19. Telemetry

### Event Logging

- Batch endpoint: `/api/event_logging/batch`
- Max batch size: 200 events
- Batch delay: 100ms
- Timeout: 10000ms
- Backoff: 500ms base, 30000ms max
- Events prefixed with `tengu_`

### OpenTelemetry Metrics

- Metrics endpoint: `/api/claude_code/metrics`
- Timeout: 5000ms
- OpenTelemetry SDK integration for tracing and metrics

### Notable Telemetry Events

- `tengu_oauth_automatic_redirect`: OAuth redirect
- `tengu_grove_oauth_401_received`: Token refresh triggered
- `tengu_oauth_profile_fetch_success`: Profile fetch success
- `tengu_session_file_read`: Session file read
- `tengu_agent_memory_loaded`: Agent memory loaded

---

## 20. Permission System

### Permission Modes

Available via `--permission-mode`:
- Modes include options like `acceptEdits`, `bypassPermissions`, `default`, `dontAsk`, `plan`

### Rule Format

```
ToolName                    # All invocations of tool
ToolName(pattern)           # Pattern-matched invocations
Bash(npm run *)             # Bash with command pattern
Read(./.env)                # File read with path pattern
WebFetch(domain:example.com)  # Web fetch with domain
mcp__server__tool           # MCP tool
```

### Rule Validation

- Tool name must start with uppercase (or be `mcp` prefix)
- MCP rules parsed for `serverName` and `toolName` components
- Pattern matching uses glob syntax
- File path patterns: `*.ts`, `src/**`, `**/*.test.ts`
- Bash patterns: command prefix matching
- WebFetch patterns: `domain:` prefix for domain restrictions

### Permission Prompt

- Interactive UI for user approval
- Can be replaced with MCP tool via `--permission-prompt-tool`

---

## 21. Auto-Memory System

### Memory Configuration

- Enabled via `tengu_oboe` feature flag
- Can be disabled: `CLAUDE_CODE_DISABLE_AUTO_MEMORY`
- Remote memory dir: `CLAUDE_CODE_REMOTE_MEMORY_DIR`
- Config setting: `autoMemoryEnabled`

### Memory Storage

- Default location: `~/.claude/` (same as config dir, via `OA()`)
- Remote override via environment variable
- Memory files (MEMORY.md) editable via `/memory` command
- Session notes: maintained per-session with token budget (`PS4` constant)

---

## 22. Connectivity & Health

### Health Check

- `/api/hello` endpoint for connectivity verification
- 5000ms timeout
- `Cache-Control: no-cache` header
- Skipped for Vertex/Foundry backends

### Doctor Command

- `/doctor` slash command for diagnosing issues
- Checks auth, connectivity, configuration

---

## Summary Statistics

| Category | Count |
|----------|-------|
| Built-in tools | 24 |
| Slash commands | 24 |
| CLI flags | ~40 |
| API endpoints (non-messages) | 16+ |
| Hook events | 6 |
| Beta headers | 15 |
| Supported models | 10+ |
| OAuth scopes | 6 |
