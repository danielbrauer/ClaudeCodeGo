# Phase 5: TUI — Integration Guide

Phase 5 replaces the current `fmt.Print`/`bufio.Scanner` UI with a rich terminal experience. The new `internal/tui/` package takes over all user-facing I/O: streaming text display, markdown rendering, syntax highlighting, diff views, tool progress, permission prompts, todo list display, and multi-line input editing.

**The TUI is a consumer of existing interfaces.** It does not change tool logic, API types, or the agentic loop. It replaces the thin presentation layer currently scattered across `main.go`, `loop.go` (stream handlers), `permission.go`, `todo.go`, and `askuser.go`.

---

## What exists today

### The REPL (`cmd/claude/main.go`, lines 248–311)

The current interactive loop is minimal:

```go
scanner := bufio.NewScanner(os.Stdin)
for {
    fmt.Print("> ")
    if !scanner.Scan() { break }
    line := strings.TrimSpace(scanner.Text())
    // ... slash commands ...
    loop.SendMessage(ctx, line)
}
```

**Problems the TUI solves:**
- Single-line input only — no multi-line editing
- No markdown rendering — assistant output is raw text
- No syntax highlighting in code blocks
- No diff display for FileEdit results
- No spinner or progress indicator during API calls or tool execution
- Slash commands are a flat `switch` block with no completion
- No token/cost display in the UI chrome

**What replaces it:** A `tui.App` struct that owns the full terminal lifecycle. `main.go` creates it and calls `app.Run(ctx)` instead of the manual REPL loop.

### The StreamHandler interface (`internal/api/streaming.go`, lines 77–86)

This is **the** integration point between the API layer and the display layer:

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

Today's `ToolAwareStreamHandler` (`loop.go`, lines 190–328) implements this with plain `fmt.Printf`. The TUI replaces it with a handler that:
- Renders text deltas into a markdown-aware viewport (token by token)
- Shows a spinner/progress bar during tool calls
- Displays tool call summaries with icons and collapsible detail
- Tracks and displays token usage from `OnMessageStart` and `OnMessageDelta`

**The handler is injected via `LoopConfig.Handler`.** The loop calls the handler methods; the handler updates the terminal. No changes to the loop are needed.

### The PermissionHandler interface (`internal/tools/registry.go`, lines 32–36)

```go
type PermissionHandler interface {
    RequestPermission(ctx context.Context, toolName string, input json.RawMessage) (bool, error)
}
```

Today's `TerminalPermissionHandler` (`permission.go`) uses raw `fmt.Printf` and `bufio.Reader`. The TUI replaces it with a styled permission prompt rendered within the TUI framework — highlighting the tool name, showing a syntax-highlighted command preview for Bash, and offering y/n via key events rather than line-based stdin reading.

**The handler is injected via `tools.NewRegistry(permHandler)`.** The registry calls `RequestPermission` before executing any tool where `RequiresPermission()` returns true.

### The conversation loop (`internal/conversation/loop.go`)

```go
type LoopConfig struct {
    Client         *api.Client
    System         []api.SystemBlock
    Tools          []api.ToolDefinition
    ToolExec       ToolExecutor
    Handler        api.StreamHandler        // ← TUI provides this
    History        *History
    Compactor      *Compactor
    OnTurnComplete func(history *History)   // ← TUI can hook into this
}
```

The loop is **not modified** in Phase 5. It already accepts a `StreamHandler` and a turn-complete callback. The TUI plugs into both:
- `Handler` → a TUI stream handler that renders to the terminal
- `OnTurnComplete` → the TUI can update status (token count, session info) and trigger session save

The loop's `SendMessage(ctx, userMessage) error` method is called by the TUI when the user submits input. The loop is **blocking** — it runs the full agentic cycle (API call → tool execution → repeat) and returns when done. The TUI needs to call this from a goroutine if the TUI framework requires a main-thread event loop (Bubble Tea does).

### Tools that currently own their own I/O

Several tools bypass the stream handler and write directly to stdout/stdin. The TUI must intercept or replace this:

| Tool | Current I/O | What the TUI needs to do |
|------|-------------|--------------------------|
| `TodoWriteTool` (`todo.go`, lines 89–105) | `fmt.Printf` to stdout for task list display | Provide a callback or output sink. The tool should call a `TodoRenderer` interface instead of printing. |
| `AskUserTool` (`askuser.go`, lines 130–195) | `fmt.Printf` + `bufio.Reader` for interactive Q&A | Provide a `UserPrompter` interface. The TUI shows styled option pickers and collects input via the TUI event loop. |
| `TerminalPermissionHandler` (`permission.go`, lines 25–47) | `fmt.Printf` + `bufio.Reader` for y/n prompts | Replace with a TUI-aware permission handler (already an interface — just provide a new implementation). |

**The cleanest approach:** Define small callback interfaces that these tools accept at construction time. The tools call the callback for display; the TUI provides the implementation. If no callback is set, fall back to the current `fmt.Printf` behavior (keeps `-p` print mode working).

---

## Architecture of `internal/tui/`

### Proposed file structure

```
internal/tui/
├── app.go          # App struct, Run() loop, wiring
├── input.go        # Multi-line input editor, key bindings
├── output.go       # Markdown rendering, syntax highlighting, diff display
├── stream.go       # StreamHandler implementation for the TUI
├── permission.go   # TUI permission prompt (implements tools.PermissionHandler)
├── progress.go     # Spinner, tool execution indicators
├── slash.go        # Slash command parsing, tab completion
├── todo.go         # Todo list renderer component
├── status.go       # Status bar: model, token count, cost, session ID
└── theme.go        # Colors, styles, box drawing
```

### The `App` struct (`tui/app.go`)

This is the entry point. `main.go` creates it and calls `Run`:

```go
type App struct {
    loop       *conversation.Loop
    client     *api.Client
    session    *session.Session
    sessStore  *session.Store
    version    string
    model      string
}

func New(cfg AppConfig) *App
func (a *App) Run(ctx context.Context) error
```

`AppConfig` bundles everything from `main.go` that the TUI needs:

```go
type AppConfig struct {
    Loop        *conversation.Loop
    Client      *api.Client
    Session     *session.Session
    SessStore   *session.Store
    Version     string
    Model       string
    PrintMode   bool   // if true, use plain PrintStreamHandler instead of TUI
}
```

### How `main.go` changes

The current `main.go` (lines 214–311) does two things after creating the loop:
1. Handles an initial prompt from CLI args
2. Runs the interactive REPL

Phase 5 replaces the REPL section. The new flow:

```go
// Create the TUI app.
app := tui.New(tui.AppConfig{
    Loop:      loop,
    Client:    client,
    Session:   currentSession,
    SessStore: sessionStore,
    Version:   version,
    Model:     model,
    PrintMode: *printMode,
})

// Handle initial prompt.
if len(flag.Args()) > 0 {
    prompt := strings.Join(flag.Args(), " ")
    if *printMode {
        // Print mode: plain handler, no TUI.
        loop.SendMessage(ctx, prompt)
        os.Exit(0)
    }
    app.SetInitialPrompt(prompt)
}

// Run the TUI (or exit if print mode with no args).
if *printMode {
    os.Exit(0)
}
if err := app.Run(ctx); err != nil {
    fmt.Fprintf(os.Stderr, "Error: %v\n", err)
    os.Exit(1)
}
```

**Print mode (`-p`)** should continue using `PrintStreamHandler` directly — no TUI framework. This keeps scripting/piping simple.

---

## Interface-by-interface integration

### 1. StreamHandler → TUI renderer

**File:** `internal/tui/stream.go`

The TUI provides its own `api.StreamHandler` implementation:

```go
type TUIStreamHandler struct {
    app *App  // or a channel/callback to push updates into the TUI event loop
}
```

**Method mapping:**

| StreamHandler method | TUI behavior |
|----------------------|-------------|
| `OnMessageStart(msg)` | Record `msg.Usage.InputTokens`, update status bar. Start a new assistant message region in the viewport. |
| `OnContentBlockStart(index, block)` | If `block.Type == "tool_use"`: show spinner with tool name. If `text`: prepare text rendering region. |
| `OnTextDelta(index, text)` | Append `text` to the current markdown render buffer. Re-render the visible portion. This is the hot path — must be fast. |
| `OnInputJSONDelta(index, json)` | Accumulate tool input JSON (same as today). Optionally show a progress dot or byte count. |
| `OnContentBlockStop(index)` | If tool_use: stop spinner, show tool summary line (using `toolInputSummary` logic). If text: finalize the markdown block. |
| `OnMessageDelta(delta, usage)` | Update `OutputTokens` in status bar. Record `stop_reason`. |
| `OnMessageStop()` | Finalize the assistant message. Re-enable user input. Update token/cost display. |
| `OnError(err)` | Display error in a styled error region (red, bordered). |

**Threading concern:** If using Bubble Tea, stream handler methods are called from the goroutine running `loop.SendMessage`. They must not directly mutate TUI state. Instead, send `tea.Msg` values via a channel or `program.Send()`:

```go
func (h *TUIStreamHandler) OnTextDelta(index int, text string) {
    h.program.Send(TextDeltaMsg{Index: index, Text: text})
}
```

The Bubble Tea `Update` function processes these messages on the main goroutine.

### 2. PermissionHandler → TUI permission prompt

**File:** `internal/tui/permission.go`

```go
type TUIPermissionHandler struct {
    program *tea.Program  // or a request/response channel pair
}

func (h *TUIPermissionHandler) RequestPermission(
    ctx context.Context, toolName string, input json.RawMessage,
) (bool, error)
```

**Interaction pattern:**
1. The agentic loop calls `RequestPermission` from its goroutine.
2. The handler sends a `PermissionRequestMsg` to the TUI event loop.
3. The TUI renders a styled prompt: tool name, action summary, y/n hint.
4. The user presses a key (y/n/a for always-allow).
5. The TUI sends the result back to the handler via a channel.
6. The handler returns `(true/false, nil)` to the registry.

This is a **synchronous call from the loop's perspective** but async from the TUI's perspective. Use a channel pair:

```go
type permRequest struct {
    toolName string
    input    json.RawMessage
    result   chan bool
}
```

### 3. TodoWrite → TUI todo display

**File:** `internal/tui/todo.go`

Currently `TodoWriteTool.Execute` calls `fmt.Printf` directly (lines 89–105). Two options:

**Option A: Callback interface (preferred)**

Define a renderer interface in the tools package:

```go
// internal/tools/todo.go
type TodoRenderer interface {
    RenderTodos(todos []TodoItem)
}
```

`TodoWriteTool` accepts an optional renderer:

```go
func NewTodoWriteTool() *TodoWriteTool                              // default: fmt.Printf
func NewTodoWriteToolWithRenderer(r TodoRenderer) *TodoWriteTool    // TUI provides this
```

**Option B: Remove print from tool, render in stream handler**

The tool returns JSON only. The TUI stream handler inspects `OnContentBlockStop` for `TodoWrite` tool calls, parses the result JSON, and renders the todo list in a dedicated TUI region. This is cleaner (tools don't do I/O) but requires the stream handler to know about tool semantics.

### 4. AskUserQuestion → TUI option picker

**File:** `internal/tui/askuser.go` or incorporated into `permission.go`

Same pattern as the permission handler. Currently `AskUserTool` reads from stdin directly. Replace with a `UserPrompter` interface:

```go
// internal/tools/askuser.go
type UserPrompter interface {
    AskQuestion(ctx context.Context, question AskUserQuestionItem) (string, error)
}
```

The TUI implementation renders a styled picker with numbered options, arrow key navigation, and "Other" free-text input. The tool calls the prompter instead of `fmt.Printf`/`reader.ReadString`.

### 5. Slash commands → TUI command dispatch

**File:** `internal/tui/slash.go`

The current slash command handling (`main.go`, lines 269–301) is a `switch` block. Move this into the TUI as a command registry:

```go
type SlashCommand struct {
    Name        string
    Description string
    Execute     func(app *App, args string) error
}
```

Built-in commands (matching the official CLI):

| Command | Action |
|---------|--------|
| `/help` | Show help in a styled panel |
| `/model` | Show current model (or switch if arg provided) |
| `/version` | Show version |
| `/compact` | Call `loop.Compact(ctx)`, show result |
| `/cost` | Show cumulative token usage and estimated cost |
| `/context` | Show context window usage breakdown |
| `/quit` | Clean exit |
| `/memory` | Open CLAUDE.md in `$EDITOR` |
| `/hooks` | List configured hooks |
| `/agents` | List sub-agents |
| `/mcp` | Show MCP server status |
| `/init` | Create `.claude/CLAUDE.md` |
| `/doctor` | Run diagnostics |
| `/fast` | Toggle fast mode |

The TUI input handler checks for `/` prefix, autocompletes command names on Tab, and dispatches.

---

## Key data flows

### User sends a message

```
User types text → Input editor captures it
                → App calls loop.SendMessage(ctx, text) in a goroutine
                → Loop builds API request, calls client.CreateMessageStream
                → SSE events arrive → StreamHandler methods called
                → TUI receives messages, updates display
                → If tool_use: loop calls registry.Execute
                → If permission needed: PermissionHandler.RequestPermission
                → TUI shows prompt, user responds
                → Tool executes, result added to history
                → Loop continues until end_turn
                → OnMessageStop → TUI re-enables input
```

### Token tracking

Token data arrives in two places:

1. `OnMessageStart(msg)` → `msg.Usage.InputTokens` (how many input tokens this request consumed)
2. `OnMessageDelta(delta, usage)` → `usage.OutputTokens` (final output token count)

The TUI accumulates these across the session for the `/cost` command:

```go
type TokenTracker struct {
    TotalInputTokens  int
    TotalOutputTokens int
    TotalCacheRead    int
    TotalCacheWrite   int
    TurnCount         int
}
```

Update on every `OnMessageStart` and `OnMessageDelta`. Display in the status bar and via `/cost`.

### Markdown rendering

Text arrives as incremental deltas via `OnTextDelta`. The TUI must:

1. Buffer text into a complete markdown document (append each delta)
2. Re-render the visible portion after each delta
3. Handle partial markdown gracefully (e.g., an unclosed code fence during streaming)

**Rendering strategy:**
- Use `charmbracelet/glamour` for full markdown → ANSI rendering
- Re-render the entire accumulated text on each delta (glamour is fast enough for this)
- OR use a simpler line-by-line renderer that handles code fences, bold, italic, links, and lists without re-rendering everything
- Show a cursor/blinking indicator at the end of streaming text

**Code blocks** need syntax highlighting. Glamour uses `alecthomas/chroma` under the hood. Detect the language from the code fence annotation (` ```go `) and apply the appropriate lexer.

### Diff display for FileEdit

When `FileEdit` executes, it returns a success message. To show a diff, the TUI can either:

**Option A:** Intercept the tool call in `OnContentBlockStop`, parse the `old_string`/`new_string` from the assembled JSON input, and render an inline diff view with red/green lines.

**Option B:** Add a hook in the tool executor that emits a "tool completed" event with structured data the TUI can render. This is cleaner but requires a new callback:

```go
type ToolEventHandler interface {
    OnToolStart(name string, input json.RawMessage)
    OnToolComplete(name string, input json.RawMessage, output string, err error)
}
```

The `ToolAwareStreamHandler` already has the tool name and assembled JSON at `OnContentBlockStop` time, so Option A is simpler and requires no interface changes.

---

## Changes to existing files

| File | Change |
|------|--------|
| `internal/tui/app.go` | **New.** Main TUI application struct, `Run()` method. |
| `internal/tui/input.go` | **New.** Multi-line input editor with key bindings. |
| `internal/tui/output.go` | **New.** Markdown rendering, code highlighting, diff display. |
| `internal/tui/stream.go` | **New.** `api.StreamHandler` implementation for the TUI. |
| `internal/tui/permission.go` | **New.** `tools.PermissionHandler` implementation for the TUI. |
| `internal/tui/progress.go` | **New.** Spinner and progress indicators. |
| `internal/tui/slash.go` | **New.** Slash command registry and dispatch. |
| `internal/tui/todo.go` | **New.** Todo list rendering component. |
| `internal/tui/status.go` | **New.** Status bar (model, tokens, cost, session). |
| `internal/tui/theme.go` | **New.** Color palette, box styles, ANSI constants. |
| `cmd/claude/main.go` | **Modify.** Replace REPL loop (lines 248–311) with `tui.App` creation and `app.Run()`. Keep print mode as-is. |
| `internal/tools/todo.go` | **Modify.** Add optional `TodoRenderer` callback. Remove direct `fmt.Printf`. |
| `internal/tools/askuser.go` | **Modify.** Add optional `UserPrompter` callback. Remove direct `fmt.Printf`/`bufio.Reader`. |
| `internal/tools/permission.go` | **No change.** `TerminalPermissionHandler` stays as the fallback for print mode. TUI provides its own `PermissionHandler` implementation. |
| `internal/conversation/loop.go` | **No change.** The loop already accepts `StreamHandler` and `OnTurnComplete`. The two stream handlers (`PrintStreamHandler`, `ToolAwareStreamHandler`) stay as fallbacks for non-TUI modes. |
| `internal/api/streaming.go` | **No change.** `StreamHandler` interface is sufficient. |
| `go.mod` | **Modify.** Add TUI dependencies. |

---

## Dependencies to add

| Package | Purpose | Notes |
|---------|---------|-------|
| `github.com/charmbracelet/bubbletea` | TUI framework | Event loop, model-view-update pattern. Alternative: `tcell` for lower-level control. |
| `github.com/charmbracelet/lipgloss` | Terminal styling | Colors, borders, padding, alignment. |
| `github.com/charmbracelet/glamour` | Markdown rendering | Converts markdown → styled ANSI. Uses chroma for syntax highlighting. |
| `github.com/charmbracelet/bubbles` | Pre-built components | Text input, spinner, viewport, list, etc. |

The Charm stack (bubbletea + lipgloss + glamour + bubbles) is the standard choice for Go TUIs. It provides everything needed and is actively maintained.

**Alternative:** `tcell` + manual rendering. More control, more work. Only consider if Bubble Tea's model-view-update pattern doesn't fit well with the streaming handler pattern (it does — Bubble Tea's `program.Send()` is designed for exactly this).

---

## Bubble Tea integration pattern

Bubble Tea uses the Elm architecture: `Model` → `Update(msg) → Model` → `View() → string`.

The key challenge is bridging the **blocking** agentic loop with Bubble Tea's **non-blocking** event loop.

### The goroutine bridge

```go
type model struct {
    // ... TUI state ...
    loopDone chan error    // signals when the agentic loop finishes
}

// When user submits input:
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case SubmitMsg:
        return m, func() tea.Msg {
            err := m.loop.SendMessage(m.ctx, msg.Text)
            return LoopDoneMsg{Err: err}
        }
    case TextDeltaMsg:
        m.outputBuffer += msg.Text
        return m, nil
    case ToolStartMsg:
        m.showSpinner = true
        return m, m.spinner.Tick
    case PermissionRequestMsg:
        m.permissionPending = &msg
        return m, nil
    // ...
    }
}
```

The stream handler sends `tea.Msg` values via `program.Send()`. The `Update` function processes them and updates the model. `View()` renders the current state.

### Message types

Define one `tea.Msg` type per stream handler event:

```go
type MessageStartMsg struct { Usage api.Usage }
type TextDeltaMsg struct { Index int; Text string }
type InputJSONDeltaMsg struct { Index int; JSON string }
type ContentBlockStartMsg struct { Index int; Block api.ContentBlock }
type ContentBlockStopMsg struct { Index int; Name string; Input json.RawMessage }
type MessageDeltaMsg struct { Delta api.MessageDeltaBody; Usage *api.Usage }
type MessageStopMsg struct{}
type StreamErrorMsg struct { Err error }
type LoopDoneMsg struct { Err error }
type PermissionRequestMsg struct { ToolName string; Input json.RawMessage; Result chan bool }
type TodoUpdateMsg struct { Todos []tools.TodoItem }
type AskUserMsg struct { Question tools.AskUserQuestionItem; Result chan string }
```

---

## How to verify

1. **Streaming text** — send a prompt, verify text appears token-by-token with markdown formatting (bold, code, lists, headings).

2. **Code blocks** — verify syntax highlighting in fenced code blocks (Go, Python, JS at minimum).

3. **Tool execution** — trigger a Bash tool call, verify spinner appears during execution, summary line appears on completion.

4. **Permission prompt** — trigger a tool requiring permission (Bash, FileEdit, FileWrite), verify styled prompt appears, y/n response works.

5. **Diff display** — trigger a FileEdit, verify the old/new strings are shown as a colored diff.

6. **Todo list** — trigger a TodoWrite, verify the task list renders with status icons.

7. **Multi-line input** — verify Shift+Enter or similar key combo creates a newline; Enter submits.

8. **Slash commands** — verify all commands from the table above work, with tab completion for command names.

9. **Token tracking** — verify `/cost` shows cumulative input/output tokens.

10. **Print mode** — verify `-p` flag still works without the TUI framework (plain text output).

11. **Ctrl+C** — verify clean exit during streaming, during tool execution, and at the input prompt.

12. **Terminal resize** — verify the TUI handles terminal resize events gracefully.

When in doubt about behavior, run the official `claude` CLI and observe how it handles each case.
