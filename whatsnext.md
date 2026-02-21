# Phase 3: Session and Config — Integration Guide

Phase 3 adds session persistence, settings hierarchy, enhanced CLAUDE.md loading, permission rules, and context compaction.

**The JavaScript source in `claude-code-source/cli.js` is the ground truth.** Every file format, loading order, merge rule, and behavioral detail must match what that code does. When this document and the JS disagree, the JS wins. Search `cli.js` to confirm every assumption before writing Go code.

---

## What exists today

### Key types you will consume

**`api.Message`** (`internal/api/types.go`) — the message format used everywhere:
```go
type Message struct {
    Role    string          `json:"role"`
    Content json.RawMessage `json:"content"` // string or []ContentBlock
}
```
Messages are created via `api.NewTextMessage(role, text)` and `api.NewBlockMessage(role, blocks)`. The `Content` field is raw JSON — it can be a JSON string (`"hello"`) or a JSON array of `ContentBlock` objects.

**`api.ContentBlock`** — union type for text, tool_use, tool_result. Tool results carry `ToolUseID`, `Content` (raw JSON), and `IsError`.

**`api.SystemBlock`** — system prompt blocks with optional `CacheControl`:
```go
type SystemBlock struct {
    Type         string        `json:"type"`
    Text         string        `json:"text,omitempty"`
    CacheControl *CacheControl `json:"cache_control,omitempty"`
}
```

**`api.Usage`** — token counts from every response:
```go
type Usage struct {
    InputTokens              int  `json:"input_tokens"`
    OutputTokens             int  `json:"output_tokens"`
    CacheCreationInputTokens *int `json:"cache_creation_input_tokens,omitempty"`
    CacheReadInputTokens     *int `json:"cache_read_input_tokens,omitempty"`
}
```

**`api.CreateMessageRequest`** — the full request struct, including `Model`, `MaxTokens`, `Messages`, `System`, `Tools`, `Stream`, `Metadata`, `Temp`, `TopP`, `TopK`.

### Key interfaces you will extend

**`conversation.ToolExecutor`** (`internal/conversation/loop.go`):
```go
type ToolExecutor interface {
    Execute(ctx context.Context, name string, input []byte) (string, error)
    HasTool(name string) bool
}
```
The `tools.Registry` implements this. Your settings-based permission rules need to integrate with the registry's existing `PermissionHandler` interface.

**`tools.PermissionHandler`** (`internal/tools/registry.go`):
```go
type PermissionHandler interface {
    RequestPermission(ctx context.Context, toolName string, input json.RawMessage) (bool, error)
}
```
Currently there are two implementations: `TerminalPermissionHandler` (prompts on stdin) and `AlwaysAllowPermissionHandler`. Phase 3 needs a new implementation that checks permission rules from settings *before* falling through to the terminal prompt.

**`tools.Tool`** — each tool declares `RequiresPermission(input json.RawMessage) bool`. The registry calls this first; if it returns true, it calls the `PermissionHandler`. Your rule-based handler is the `PermissionHandler`, not a replacement for `RequiresPermission`.

### Existing code you will modify

**`conversation.History`** (`internal/conversation/history.go`) — currently an in-memory `[]api.Message` with no serialization. You need to add `MarshalJSON`/`UnmarshalJSON` (or equivalent export/import methods) so sessions can be saved and loaded.

**`conversation.Loop`** (`internal/conversation/loop.go`) — currently creates a fresh `History` internally. It needs to accept an existing `History` for session resume, and it needs to trigger compaction when token counts get high.

**`conversation.BuildSystemPrompt`** (`internal/conversation/system_prompt.go`) — currently a standalone function that loads CLAUDE.md from a fixed set of paths. Phase 3 upgrades this to support `@path` imports, `.claude/rules/` directory, and injection of permission context. The function signature will likely need to accept a settings/config struct instead of just `cwd`.

**`cmd/claude/main.go`** — currently ignores `-c` and `-r` flags, hardcodes `BuildSystemPrompt(cwd)`, and creates the `TerminalPermissionHandler` directly. Phase 3 wires in session loading, settings loading, and the rule-based permission handler.

---

## 1. Session persistence (`internal/session/`)

### What it does

Sessions store the full conversation history so users can resume with `claude -c` (most recent) or `claude -r <session-id>` (by ID).

### What the JS does (ground truth)

Search `cli.js` for `session`, `conversation`, `save`, `resume`, `compact`, `checkpoint`. Key things to confirm:
- Where sessions are stored (likely `~/.claude/sessions/` or `~/.claude/projects/<hash>/sessions/`)
- The JSON format of a saved session (message array, metadata like model, cwd, timestamps)
- How session IDs are generated
- How "most recent" is determined (by file mtime? by a metadata field?)
- Whether tool results are stored verbatim or summarized
- Whether sessions are saved after every turn or only on exit

### Interface with existing code

**Reading history out of the loop:**

`conversation.History` currently has `Messages() []api.Message` which returns the internal slice. For session save, you need to serialize this. Add methods to `History`:

```go
// SetMessages replaces the message list (for session resume).
func (h *History) SetMessages(msgs []api.Message)

// or, add a constructor:
func NewHistoryFrom(msgs []api.Message) *History
```

Then modify `conversation.NewLoop` (or add a `LoopConfig` field) so the loop can start with a pre-populated history:

```go
type LoopConfig struct {
    // ... existing fields ...
    History *History // if non-nil, resume from this history
}
```

In `NewLoop`, if `cfg.History != nil`, use it instead of calling `NewHistory()`.

**Saving after each turn:**

The loop's `run()` method is the natural save point. After `l.history.AddAssistantResponse(resp.Content)` and after `l.history.AddToolResults(toolResults)`, the session should be persisted. Rather than having the loop call the session store directly, consider a callback or event interface:

```go
type LoopConfig struct {
    // ...
    OnTurnComplete func(history *History) // called after each API round-trip
}
```

The session package calls `session.Save()` inside this callback.

**Session store interface:**

```go
// internal/session/session.go
type Session struct {
    ID        string        `json:"id"`
    Model     string        `json:"model"`
    CWD       string        `json:"cwd"`
    Messages  []api.Message `json:"messages"`
    CreatedAt time.Time     `json:"created_at"`
    UpdatedAt time.Time     `json:"updated_at"`
    // ... whatever else cli.js stores
}

type Store struct {
    dir string // e.g. ~/.claude/projects/<hash>/sessions/
}

func (s *Store) Save(session *Session) error
func (s *Store) Load(id string) (*Session, error)
func (s *Store) MostRecent() (*Session, error)
func (s *Store) List() ([]*Session, error)
```

**Verify the exact JSON schema by inspecting saved sessions from the official CLI**, not by guessing. Run the official CLI, have a conversation, then look at what it wrote to disk. The Go implementation must produce and consume the same format.

### Wiring in main.go

```go
// Before creating the loop:
var history *conversation.History
if *continueFlag {
    sess, err := sessionStore.MostRecent()
    // ... load sess.Messages into history
}
if *resumeFlag != "" {
    sess, err := sessionStore.Load(*resumeFlag)
    // ...
}

loop := conversation.NewLoop(conversation.LoopConfig{
    // ...
    History: history,
})
```

---

## 2. Settings hierarchy (`internal/config/`)

### What it does

Settings come from five sources, highest priority first:
1. **Managed** — `/etc/claude/settings.json` (IT-deployed)
2. **CLI flags** — command-line overrides
3. **Local** — `.claude/settings.local.json` (per-project, gitignored)
4. **Project** — `.claude/settings.json` (per-project, committed)
5. **User** — `~/.claude/settings.json` (global)

### What the JS does (ground truth)

Search `cli.js` for `settings.json`, `settings.local`, `managed`, `/etc/claude`. Confirm:
- Exact file paths at each level
- Merge strategy (deep merge? shallow merge? per-key override?)
- What keys exist (`permissions`, `model`, `env`, `hooks`, `sandbox`, etc.)
- How `env` values are injected (into the process env? into Bash tool env?)
- Whether any keys are additive across levels (e.g., do permission rules merge or replace?)

### Settings struct

```go
// internal/config/settings.go
type Settings struct {
    Permissions []PermissionRule `json:"permissions,omitempty"`
    Model       string           `json:"model,omitempty"`
    Env         map[string]string `json:"env,omitempty"`
    Hooks       map[string]Hook  `json:"hooks,omitempty"` // Phase 7, but parse now
    Sandbox     *SandboxConfig   `json:"sandbox,omitempty"`
    // ... check cli.js for the complete set of keys
}
```

### Interface with existing code

The merged settings affect several existing components:

- **Model selection** in `main.go` — currently hardcoded fallback to `api.ModelClaude4Sonnet`. Settings should provide the default; CLI flag overrides it.
- **Permission rules** — feed into the new rule-based `PermissionHandler` (see section 4 below).
- **System prompt** — `BuildSystemPrompt` may need settings context (e.g., to inject permission descriptions).
- **Environment variables** — `settings.Env` should be applied to the Bash tool's execution environment. The `BashTool` currently uses `cmd.Dir` but doesn't set `cmd.Env`. You'll need to extend `NewBashTool` to accept env vars, or add a method.

### Loading and merging

```go
func LoadSettings(cwd string) (*Settings, error)
```

This function reads all five levels and merges them. The merge semantics must match the JS exactly. Pay particular attention to whether `permissions` arrays are concatenated or replaced.

---

## 3. CLAUDE.md loading enhancements (`internal/conversation/system_prompt.go`)

### What exists

`BuildSystemPrompt(cwd)` loads CLAUDE.md from three location groups:
1. `~/.claude/CLAUDE.md`
2. Every directory from root to CWD
3. `.claude/CLAUDE.md`

It concatenates sections with `---` separators and injects them into one `SystemBlock`.

### What Phase 3 adds

**`@path` imports** — CLAUDE.md files can contain `@path/to/other/file` directives that include content from other files. Search `cli.js` for `@` handling in CLAUDE.md loading to find:
- The exact syntax (is it `@path` on its own line? can it be inline?)
- Whether paths are relative to the CLAUDE.md file or to the CWD
- Whether it's recursive (can an imported file itself contain `@path`?)
- Any depth limit or cycle detection

**`.claude/rules/` directory** — all `.md` files in `.claude/rules/` are loaded as additional instructions. Search `cli.js` for `rules` to confirm:
- Load order within the directory (alphabetical? glob order?)
- Whether subdirectories are traversed
- Whether this is at project level only or also at user level (`~/.claude/rules/`)

### Interface with existing code

The current `BuildSystemPrompt(cwd string) []api.SystemBlock` signature may need to change. If settings influence the system prompt (e.g., injecting permission rule summaries), the signature should become:

```go
func BuildSystemPrompt(cwd string, settings *config.Settings) []api.SystemBlock
```

Or, if you prefer a builder pattern:

```go
type SystemPromptBuilder struct {
    CWD      string
    Settings *config.Settings
}
func (b *SystemPromptBuilder) Build() []api.SystemBlock
```

Either way, update the call site in `main.go` accordingly.

The internal `loadClaudeMD(cwd)` function needs to be rewritten to handle `@path` resolution and `.claude/rules/` loading. Keep it in `system_prompt.go` or move it to a dedicated `internal/config/claudemd.go` — either works, but the CLAUDE.md spec says `internal/config/claudemd.go`.

---

## 4. Permission rules (`internal/config/permissions.go`)

### What it does

Settings files contain permission rules that pre-authorize or deny specific tool calls, eliminating the need for interactive prompts:

```json
{
  "permissions": [
    {"tool": "Bash", "pattern": "npm run *", "action": "allow"},
    {"tool": "FileRead", "pattern": ".env", "action": "deny"},
    {"tool": "Bash", "action": "ask"}
  ]
}
```

### What the JS does (ground truth)

Search `cli.js` for `permission`, `allow`, `deny`, `ask`, `glob`, `pattern`. Confirm:
- The exact JSON schema for permission rules
- How patterns work for different tools (glob for Bash commands? file path glob for Read/Edit/Write? domain matching for WebFetch?)
- Rule evaluation order (first match? most specific? all levels merged then evaluated?)
- Whether `ask` is the implicit default for unmatched tool calls
- What `Bash(npm run *)` notation means exactly and how it's parsed

### Interface with existing code

Create a new `PermissionHandler` implementation that wraps the existing terminal handler:

```go
// internal/config/permissions.go (or internal/tools/rulepermission.go)
type RuleBasedPermissionHandler struct {
    rules    []PermissionRule
    fallback tools.PermissionHandler // the TerminalPermissionHandler
}

func (h *RuleBasedPermissionHandler) RequestPermission(
    ctx context.Context, toolName string, input json.RawMessage,
) (bool, error) {
    action := h.matchRule(toolName, input)
    switch action {
    case "allow":
        return true, nil
    case "deny":
        return false, nil
    default: // "ask" or no match
        return h.fallback.RequestPermission(ctx, toolName, input)
    }
}
```

In `main.go`, replace the direct `TerminalPermissionHandler` creation:

```go
// Before (Phase 2):
permHandler = tools.NewTerminalPermissionHandler()

// After (Phase 3):
terminalHandler := tools.NewTerminalPermissionHandler()
permHandler = config.NewRuleBasedPermissionHandler(settings.Permissions, terminalHandler)
```

The `tools.Registry` doesn't change — it still calls `PermissionHandler.RequestPermission()`. The rule evaluation is hidden inside the handler.

### Pattern matching

You need a function that takes a tool name, the tool's JSON input, and a rule, and decides if the rule matches. The matching logic varies by tool:

- **Bash** — match the `command` field against the pattern (glob-style)
- **FileRead/FileEdit/FileWrite** — match `file_path` against the pattern
- **WebFetch** — match the URL's domain against `domain:example.com` patterns
- **Glob/Grep** — match the `path` field

The exact matching semantics must come from `cli.js`. Don't invent your own.

---

## 5. Context compaction (`internal/conversation/compaction.go`)

### What it does

When the conversation history approaches the model's context window limit, older messages are summarized by calling the API, and the detailed messages are replaced with the summary. This keeps the conversation going without hitting token limits.

### What the JS does (ground truth)

Search `cli.js` for `compact`, `summarize`, `context`, `checkpoint`, `token`, `limit`. Confirm:
- The trigger condition (what token threshold triggers compaction? is it based on input_tokens from the Usage?)
- What gets summarized (all messages? only messages before a certain point? only non-recent messages?)
- The prompt sent to the API for summarization
- How the summary replaces messages (single summary message? preserves the most recent N messages?)
- Whether compaction happens automatically or only via `/compact` command
- What context window sizes the JS uses for different models

### Interface with existing code

Compaction needs to read the history and modify it. Add to `History`:

```go
// ReplaceRange replaces messages[start:end] with replacement.
func (h *History) ReplaceRange(start, end int, replacement []api.Message)
```

The compaction logic also needs to call the API to generate the summary. It should accept the `api.Client` directly:

```go
// internal/conversation/compaction.go
type Compactor struct {
    Client           *api.Client
    MaxInputTokens   int // trigger threshold
    PreserveRecent   int // number of recent messages to keep
}

func (c *Compactor) ShouldCompact(usage api.Usage) bool
func (c *Compactor) Compact(ctx context.Context, history *History) error
```

Wire this into the loop. After each API call in `run()`, check if compaction is needed:

```go
resp, err := l.client.CreateMessageStream(ctx, req, l.handler)
// ...
if l.compactor != nil && l.compactor.ShouldCompact(resp.Usage) {
    if err := l.compactor.Compact(ctx, l.history); err != nil {
        // log warning but don't fail the loop
    }
}
```

Add `Compactor` to `LoopConfig`:

```go
type LoopConfig struct {
    // ... existing fields ...
    Compactor *Compactor
}
```

Also wire the `/compact` slash command in `main.go` to trigger manual compaction.

---

## Summary of changes by file

| File | Change |
|------|--------|
| `internal/session/session.go` | **New.** Session struct, Store with Save/Load/MostRecent/List. |
| `internal/session/persistence.go` | **New.** File I/O for sessions. Format must match official CLI. |
| `internal/config/settings.go` | **New.** Settings struct, LoadSettings with 5-level merge. |
| `internal/config/claudemd.go` | **New.** Enhanced CLAUDE.md loader with `@path` imports and `.claude/rules/`. |
| `internal/config/permissions.go` | **New.** RuleBasedPermissionHandler, pattern matching. |
| `internal/conversation/history.go` | **Modify.** Add SetMessages/export, serialization support. |
| `internal/conversation/loop.go` | **Modify.** Accept existing History in LoopConfig, add OnTurnComplete callback, add Compactor. |
| `internal/conversation/compaction.go` | **New.** Compactor with ShouldCompact/Compact. |
| `internal/conversation/system_prompt.go` | **Modify.** Accept Settings, use enhanced CLAUDE.md loader. |
| `cmd/claude/main.go` | **Modify.** Wire session load/save, settings loading, rule-based permissions, `/compact` command, activate `-c`/`-r` flags. |
| `go.mod` | **Modify.** Add any new dependencies (e.g., glob matching library for permission patterns). |

---

## How to verify

For every feature, compare against the official CLI:

1. **Session format** — Run the official CLI (`npx @anthropic-ai/claude-code`), have a multi-turn conversation, then inspect the session files on disk. Your Go code must read those files and produce files the official CLI can read.

2. **Settings merge** — Create settings files at all five levels with conflicting values. Run the official CLI and observe which value wins. Your merge logic must produce the same result.

3. **CLAUDE.md loading** — Create CLAUDE.md files with `@path` imports and a `.claude/rules/` directory. Run the official CLI and inspect the system prompt (you can ask the model "what are your instructions?" to see what was injected). Your output must match.

4. **Permission rules** — Configure rules in settings, then trigger tool calls that should be auto-allowed, auto-denied, or prompted. Compare behavior with the official CLI.

5. **Context compaction** — Fill a conversation to near the context limit and observe when/how the official CLI compacts. Your trigger threshold and summary behavior must match.

When in doubt, read `cli.js`. It is 587K lines (prettified) but it is searchable, and it is the only source of truth.
