# Bubble Tea Integration Plan

This document covers the design decisions and concrete patterns for integrating
Bubble Tea (BT) into ClaudeCodeGo's TUI layer. It is intended for the
implementor of Phase 5.

---

## 1. No Alternate Screen

BT defaults to **inline mode** — it does NOT enter the alternate screen unless
you explicitly pass `tea.WithAltScreen()`. We must never use that option.

```go
p := tea.NewProgram(model) // inline mode, scrollback preserved
// NOT: tea.NewProgram(model, tea.WithAltScreen())
```

In inline mode, BT's standard renderer maintains a "live region" at the
bottom of the terminal output. It tracks how many lines it last rendered
(`linesRendered`) and uses ANSI cursor-up sequences to reposition and
overwrite that region on each frame. Everything above the live region is
normal terminal scrollback.

This is exactly the behavior we need: scrollback stays intact, and the TUI
manages a small live area at the bottom for interactive elements.

---

## 2. The Two-Region Model

All output falls into one of two categories:

### Scrollback (persistent)

Output that has been "committed" — it will never be redrawn or removed.
Users can scroll up through their terminal to review it. This includes:

- Completed assistant text (after streaming finishes)
- Tool call summaries and results
- Completed permission prompts (the question + answer)
- Completed todo list snapshots
- Error messages
- User input (echoed back after submission)

BT provides `tea.Println` and `tea.Printf` commands (returned from `Update`)
and `program.Println()` / `program.Printf()` methods (callable from any
goroutine). These print persistent lines above the live region. The standard
renderer coordinates this correctly — it flushes queued print lines, then
redraws the live region below them.

### Live region (ephemeral)

The return value of `View()`. This is redrawn every frame. It contains
whatever is "in progress":

- During streaming: the current (incomplete) assistant text being received
- During tool execution: spinner + tool name + status
- During permission prompt: the styled prompt awaiting y/n
- During user input: the input editor with cursor
- Status bar: model name, token count, session info

The live region should be kept **as small as practical**. More on this in the
streaming section below.

### Visual model

```
┌─ terminal scrollback ─────────────────────────────────┐
│ > User: How do I fix the auth bug?                     │ ← Println'd
│                                                        │
│ I'll look into the authentication code.                │ ← Println'd
│                                                        │
│ ● Read  src/auth/login.go                              │ ← Println'd
│ ● Edit  src/auth/login.go                              │ ← Println'd
│   - if token.Expired() {                               │
│   + if token == nil || token.Expired() {               │
│                                                        │
│ I've fixed the null check. The issue was...            │ ← Println'd
├─ live region (View) ──────────────────────────────────┤
│ [~] Investigating auth bug                             │ ← in-progress todo
│ [x] Read auth code                                     │ ← completed todo
│                                                        │
│ > _                                                    │ ← input prompt
│                                                        │
│ claude-opus-4-20250514 · 1.2k in / 890 out               │ ← status bar
└────────────────────────────────────────────────────────┘
```

---

## 3. Streaming Text

Text arrives token-by-token via `OnTextDelta`. The question is where this
in-progress text lives: in the View (live region) or in scrollback (Println).

### Recommended approach: accumulate in View, flush on completion

During streaming:

1. Each `OnTextDelta` sends a `TextDeltaMsg` to the BT program (via
   `program.Send`).
2. The `Update` function appends the text to a buffer (`m.streamingText`).
3. `View()` renders `m.streamingText` through glamour (markdown → ANSI).
4. The live region grows as text accumulates.

When the content block completes (`OnContentBlockStop` for a text block):

1. The complete markdown text is rendered via glamour.
2. The rendered output is committed to scrollback via `tea.Println`.
3. `m.streamingText` is cleared.
4. The live region shrinks back down.

This means during streaming, the View contains:
```
[rendered markdown of text so far]
[spinner or status]
```

And after completion, it collapses to just:
```
[spinner or status]  (or input prompt if turn is done)
```

### Terminal height concern

If the streaming text exceeds the terminal height, BT's cursor-up
repositioning will clamp to the top of the screen. The renderer will
overwrite visible lines from the top down, and earlier lines of the View
effectively scroll into terminal scrollback organically (the terminal
itself scrolls them up as new lines are written below).

In practice this works fine — the user sees the most recent portion of the
streaming text, which is the natural reading experience. When the block
completes and we Println the full rendered text, it all ends up in scrollback
properly.

If empirical testing reveals visual artifacts with very long streaming
responses, the fallback is **progressive flushing**: periodically detect
"safe" markdown boundaries (completed paragraphs, completed code blocks) and
Println those portions early, keeping only the trailing incomplete portion in
the View. This is more complex and should only be implemented if needed.

### Markdown rendering during streaming

Use `charmbracelet/glamour` for rendering. Re-render the full accumulated
text on each delta. Glamour is fast enough for this — it renders several KB
of markdown in under a millisecond.

Handle partial markdown gracefully:
- An unclosed code fence (` ``` ` without closing) should render as a code
  block anyway (glamour handles this).
- An incomplete bold marker (`**partial`) can be left as-is.
- The glamour render width should track terminal width (listen for
  `tea.WindowSizeMsg`).

---

## 4. Tool Rendering Interface

### Design principle: tools are BT-aware

Per the project requirement, tools in the Go version should know they're
talking to Bubble Tea. There is no generic `ToolRenderer` abstraction that
pretends the UI could be anything. Tools import `bubbletea` and use its
types directly.

The mechanism is simple: tools accept a `*tea.Program` at construction time.
When the program is non-nil (interactive TUI mode), tools use
`program.Send()` to push messages and `program.Println()` for persistent
output. When nil (print mode / `-p`), tools fall back to `fmt.Printf`.

### Message types tools send

Each tool-initiated UI event is a distinct `tea.Msg`. These are defined in the
`tui` package (or a shared `tui/msg` package to avoid import cycles):

```go
package msg

import (
    "encoding/json"
    "github.com/anthropics/claude-code-go/internal/tools"
)

// TodoUpdateMsg is sent by TodoWriteTool when the task list changes.
type TodoUpdateMsg struct {
    Todos []tools.TodoItem
}

// AskUserRequestMsg is sent by AskUserTool when it needs user input.
// The tool blocks on ResponseCh until the TUI sends an answer.
type AskUserRequestMsg struct {
    Questions  []tools.AskUserQuestionItem
    ResponseCh chan map[string]string // TUI sends answers here
}

// PermissionRequestMsg is sent by the permission handler.
type PermissionRequestMsg struct {
    ToolName string
    Input    json.RawMessage
    Summary  string
    ResultCh chan bool // TUI sends true/false here
}

// ToolStartMsg indicates a tool has started executing.
type ToolStartMsg struct {
    Name    string
    Summary string // e.g. "$ ls -la" or "src/main.go"
}

// ToolCompleteMsg indicates a tool has finished.
type ToolCompleteMsg struct {
    Name   string
    Output string
    Err    error
}
```

The BT model's `Update` function handles these with a type switch:

```go
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case msg.TodoUpdateMsg:
        m.todos = msg.Todos
        return m, nil

    case msg.AskUserRequestMsg:
        m.askUserPending = &msg
        m.mode = modeAskUser
        return m, nil

    case msg.PermissionRequestMsg:
        m.permissionPending = &msg
        m.mode = modePermission
        return m, nil

    // ...
    }
}
```

### Tool construction

Tools receive the program reference at construction time. Example for
TodoWriteTool:

```go
type TodoWriteTool struct {
    mu      sync.Mutex
    todos   []TodoItem
    program *tea.Program // nil in print mode
}

func NewTodoWriteTool(program *tea.Program) *TodoWriteTool {
    return &TodoWriteTool{program: program}
}

func (t *TodoWriteTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
    // ... parse input, update todos ...

    if t.program != nil {
        t.program.Send(msg.TodoUpdateMsg{Todos: t.todos})
    } else {
        // Print mode fallback
        for _, item := range t.todos {
            fmt.Printf("  [%s] %s\n", statusIcon(item.Status), item.Content)
        }
    }

    // ... return JSON result ...
}
```

### What each tool renders

| Tool | During execution | On completion |
|------|-----------------|---------------|
| **Bash** | Spinner + `$ command` summary in View | Println: output (truncated if long), exit code on failure |
| **FileRead** | Spinner + file path in View | Println: summary line (e.g., "Read 142 lines from src/main.go") |
| **FileEdit** | Spinner + file path in View | Println: inline diff (red/green) of old_string → new_string |
| **FileWrite** | Spinner + file path in View | Println: "Wrote 1.2KB to src/main.go" |
| **Glob** | Spinner + pattern in View | Println: file list (collapsed if > 20 files) |
| **Grep** | Spinner + `/pattern/` in View | Println: matching lines with highlighted matches |
| **TodoWrite** | Updates todo list in View (lives in the live region, always visible) | No Println — the todo list in View is the canonical display |
| **AskUserQuestion** | Shows interactive picker in View | Println: the question + chosen answer |
| **Agent** | Spinner + description in View | Println: sub-agent result summary |
| **WebFetch** | Spinner + URL in View | Println: fetched content summary |
| **WebSearch** | Spinner + query in View | Println: result count + top results |

### The diff display for FileEdit

FileEdit is the most visually complex tool output. When a FileEdit completes,
render an inline diff:

```
● Edit  src/auth/login.go
  - if token.Expired() {
  + if token == nil || token.Expired() {
```

Use lipgloss styles for coloring: red background for removed lines, green
for added lines. The tool itself doesn't need to do this rendering — the
stream handler or a post-execution hook sees the tool name + input JSON
(which contains `old_string` and `new_string`) and renders the diff via
Println.

This logic belongs in the TUI's stream handler (at `OnContentBlockStop` time)
or in a `ToolEventHandler` callback on the registry, **not** in the FileEdit
tool itself.

---

## 5. Interactive Tools (Blocking User Input)

Two tools require user interaction during execution: `AskUserQuestion` and
the permission system. Both follow the same channel-based pattern.

### The channel handshake

The tool runs on the agentic loop's goroutine. It needs to pause, show UI,
wait for user input, and resume. BT's Update loop runs on the main goroutine.

Pattern:

```
Agentic loop goroutine          BT main goroutine
─────────────────────           ──────────────────
tool.Execute() called
  │
  ├─ create responseCh
  ├─ program.Send(RequestMsg{ResponseCh: responseCh})
  ├─ block on <-responseCh     ──→ Update receives RequestMsg
  │                                  ├─ render picker/prompt in View
  │                                  ├─ user presses key
  │                                  └─ responseCh <- answer
  ├─ receive answer            ←──
  └─ continue execution
```

The `responseCh` is created by the tool, included in the message, and the
BT Update loop sends the response back through it. This keeps the tool's
Execute method synchronous from the agentic loop's perspective.

### Permission handler implementation

```go
type TUIPermissionHandler struct {
    program *tea.Program
}

func (h *TUIPermissionHandler) RequestPermission(
    ctx context.Context, toolName string, input json.RawMessage,
) (bool, error) {
    if h.program == nil {
        // Shouldn't happen in TUI mode, but degrade gracefully
        return false, nil
    }

    resultCh := make(chan bool, 1)
    h.program.Send(msg.PermissionRequestMsg{
        ToolName: toolName,
        Input:    input,
        Summary:  summarizeToolInput(toolName, input),
        ResultCh: resultCh,
    })

    select {
    case <-ctx.Done():
        return false, ctx.Err()
    case allowed := <-resultCh:
        return allowed, nil
    }
}
```

The BT model handles this in Update:

```go
case msg.PermissionRequestMsg:
    m.permissionPending = &msg
    return m, nil

case tea.KeyMsg:
    if m.permissionPending != nil {
        switch msg.String() {
        case "y":
            m.permissionPending.ResultCh <- true
            // Println the prompt + "allowed" for scrollback
            cmd := tea.Println(renderPermissionResult(m.permissionPending, true))
            m.permissionPending = nil
            return m, cmd
        case "n":
            m.permissionPending.ResultCh <- false
            cmd := tea.Println(renderPermissionResult(m.permissionPending, false))
            m.permissionPending = nil
            return m, cmd
        }
    }
```

### AskUserQuestion works identically

Same channel pattern, but the View renders a numbered option list with
arrow-key navigation instead of a simple y/n prompt. The response channel
carries a `map[string]string` of question→answer pairs.

---

## 6. The Goroutine Bridge

The agentic loop (`loop.SendMessage`) is blocking — it runs the full cycle
of API call → tool execution → repeat until end_turn. BT requires the main
goroutine for its event loop. These must be bridged.

### Launching the loop

When the user submits input, return a `tea.Cmd` that runs the loop in a
goroutine:

```go
case SubmitInputMsg:
    m.inputText = ""
    m.mode = modeStreaming
    userText := msg.Text

    return m, func() tea.Msg {
        // This runs in a goroutine managed by BT's command system.
        err := m.loop.SendMessage(m.ctx, userText)
        return LoopDoneMsg{Err: err}
    }
```

The stream handler sends intermediate messages via `program.Send()`:

```go
type TUIStreamHandler struct {
    program *tea.Program
}

func (h *TUIStreamHandler) OnTextDelta(index int, text string) {
    h.program.Send(TextDeltaMsg{Index: index, Text: text})
}

func (h *TUIStreamHandler) OnContentBlockStart(index int, block api.ContentBlock) {
    h.program.Send(ContentBlockStartMsg{Index: index, Block: block})
}

// etc.
```

Each of these `Send` calls wakes the BT event loop, which processes the
message in `Update` and calls `View` to redraw.

### Avoiding deadlocks

The channel handshake (permission, ask-user) introduces a potential deadlock
if misused:

- The agentic loop goroutine blocks on `responseCh`.
- The BT Update loop must be free to process the request message and send
  the response.
- If anything in the BT event loop tried to synchronously call into the
  agentic loop, it would deadlock.

Rule: **never call blocking loop methods from Update.** Always use `tea.Cmd`
(which runs in a goroutine) for anything that touches the loop. The `Send`
direction (loop → BT) is always non-blocking (channel send), and the
response direction (BT → loop) is a single channel send that unblocks the
waiting goroutine.

---

## 7. The View Structure

The `View()` function composes the live region from several components:

```go
func (m model) View() string {
    var b strings.Builder

    // 1. Streaming text (if currently receiving)
    if m.streamingText != "" {
        rendered := m.renderMarkdown(m.streamingText)
        b.WriteString(rendered)
        b.WriteString("\n")
    }

    // 2. Active tool (spinner + summary)
    if m.activeTool != "" {
        b.WriteString(m.spinner.View())
        b.WriteString(" ")
        b.WriteString(m.activeTool)
        b.WriteString("\n")
    }

    // 3. Permission prompt (if pending)
    if m.permissionPending != nil {
        b.WriteString(m.renderPermissionPrompt())
        b.WriteString("\n")
    }

    // 4. Ask-user prompt (if pending)
    if m.askUserPending != nil {
        b.WriteString(m.renderAskUserPrompt())
        b.WriteString("\n")
    }

    // 5. Todo list (always visible if non-empty)
    if len(m.todos) > 0 {
        b.WriteString(m.renderTodoList())
        b.WriteString("\n")
    }

    // 6. Input prompt (if waiting for user input)
    if m.mode == modeInput {
        b.WriteString(m.textInput.View())
        b.WriteString("\n")
    }

    // 7. Status bar
    b.WriteString(m.renderStatusBar())

    return b.String()
}
```

Only the sections relevant to the current state are rendered. During
streaming, sections 1 + maybe 5 + 7 are active. During input, sections
5 + 6 + 7 are active. During a permission prompt, sections 3 + 5 + 7.

---

## 8. Program Options

Initialize the BT program with these options:

```go
p := tea.NewProgram(
    initialModel,
    tea.WithMouseCellMotion(),    // optional: enable mouse for scrolling
    tea.WithBracketedPaste(),     // handle pasted text correctly (multi-line)
)
```

Do **not** pass:
- `tea.WithAltScreen()` — destroys scrollback
- `tea.WithoutRenderer()` — disables all rendering (only for daemon/print mode)

For print mode (`-p`), do not create a BT program at all. Use the existing
`PrintStreamHandler` directly, no BT involvement.

---

## 9. Key Bindings and Input

### Multi-line input

Use `bubbles/textarea` for the input editor. Configure:
- Enter: submit (send message)
- Shift+Enter or Alt+Enter: newline
- Ctrl+C: cancel current operation or exit
- Tab: slash command completion (when input starts with `/`)
- Up/Down: navigate input history

### Ctrl+C handling

BT sends `tea.KeyMsg{Type: tea.KeyCtrlC}`. Handle contextually:

| State | Ctrl+C behavior |
|-------|----------------|
| Input idle | Quit the program |
| Streaming | Cancel the API call (cancel context) |
| Tool executing | Cancel the tool (cancel context) |
| Permission prompt | Deny permission |
| Ask-user prompt | Cancel (return empty answer) |

Use a `context.WithCancel` for the agentic loop. On Ctrl+C during
streaming/tool execution, cancel the context. The loop and tools check
`ctx.Done()` and abort. Send a `LoopCancelledMsg` to reset the UI to input
mode.

---

## 10. Dependencies

```
github.com/charmbracelet/bubbletea    v1.x   - framework
github.com/charmbracelet/bubbles      v0.x   - textarea, spinner, viewport
github.com/charmbracelet/lipgloss     v1.x   - styling (colors, borders)
github.com/charmbracelet/glamour      v0.x   - markdown rendering
```

All four are from the Charm ecosystem and work together seamlessly. Pin to
the latest stable versions at the time of implementation.

---

## 11. Migration Checklist

These are the concrete changes to make, in order:

### Step 1: Add dependencies

```
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/bubbles@latest
go get github.com/charmbracelet/lipgloss@latest
go get github.com/charmbracelet/glamour@latest
```

### Step 2: Define message types

Create `internal/tui/msg.go` with all `tea.Msg` types (TextDeltaMsg,
ToolStartMsg, PermissionRequestMsg, etc.). These are the protocol between
the agentic loop goroutine and the BT main loop.

### Step 3: Implement the TUI stream handler

Create `internal/tui/stream.go` implementing `api.StreamHandler`. Each
method calls `program.Send(SomeMsg{...})`. This replaces
`ToolAwareStreamHandler`.

### Step 4: Implement the TUI permission handler

Create `internal/tui/permission.go` implementing `tools.PermissionHandler`.
Uses the channel handshake pattern. This replaces
`TerminalPermissionHandler` in TUI mode.

### Step 5: Create the BT model

Create `internal/tui/model.go` with the main model struct, `Init`,
`Update`, and `View`. Start with just text streaming + input — no fancy
components yet.

### Step 6: Wire into main.go

Replace the REPL loop in `main.go` (lines 248–311) with BT program
creation and `program.Run()`. The tool registry, loop config, and
session management stay the same — only the handler and permission handler
change.

### Step 7: Update interactive tools

Modify `TodoWriteTool` and `AskUserTool` to accept `*tea.Program` and
use `program.Send()` instead of `fmt.Printf` / `bufio.Reader`. Keep the
nil-program fallback for print mode.

### Step 8: Add components incrementally

- Spinner (use `bubbles/spinner`)
- Markdown rendering (glamour)
- Diff display (lipgloss styled)
- Slash command completion
- Status bar
- Input history

Each component can be added and tested independently.

---

## 12. Potential Issues and Mitigations

### Issue: View exceeds terminal height during long streaming responses

**Mitigation:** BT's standard renderer handles this — lines scroll off the
top naturally. The user sees the tail of the streaming text, which is the
expected UX. If artifacts occur, implement progressive flushing (Println
completed markdown blocks, keep only the trailing incomplete block in View).

### Issue: Glamour re-render cost on every text delta

**Mitigation:** Glamour is fast (sub-millisecond for typical responses). If
profiling shows it's a bottleneck, throttle re-renders to every N
milliseconds using BT's `tea.Tick` and batch deltas.

### Issue: Race conditions between Send and program lifecycle

**Mitigation:** `program.Send()` is safe to call from any goroutine and
handles the case where the program hasn't started or has already quit (it
uses a select with the program's done channel). No additional
synchronization needed.

### Issue: Terminal resize during streaming

**Mitigation:** BT sends `tea.WindowSizeMsg` on resize. Update the glamour
renderer width and the lipgloss max width in the Update handler. The next
View call uses the new dimensions.

### Issue: Print mode (-p) must work without BT

**Mitigation:** The BT program is only created in interactive mode. Print
mode continues using `PrintStreamHandler` and `AlwaysAllowPermissionHandler`
(or `TerminalPermissionHandler` if permissions are enabled). Tools check for
nil program and fall back to `fmt.Printf`. No BT code is exercised in print
mode.
