# Phase 7: Hooks, Skills, and Advanced Features — Integration Guide

Phase 7 adds hooks (lifecycle event callbacks), skills (slash-command–driven
prompt bundles), and several advanced CLI features (output formats, pipe
support). Unlike Phase 6, which was purely additive, Phase 7 touches the
agentic loop, the system prompt, the TUI model, the permission system, and
`main.go`.

---

## What exists today and where Phase 7 hooks in

### 1. The agentic loop (`internal/conversation/loop.go`)

The loop runs in `Loop.run()` (lines 98–171). The flow is:

```
for {
    1. Build request with history, system, tools   (line 100–104)
    2. Call API with streaming handler              (line 106)
    3. Add assistant response to history            (line 116)
    4. Check auto-compaction                        (line 119–124)
    5. If stop_reason != tool_use → done            (line 127–131)
    6. For each tool_use block:
       a. Check tool exists                         (line 140)
       b. Execute tool via toolExec.Execute()       (line 147)
       c. Collect tool result                       (line 154–158)
    7. Add tool results to history                  (line 167)
    8. Notify turn complete                         (line 168)
    9. Loop back to step 1
}
```

**Hook firing points:**

| Hook event | Where in loop.go | Context available |
|------------|------------------|-------------------|
| `PreToolUse` | Before line 147 (`toolExec.Execute`) | `block.Name`, `block.Input` |
| `PostToolUse` | After line 147, around line 159 | tool name, input, output, isError |
| `Stop` | At line 129 (stop_reason != tool_use) | assistant response content |

Phase 7 needs to add a `HookRunner` field to the `Loop` struct and call it
at these points. The runner must be optional — `nil` means no hooks.

**Key constraint:** `UserPromptSubmit` fires before `l.history.AddUserMessage`
(line 86). The hook can modify or reject the message. This means
`Loop.SendMessage` needs a hook callout before line 86, or the hook fires
in the caller (TUI model or main.go). The cleanest approach is to fire it
in `SendMessage` itself:

```go
func (l *Loop) SendMessage(ctx context.Context, userMessage string) error {
    // Phase 7: UserPromptSubmit hook.
    if l.hooks != nil {
        result, err := l.hooks.RunUserPromptSubmit(ctx, userMessage)
        if err != nil { return err }
        if result.Block { return nil }  // hook rejected the message
        userMessage = result.Message    // hook may modify the message
    }
    l.history.AddUserMessage(userMessage)
    return l.run(ctx)
}
```

**`SessionStart` hook** does not fire in the loop — it fires once in
`main.go` after the loop is created (or in `app.Run` for the TUI path).

### 2. LoopConfig (`internal/conversation/loop.go`, lines 32–41)

```go
type LoopConfig struct {
    Client         *api.Client
    System         []api.SystemBlock
    Tools          []api.ToolDefinition
    ToolExec       ToolExecutor
    Handler        api.StreamHandler
    History        *History
    Compactor      *Compactor
    OnTurnComplete func(history *History)
}
```

Add a `Hooks` field:

```go
    Hooks          HookRunner           // Phase 7: nil = no hooks
```

This must also propagate to sub-agents. The `AgentTool` (`internal/tools/agent.go`, line 142–149) builds a `LoopConfig` for each sub-agent. Phase 7 must pass the hooks through:

```go
loopCfg := conversation.LoopConfig{
    Client:   t.client,
    System:   t.system,
    Tools:    t.tools,
    ToolExec: t.toolExec,
    Handler:  handler,
    History:  history,
    Hooks:    t.hooks,  // Phase 7: propagate hooks to sub-agents
}
```

The `AgentTool` struct needs a `hooks` field set from `main.go`.

### 3. The system prompt (`internal/conversation/system_prompt.go`)

`BuildSystemPrompt` (lines 16–49) assembles the prompt in this order:

1. Core identity (lines 20–21)
2. Environment info (lines 24–27)
3. CLAUDE.md content (lines 30–33)
4. Permission rules (lines 36–41)

**Skills inject after CLAUDE.md, before permission rules.** Add a new
parameter for skill content:

```go
func BuildSystemPrompt(cwd string, settings *config.Settings, skillContent string) []api.SystemBlock {
    // ... existing code ...

    // Phase 7: Inject skill instructions.
    if skillContent != "" {
        parts = append(parts, "\n# Active Skills\n\n"+skillContent)
    }

    // Permission rules (existing code at line 36)
    // ...
}
```

The caller in `main.go` (line 121) changes from:

```go
system := conversation.BuildSystemPrompt(cwd, settings)
```

to:

```go
skillContent := skills.LoadActiveSkills(cwd)
system := conversation.BuildSystemPrompt(cwd, settings, skillContent)
```

### 4. Settings and hooks config (`internal/config/settings.go`)

The `Settings` struct already has a `Hooks` field (line 22):

```go
Hooks json.RawMessage `json:"hooks,omitempty"` // parsed later in Phase 7
```

And `mergeSettings` already handles merge (lines 112–116):

```go
result.Hooks = base.Hooks
if overlay.Hooks != nil {
    result.Hooks = overlay.Hooks
}
```

**Phase 7 must define the hook config schema and parse it.** Expected format:

```json
{
  "hooks": {
    "PreToolUse": [
      { "type": "command", "command": "./scripts/pre-tool.sh" }
    ],
    "PostToolUse": [
      { "type": "command", "command": "echo done" }
    ],
    "UserPromptSubmit": [
      { "type": "prompt", "prompt": "Check for sensitive data" }
    ],
    "SessionStart": [
      { "type": "command", "command": "./scripts/setup.sh" }
    ],
    "Stop": [
      { "type": "command", "command": "./scripts/cleanup.sh" }
    ]
  }
}
```

Create an `internal/hooks/` package with:

```go
type HookConfig struct {
    PreToolUse        []HookDef `json:"PreToolUse,omitempty"`
    PostToolUse       []HookDef `json:"PostToolUse,omitempty"`
    UserPromptSubmit  []HookDef `json:"UserPromptSubmit,omitempty"`
    SessionStart      []HookDef `json:"SessionStart,omitempty"`
    PermissionRequest []HookDef `json:"PermissionRequest,omitempty"`
    Stop              []HookDef `json:"Stop,omitempty"`
}

type HookDef struct {
    Type    string `json:"type"`    // "command", "prompt", "agent"
    Command string `json:"command,omitempty"`
    Prompt  string `json:"prompt,omitempty"`
}
```

Parse from `settings.Hooks` in `main.go` after loading settings:

```go
var hookConfig hooks.HookConfig
if settings.Hooks != nil {
    json.Unmarshal(settings.Hooks, &hookConfig)
}
hookRunner := hooks.NewRunner(hookConfig)
```

### 5. CLAUDE.md loading pattern (`internal/config/claudemd.go`)

Skills follow the same multi-location discovery pattern as CLAUDE.md:

| Location | Purpose |
|----------|---------|
| `~/.claude/skills/` | User-level skills (all projects) |
| `.claude/skills/` | Project-level skills |

Each skill is a markdown file with YAML frontmatter:

```markdown
---
name: commit
description: Create a git commit
trigger: /commit
---

# Commit Skill

Instructions for creating commits...
```

The `LoadClaudeMD` function (lines 17–51) shows the pattern to follow:
directory listing, file reading, content assembly. Skills add frontmatter
parsing (use `strings.SplitN(content, "---", 3)` to extract it) and
slash-command registration.

### 6. Permission system (`internal/config/permissions.go`)

The `RuleBasedPermissionHandler.RequestPermission` method (lines 36–48)
is where `PermissionRequest` hooks fire:

```go
func (h *RuleBasedPermissionHandler) RequestPermission(
    ctx context.Context, toolName string, input json.RawMessage,
) (bool, error) {
    // Phase 7: Fire PermissionRequest hook here.
    // Hook can observe or override the decision.

    action := h.matchRule(toolName, input)
    switch action {
    case "allow":
        return true, nil
    case "deny":
        return false, nil
    default:
        return h.fallback.RequestPermission(ctx, toolName, input)
    }
}
```

The cleanest approach: wrap the `RuleBasedPermissionHandler` with a
`HookAwarePermissionHandler` that calls hooks before/after the inner handler.
This avoids modifying `permissions.go` directly:

```go
type HookAwarePermissionHandler struct {
    inner   config.PermissionHandler
    hooks   *hooks.Runner
}

func (h *HookAwarePermissionHandler) RequestPermission(
    ctx context.Context, toolName string, input json.RawMessage,
) (bool, error) {
    if h.hooks != nil {
        h.hooks.RunPermissionRequest(ctx, toolName, input)
    }
    return h.inner.RequestPermission(ctx, toolName, input)
}
```

### 7. Permission handlers (`internal/tools/permission.go`)

Two existing handlers:

- `TerminalPermissionHandler` (line 13) — prompts via stdin (print mode)
- `AlwaysAllowPermissionHandler` (line 84) — auto-approves (scripting)

Phase 7 does not need to modify these. The hook wrapping happens at a
higher level (see section 6).

### 8. Stream handlers (`internal/tui/stream.go` and `internal/conversation/loop.go`)

The `api.StreamHandler` interface (defined in `internal/api/streaming.go`, lines 77–86):

```go
type StreamHandler interface {
    OnMessageStart(msg MessageResponse)
    OnContentBlockStart(index int, block ContentBlock)
    OnTextDelta(index int, text string)
    OnInputJSONDelta(index int, partialJSON string)
    OnContentBlockStop(index int)
    OnMessageDelta(delta MessageDeltaBody, usage *Usage)
    OnMessageStop()
    OnError(err error)
}
```

Three existing implementations:

| Handler | Location | Purpose |
|---------|----------|---------|
| `PrintStreamHandler` | `loop.go:180` | Plain text to stdout |
| `ToolAwareStreamHandler` | `loop.go:207` | Text + tool summaries to stdout |
| `TUIStreamHandler` | `tui/stream.go:14` | Events → Bubble Tea messages |

**Phase 7 adds two new handlers** for `--output-format`:

| Handler | Format | Behavior |
|---------|--------|----------|
| `JSONStreamHandler` | `json` | Collect entire response, emit one JSON object at the end |
| `StreamJSONStreamHandler` | `stream-json` | Emit one JSON line per streaming event as it arrives |

These are selected in `main.go` based on the `--output-format` flag:

```go
outputFormat := flag.String("output-format", "text", "Output format: text, json, stream-json")
// ...
switch *outputFormat {
case "json":
    loop.SetHandler(NewJSONStreamHandler(os.Stdout))
case "stream-json":
    loop.SetHandler(NewStreamJSONStreamHandler(os.Stdout))
default:
    // existing behavior (ToolAwareStreamHandler or TUIStreamHandler)
}
```

### 9. TUI model (`internal/tui/model.go`)

The `handleSubmit` method (lines 291–342) processes user input:

```go
func (m model) handleSubmit(text string) (tea.Model, tea.Cmd) {
    // Echo to scrollback (line 295)
    // Check slash commands (line 299–328)
    // Otherwise: send to loop (line 331–341)
}
```

**`UserPromptSubmit` hook in TUI mode:** The hook needs to fire after the
user presses Enter but before the message is sent to the loop. If the loop
owns the hook (recommended — see section 1), the TUI doesn't need to change.
The TUI calls `m.loop.SendMessage(ctx, text)` at line 337, and the hook
fires inside `SendMessage`.

**Skill slash commands:** Skills register slash commands in the `slashRegistry`.
The model already has `slashReg *slashRegistry` (line 42). Phase 7 needs to:

1. Load skills at startup
2. For each skill with a `trigger: /name` frontmatter, register a `SlashCommand`
3. Pass skill content to the slash registry or the model

The cleanest approach: `newSlashRegistry` could accept a list of skill
definitions, or the model could receive them and register after creation.
Since `newSlashRegistry` is called in `newModel` (line 85), skills can be
passed via the `newModel` parameters or as a separate `RegisterSkillCommands`
method on the model.

### 10. main.go (`cmd/claude/main.go`)

Phase 7 touches `main.go` in several places. Here is the execution flow
with hook/skill callout points marked:

```
Line  26: Parse CLI flags
          ← Add: --output-format, --pipe (stdin support)
Line  90: Load settings
          ← After: Parse hookConfig from settings.Hooks
Line 121: Build system prompt
          ← Change: Pass skill content to BuildSystemPrompt
Line 123: Set up permission handler
          ← After: Wrap with HookAwarePermissionHandler
Line 142: Create tool registry and register tools
Line 192: Create AgentTool
          ← Change: Pass hookRunner to AgentTool
Line 244: Create Loop
          ← Change: Pass hookRunner in LoopConfig
          ← After: Fire SessionStart hook
Line 270: Print mode branch
          ← Change: Handle --output-format (json, stream-json)
          ← Add: Pipe/stdin support (read prompt from stdin if !isatty)
Line 282: TUI mode branch
          ← Change: Pass skills to TUI for slash command registration
```

### 11. AgentTool (`internal/tools/agent.go`)

The `AgentTool` struct (lines 39–49):

```go
type AgentTool struct {
    client   *api.Client
    system   []api.SystemBlock
    tools    []api.ToolDefinition
    toolExec conversation.ToolExecutor
    bgStore  *BackgroundTaskStore
    mu       sync.Mutex
    agents   map[string]*agentState
    nextID   int
}
```

**Add a `hooks` field** so sub-agents inherit the hook runner:

```go
    hooks    conversation.HookRunner  // Phase 7
```

`NewAgentTool` (line 52) gains a `hooks` parameter from `main.go`.
`Execute` (line 142–149) passes it to the sub-agent's `LoopConfig`.

---

## New packages and files

### `internal/hooks/`

| File | Purpose |
|------|---------|
| `types.go` | `HookConfig`, `HookDef`, `HookResult`, event type constants |
| `runner.go` | `Runner` — executes hooks: runs shell commands, injects prompts, spawns agents |
| `runner_test.go` | Tests for hook execution |

The `Runner` implements a `HookRunner` interface that the loop calls:

```go
// Defined in internal/conversation/ to avoid import cycles.
type HookRunner interface {
    RunPreToolUse(ctx context.Context, toolName string, input json.RawMessage) error
    RunPostToolUse(ctx context.Context, toolName string, input json.RawMessage, output string, isError bool) error
    RunUserPromptSubmit(ctx context.Context, message string) (HookSubmitResult, error)
    RunSessionStart(ctx context.Context) error
    RunStop(ctx context.Context) error
    RunPermissionRequest(ctx context.Context, toolName string, input json.RawMessage) error
}

type HookSubmitResult struct {
    Block   bool   // true = reject the message
    Message string // possibly modified message
}
```

Hook execution for command hooks:
1. Set environment variables: `TOOL_NAME`, `TOOL_INPUT`, `TOOL_OUTPUT`, `USER_MESSAGE`
2. Run the command via `os/exec`
3. Check exit code: 0 = continue, non-zero = block/error
4. Capture stdout as hook output (for prompt injection or feedback)

### `internal/skills/`

| File | Purpose |
|------|---------|
| `loader.go` | Discover and parse skill files from `~/.claude/skills/` and `.claude/skills/` |
| `types.go` | `Skill` struct (name, description, trigger, content) |
| `loader_test.go` | Tests for skill loading |

```go
type Skill struct {
    Name        string
    Description string
    Trigger     string // slash command name, e.g. "/commit"
    Content     string // markdown body (instructions/prompt)
    FilePath    string // source file for debugging
}

func LoadSkills(cwd string) []Skill
func ActiveSkillContent(skills []Skill) string  // for system prompt injection
```

---

## Output format handlers

### JSONStreamHandler

Collects the full response and emits a single JSON object when done:

```go
type JSONStreamHandler struct {
    writer    io.Writer
    content   []api.ContentBlock
    usage     api.Usage
    model     string
    stopReason string
}
```

On `OnMessageStop`, marshals and writes:

```json
{
  "type": "message",
  "role": "assistant",
  "content": [{"type": "text", "text": "..."}],
  "model": "claude-sonnet-4-20250514",
  "stop_reason": "end_turn",
  "usage": {"input_tokens": 100, "output_tokens": 50}
}
```

### StreamJSONStreamHandler

Emits one JSON line per event:

```go
type StreamJSONStreamHandler struct {
    writer io.Writer
}
```

Each event is written immediately as a line:

```json
{"type":"message_start","message":{...}}
{"type":"content_block_start","index":0,"content_block":{...}}
{"type":"text_delta","index":0,"text":"Hello"}
{"type":"message_stop"}
```

Both handlers live in a new file, e.g. `internal/conversation/json_handlers.go`,
since they implement `api.StreamHandler` and are used in both print mode and
pipe mode.

---

## Pipe/stdin support

When stdin is not a terminal (piped input), read the prompt from stdin:

```go
import "golang.org/x/term"

if !term.IsTerminal(int(os.Stdin.Fd())) {
    // Read prompt from stdin pipe.
    data, _ := io.ReadAll(os.Stdin)
    initialPrompt = string(data)
    *printMode = true  // force print mode when piped
}
```

This goes in `main.go` after CLI flag parsing, before the print mode branch.
Combined with `--output-format json`, this enables Unix pipeline workflows:

```bash
echo "Explain this code" | claude -p --output-format json
cat file.go | claude -p "Review this code"
```

---

## CLI flag additions

| Flag | Type | Default | Purpose |
|------|------|---------|---------|
| `--output-format` | string | `"text"` | Output format: `text`, `json`, `stream-json` |

No new flags are needed for hooks or skills — they're configuration-driven
via `settings.json` and skill files.

---

## Files changed

| File | Change |
|------|--------|
| `internal/hooks/types.go` | **New.** Hook config types, event constants |
| `internal/hooks/runner.go` | **New.** Hook execution (command, prompt, agent) |
| `internal/hooks/runner_test.go` | **New.** Hook runner tests |
| `internal/skills/types.go` | **New.** Skill struct |
| `internal/skills/loader.go` | **New.** Skill discovery and parsing |
| `internal/skills/loader_test.go` | **New.** Skill loader tests |
| `internal/conversation/loop.go` | **Modify.** Add `HookRunner` to `Loop`, fire PreToolUse/PostToolUse/UserPromptSubmit/Stop hooks |
| `internal/conversation/loop.go` | **Modify.** Add `Hooks` field to `LoopConfig` |
| `internal/conversation/system_prompt.go` | **Modify.** Accept skill content parameter |
| `internal/conversation/json_handlers.go` | **New.** `JSONStreamHandler`, `StreamJSONStreamHandler` |
| `internal/config/settings.go` | No structural changes needed (Hooks field already exists) |
| `internal/tools/agent.go` | **Modify.** Add `hooks` field, propagate to sub-agent LoopConfig |
| `internal/tui/slash.go` | **Modify.** Register skill-based slash commands |
| `internal/tui/model.go` | **Modify.** Accept skills for slash command registration |
| `internal/tui/app.go` | **Modify.** Pass skills to model, fire SessionStart hook |
| `cmd/claude/main.go` | **Modify.** Parse hooks, load skills, add --output-format, pipe support, wire hooks into loop and agent |

### Files NOT changed

| File | Why |
|------|-----|
| `internal/tools/registry.go` | Hooks don't change tool dispatch — they wrap it |
| `internal/config/permissions.go` | PermissionRequest hook wraps the handler, doesn't modify it |
| `internal/api/` | No API changes |
| `internal/mcp/` | MCP is independent of hooks/skills |
| `internal/session/` | Session format doesn't change |
| `internal/tui/stream.go` | TUI stream handler is unaffected; output format handlers are separate |

---

## Ordering and dependencies

```
1. internal/hooks/types.go       — no deps, define types first
2. internal/hooks/runner.go      — depends on types.go
3. internal/skills/types.go      — no deps
4. internal/skills/loader.go     — depends on types.go, os, filepath
5. conversation/loop.go          — add HookRunner interface + firing points
6. conversation/system_prompt.go — add skill content parameter
7. conversation/json_handlers.go — new StreamHandler implementations
8. tools/agent.go                — add hooks propagation
9. tui/slash.go                  — register skill commands
10. tui/model.go + app.go        — pass skills, fire SessionStart
11. cmd/claude/main.go           — wire everything together
```

Items 1–4 can be done in parallel. Items 5–8 can be done in parallel.
Items 9–10 can be done together. Item 11 must be last.

---

## How hooks fire in the loop — detailed walkthrough

### PreToolUse / PostToolUse

In `Loop.run()`, the tool execution block (lines 133–160) becomes:

```go
for _, block := range resp.Content {
    if block.Type != api.ContentTypeToolUse {
        continue
    }

    // Phase 7: PreToolUse hook.
    if l.hooks != nil {
        if err := l.hooks.RunPreToolUse(ctx, block.Name, block.Input); err != nil {
            result := MakeToolResult(block.ID,
                fmt.Sprintf("Hook blocked tool execution: %v", err), true)
            toolResults = append(toolResults, result)
            continue
        }
    }

    // Existing tool execution (line 147).
    output, execErr := l.toolExec.Execute(ctx, block.Name, block.Input)

    // Phase 7: PostToolUse hook.
    if l.hooks != nil {
        l.hooks.RunPostToolUse(ctx, block.Name, block.Input, output, execErr != nil)
    }

    // Existing result handling (lines 148–159).
    // ...
}
```

### Stop hook

At line 127–131:

```go
if resp.StopReason != api.StopReasonToolUse {
    // Phase 7: Stop hook.
    if l.hooks != nil {
        l.hooks.RunStop(ctx)
    }
    l.notifyTurnComplete()
    return nil
}
```

### SessionStart hook

Fires once in `main.go` after the loop is created:

```go
loop := conversation.NewLoop(loopCfg)

// Phase 7: Fire SessionStart hook.
if hookRunner != nil {
    hookRunner.RunSessionStart(ctx)
}
```

---

## How skills register slash commands

After loading skills in `main.go`:

```go
skills := skills.LoadSkills(cwd)
```

Pass them to the TUI via `AppConfig`:

```go
type AppConfig struct {
    // ... existing fields ...
    Skills []skills.Skill  // Phase 7
}
```

In `newModel` or `app.Run`, register slash commands:

```go
for _, skill := range cfg.Skills {
    if skill.Trigger == "" {
        continue
    }
    // Strip leading "/" from trigger.
    name := strings.TrimPrefix(skill.Trigger, "/")
    content := skill.Content
    slash.register(SlashCommand{
        Name:        name,
        Description: skill.Description,
        Execute: func(m *model) string {
            // Send the skill's content as a user message.
            // This triggers the agentic loop with the skill's instructions.
            return ""  // handled specially — submits content as a message
        },
    })
}
```

Skill slash commands are special: they don't just return a string — they
submit the skill's prompt as a user message to the loop. This requires
either returning a special sentinel or handling them in `handleSubmit`
(similar to how `/compact` and `/quit` are handled specially today).

---

## How to verify

1. **Command hooks** — Configure a `PreToolUse` hook that logs to a file.
   Trigger a tool call and verify the log entry.

2. **Hook blocking** — Configure a `PreToolUse` hook that exits non-zero
   for `Bash` calls containing `rm`. Verify the tool is blocked.

3. **UserPromptSubmit hook** — Configure a hook that prepends context.
   Verify the modified message appears in the conversation.

4. **SessionStart hook** — Configure a hook that runs a setup script.
   Verify it runs when the CLI starts.

5. **Stop hook** — Configure a hook that runs cleanup. Verify it fires
   when the conversation ends.

6. **Skills loading** — Create a `.claude/skills/commit.md` with
   frontmatter. Verify `/commit` appears in `/help` output.

7. **Skill execution** — Run `/commit` and verify the skill's prompt
   is sent to the model.

8. **JSON output** — Run `claude -p "hello" --output-format json` and
   verify valid JSON output.

9. **Stream-JSON output** — Run with `--output-format stream-json` and
   verify one JSON line per event.

10. **Pipe support** — Run `echo "hello" | claude -p` and verify it
    reads the prompt from stdin.

11. **Sub-agent hooks** — Trigger a sub-agent and verify PreToolUse hooks
    fire for the sub-agent's tool calls.

12. **PermissionRequest hook** — Configure a hook, trigger a permission
    prompt, verify the hook fires.
