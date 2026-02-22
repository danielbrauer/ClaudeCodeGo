# Phase 4: Remaining Tools — Integration Guide

Phase 4 completes the tool set. Every new tool is a file in `internal/tools/` that implements the `tools.Tool` interface and gets registered in `main.go`.

**The TypeScript definitions in `claude-code-source/sdk-tools.d.ts` are the ground truth for input/output schemas.** When this document and the TS types disagree, the TS types win.

---

## What exists today

### The Tool interface (`internal/tools/registry.go`)

Every tool implements this:

```go
type Tool interface {
    Name() string                                              // e.g. "Agent", "TodoWrite"
    Description() string                                       // sent to the API
    InputSchema() json.RawMessage                              // JSON Schema for input
    Execute(ctx context.Context, input json.RawMessage) (string, error)
    RequiresPermission(input json.RawMessage) bool
}
```

`Execute` returns a **string** that becomes the `tool_result` content sent back to the API. For simple tools this is plain text. For tools with structured output (like `AgentOutput`), the string should be JSON that the model can parse.

### Registry and wiring (`internal/tools/registry.go`, `cmd/claude/main.go`)

Tools are registered in `main.go` lines 139-149:

```go
registry := tools.NewRegistry(permHandler)
registry.Register(tools.NewBashTool(cwd))
registry.Register(tools.NewFileReadTool())
// ... etc
```

Phase 4 adds more `registry.Register(...)` calls here. The registry handles permission checks automatically — if `RequiresPermission()` returns true, it calls the `PermissionHandler` before executing.

### The agentic loop (`internal/conversation/loop.go`)

The loop dispatches tool calls via the `ToolExecutor` interface:

```go
type ToolExecutor interface {
    Execute(ctx context.Context, name string, input []byte) (string, error)
    HasTool(name string) bool
}
```

`tools.Registry` implements this. When the API responds with `stop_reason: "tool_use"`, the loop iterates over `tool_use` content blocks, calls `registry.Execute(ctx, block.Name, block.Input)`, and collects the results. **Phase 4 tools don't need to modify this loop — they just need to be registered.**

### Stream handler (`internal/conversation/loop.go`)

`ToolAwareStreamHandler` displays `[tool: <name>] <summary>` during streaming. The `toolInputSummary` function (line 244) has cases for existing tools. **Add cases for new tools** so the user sees meaningful output during execution:

```go
case "Agent":
    if s := extractString("description"); s != "" {
        return s
    }
case "TodoWrite":
    return "updating task list"
case "WebFetch":
    if s := extractString("url"); s != "" {
        return s
    }
// etc.
```

### API client (`internal/api/client.go`)

The `api.Client` is needed by the Agent tool to spawn sub-agents. It's currently constructed in `main.go` and passed into `LoopConfig`. The Agent tool needs access to the client, the system prompt, and the tool registry to create child loops. There are two clean ways to do this:

1. **Pass dependencies into the tool constructor** — `NewAgentTool(client, system, registry)`
2. **Define a LoopFactory** — a function the Agent tool calls to create sub-loops

Option 1 is simpler and matches how `BashTool` already takes `workDir`.

### Session store (`internal/session/session.go`)

The Agent tool may need to create sessions for sub-agents (or not — check `cli.js`). The session store is available in `main.go` but is not currently passed to tools. If sub-agents need their own sessions, you'll need to thread the store through.

### Config types (`internal/config/settings.go`)

Settings are loaded in `main.go` and passed to `BuildSystemPrompt`. The Config tool reads and writes settings at runtime. It needs the CWD (to find project settings) and the home directory (for user settings). It does **not** need access to the merged in-memory settings — it operates on the files directly.

---

## Tool-by-tool guide

### 1. TodoWrite (`internal/tools/todo.go`)

**What it does:** Manages a structured task list displayed to the user. The model calls it to track progress on multi-step tasks.

**Input schema (from `sdk-tools.d.ts`):**
```typescript
{
  todos: {
    content: string;          // imperative: "Run tests"
    status: "pending" | "in_progress" | "completed";
    activeForm: string;       // continuous: "Running tests"
  }[];
}
```

**Implementation:** This is purely in-memory state with terminal display. No file I/O needed.

```go
type TodoWriteTool struct {
    mu    sync.Mutex
    todos []TodoItem
}

type TodoItem struct {
    Content    string `json:"content"`
    Status     string `json:"status"`
    ActiveForm string `json:"activeForm"`
}
```

`Execute` replaces the todo list and returns JSON with `oldTodos` and `newTodos`. The tool also prints a formatted task list to the terminal (the stream handler won't see this — the tool itself should print).

**RequiresPermission:** false.

**Wiring:** `registry.Register(tools.NewTodoWriteTool())`

---

### 2. AskUserQuestion (`internal/tools/askuser.go`)

**What it does:** Presents structured questions with options to the user via the terminal. The model uses this to get decisions.

**Input schema (from `sdk-tools.d.ts`):**
```typescript
{
  questions: [{
    question: string;       // "Which library for date formatting?"
    header: string;         // max 12 chars, e.g. "Library"
    options: [{
      label: string;        // "date-fns"
      description: string;  // "Lightweight, tree-shakeable"
    }, ...];                // 2-4 options
    multiSelect: boolean;
  }, ...];                  // 1-4 questions
}
```

**Implementation:** Read user input from stdin (similar to `TerminalPermissionHandler`). Display each question, number the options, read the selection. An "Other" option is always appended automatically.

```go
type AskUserTool struct {
    reader *bufio.Reader
}
```

**Output:** JSON with `questions` (echo back) and `answers` (map of question text → selected label or free-text).

**RequiresPermission:** false (user interaction IS the permission).

**Wiring:** `registry.Register(tools.NewAskUserTool())`

---

### 3. Agent (`internal/tools/agent.go`)

**What it does:** Spawns a sub-agent with its own isolated conversation loop. The parent agent delegates tasks to child agents. This is the most complex Phase 4 tool.

**Input schema (from `sdk-tools.d.ts`):**
```typescript
{
  description: string;          // "Search for auth code"
  prompt: string;               // full task description
  subagent_type: string;        // agent type identifier
  model?: "sonnet" | "opus" | "haiku";
  resume?: string;              // agent ID to resume
  run_in_background?: boolean;
  max_turns?: number;
}
```

**Implementation:**

The Agent tool creates a **new `conversation.Loop`** with its own `History`, the same system prompt and tool definitions, and runs it to completion (or until `max_turns`). Key design:

```go
type AgentTool struct {
    client   *api.Client
    system   []api.SystemBlock
    tools    []api.ToolDefinition
    toolExec conversation.ToolExecutor
    mu       sync.Mutex
    agents   map[string]*agentState  // track running/completed agents
}

type agentState struct {
    id       string
    loop     *conversation.Loop
    done     chan struct{}
    result   string
    err      error
}
```

Constructor: `NewAgentTool(client, system, toolDefs, toolExec)`

**Synchronous execution:** Create a loop, call `loop.SendMessage(ctx, prompt)`, collect the final text response from history, return it as the tool result.

**Background execution (`run_in_background: true`):** Launch a goroutine, return immediately with an `async_launched` result containing the agent ID and an output file path. The TaskOutput tool reads the result later.

**Resume (`resume: "<id>"`):** Look up the agent by ID in `agents`, re-use its loop (which still has the history), send another message.

**Output:** JSON with `agentId`, `content` (text blocks), `totalToolUseCount`, `totalDurationMs`, `totalTokens`, `usage`, `status`.

**RequiresPermission:** false (sub-agents inherit the parent's permission handler).

**Wiring:** This tool needs the client, system prompt, tool definitions, and tool executor. In `main.go`:

```go
agentTool := tools.NewAgentTool(client, system, registry.Definitions(), registry, bgStore)
registry.Register(agentTool)
```

**Note:** Registering the agent tool AFTER other tools means `registry.Definitions()` won't include the Agent tool itself in its own sub-loops. This is correct — sub-agents should not recursively spawn more agents (or if they should, pass the full definitions including Agent). Check `cli.js` to confirm whether sub-agents have access to the Agent tool.

---

### 4. TaskOutput (`internal/tools/taskoutput.go`)

**What it does:** Reads the output of a background agent or command.

**Input schema:**
```typescript
{
  task_id: string;
  block: boolean;     // whether to wait for completion
  timeout: number;    // max wait time in ms
}
```

**Implementation:** Looks up the agent by ID in the `AgentTool`'s state map. If `block` is true, waits on the agent's `done` channel (with timeout). Returns the agent's result.

This tool needs a reference to the agent registry. Either:
- Make it a method on `AgentTool` itself (not clean for the Tool interface)
- Have a shared `BackgroundTaskStore` that both `AgentTool` and `TaskOutput` reference

```go
type BackgroundTaskStore struct {
    mu    sync.Mutex
    tasks map[string]*BackgroundTask
}

type BackgroundTask struct {
    ID     string
    Done   chan struct{}
    Result string
    Err    error
}
```

Both `AgentTool` and `TaskOutputTool` take a `*BackgroundTaskStore` in their constructors.

**RequiresPermission:** false.

---

### 5. TaskStop (`internal/tools/taskstop.go`)

**What it does:** Stops a running background task.

**Input schema:**
```typescript
{
  task_id?: string;
  shell_id?: string;   // deprecated alias for task_id
}
```

**Implementation:** Looks up the task in `BackgroundTaskStore`, cancels its context, waits briefly for cleanup.

**RequiresPermission:** false.

---

### 6. WebFetch (`internal/tools/webfetch.go`)

**What it does:** Fetches a URL, converts HTML to markdown, and processes it with a prompt using the API.

**Input schema:**
```typescript
{
  url: string;
  prompt: string;
}
```

**Implementation:**

1. HTTP GET the URL (follow redirects, enforce HTTPS upgrade)
2. Convert HTML to plain text or markdown (use a library or write a basic tag stripper)
3. Send the content + prompt to the API as a separate (non-streaming) call
4. Return the model's response

```go
type WebFetchTool struct {
    client     *api.Client
    httpClient *http.Client
}
```

The tool needs `api.Client` to process the fetched content. Use a dedicated HTTP client with reasonable timeouts and a 15-minute cache (per the CLI's spec).

**Output:** JSON with `bytes`, `code`, `codeText`, `result`, `durationMs`, `url`.

**RequiresPermission:** true (network access). Add a case to `extractMatchValue` in `internal/config/permissions.go` if not already there (it is — WebFetch is already handled).

**Wiring:** `registry.Register(tools.NewWebFetchTool(client))`

---

### 7. WebSearch (`internal/tools/websearch.go`)

**What it does:** Searches the web and returns results. In the official CLI, this is a server-side tool — the API itself performs the search. Check `cli.js` to confirm whether this is a client-side HTTP call or a server-side tool_use that the API handles natively.

**Input schema:**
```typescript
{
  query: string;
  allowed_domains?: string[];
  blocked_domains?: string[];
}
```

**If server-side:** The tool is a passthrough. `Execute` just returns an error or placeholder saying the API handles it. The tool definition still needs to be sent so the model knows it's available, but execution may never happen client-side. Search `cli.js` for `web_search`, `server_tool`, `server-tool` to understand the mechanism.

**If client-side:** Use a search API (the official CLI likely uses an internal Anthropic endpoint). The output schema has structured results with titles and URLs.

**RequiresPermission:** false (read-only, no local side effects).

---

### 8. NotebookEdit (`internal/tools/notebook.go`)

**What it does:** Edits Jupyter notebook (.ipynb) cells. Notebooks are JSON files with a specific structure.

**Input schema:**
```typescript
{
  notebook_path: string;     // absolute path
  cell_id?: string;          // target cell
  new_source: string;        // new cell content
  cell_type?: "code" | "markdown";
  edit_mode?: "replace" | "insert" | "delete";
}
```

**Implementation:**

1. Read the .ipynb file (it's JSON)
2. Parse the notebook structure: `{ cells: [{ id, cell_type, source, ... }], metadata, ... }`
3. Find the target cell by `cell_id` (or by index)
4. Apply the edit (replace source, insert new cell, or delete cell)
5. Write the file back

```go
type NotebookEditTool struct{}

type Notebook struct {
    Cells         []NotebookCell         `json:"cells"`
    Metadata      map[string]interface{} `json:"metadata"`
    NBFormat      int                    `json:"nbformat"`
    NBFormatMinor int                    `json:"nbformat_minor"`
}

type NotebookCell struct {
    ID       string                 `json:"id,omitempty"`
    CellType string                 `json:"cell_type"`
    Source   interface{}            `json:"source"`  // string or []string
    Metadata map[string]interface{} `json:"metadata,omitempty"`
    Outputs  []interface{}          `json:"outputs,omitempty"`
}
```

**Note:** The `source` field in .ipynb can be a single string or an array of strings (one per line). Handle both when reading; write back in the same format the file originally used.

**Output:** JSON with `new_source`, `cell_id`, `cell_type`, `language`, `edit_mode`, `notebook_path`, `original_file`, `updated_file`.

**RequiresPermission:** true (writes to disk).

---

### 9. ExitPlanMode (`internal/tools/planmode.go`)

**What it does:** Signals that the model has finished writing a plan and is ready for user approval. This is a coordination tool, not a file operation.

**Input schema:**
```typescript
{
  allowedPrompts?: {
    tool: "Bash";
    prompt: string;   // "run tests", "install dependencies"
  }[];
}
```

**Implementation:** Minimal — the tool returns a message indicating the plan is ready for review. The TUI (Phase 5) will handle the actual approval UI. For now, just return confirmation text.

**RequiresPermission:** false.

---

### 10. Config (`internal/tools/config_tool.go`)

**What it does:** Gets or sets configuration values at runtime.

**Input schema:**
```typescript
{
  setting: string;               // "model", "theme", "permissions.defaultMode"
  value?: string | boolean | number;  // omit to get
}
```

**Implementation:**

- **Get:** Read the appropriate settings file and extract the key
- **Set:** Read the file, modify the key, write it back

The tool operates on `~/.claude/settings.json` (user level) by default. It uses `config.loadSettingsFile` and `config.LoadSettings` from the existing config package. You may want to export `loadSettingsFile` (currently unexported) or add a helper.

```go
type ConfigTool struct {
    cwd string
}
```

**Output:** JSON with `success`, `operation`, `setting`, `value`/`previousValue`/`newValue`.

**RequiresPermission:** false.

---

### 11. EnterWorktree (`internal/tools/worktree.go`)

**What it does:** Creates an isolated git worktree so an agent can work on a separate copy of the repo without affecting the main working directory.

**Input schema:**
```typescript
{
  name?: string;   // optional worktree name
}
```

**Implementation:**

1. Verify the CWD is a git repo (`git rev-parse --git-dir`)
2. Generate a worktree name if not provided
3. Create the worktree (`git worktree add <path> -b <branch>`)
4. Return the worktree path and branch name

```go
type WorktreeTool struct {
    workDir string
}
```

**Output:** JSON with `worktreePath`, `worktreeBranch`, `message`.

**RequiresPermission:** true (creates files and git branches).

---

### 12. FileRead extensions

The existing `FileReadTool` (`internal/tools/fileread.go`) handles text files. Phase 4 adds:

- **Images** (PNG, JPG, etc.) — return base64-encoded content as an image content block
- **PDFs** — extract text from page ranges using the `pages` input field
- **Jupyter notebooks** — render all cells with outputs

These are extensions to the existing `FileReadTool.Execute`, not new tools. Add detection logic based on file extension:

```go
func (t *FileReadTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
    // ... existing parsing ...

    ext := strings.ToLower(filepath.Ext(in.FilePath))
    switch ext {
    case ".png", ".jpg", ".jpeg", ".gif", ".webp":
        return t.readImage(in.FilePath)
    case ".pdf":
        return t.readPDF(in.FilePath, in.Pages)
    case ".ipynb":
        return t.readNotebook(in.FilePath)
    default:
        // existing text file logic
    }
}
```

For images, the result needs to be an image content block that the API understands. Since `Execute` returns a string, you may need to encode the image data as a base64 string with a prefix the model recognizes, or reconsider whether the tool result format needs to support structured content blocks (check how `cli.js` returns image data from FileRead).

For PDFs, consider using an external tool (`pdftotext`) via `exec.Command` or a Go library.

---

### 13. Git checkpoint system

Before file modifications (FileEdit, FileWrite), create automatic git snapshots so changes can be reverted.

**Implementation approach:**

Rather than a separate tool, this is a wrapper around FileEdit and FileWrite. Before executing, check if the target file is in a git repo and stash/commit the current state:

```go
type CheckpointingFileEditTool struct {
    inner   *FileEditTool
    workDir string
}

func (t *CheckpointingFileEditTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
    // 1. Parse input to get file_path
    // 2. If file is in a git repo, create a checkpoint
    // 3. Delegate to t.inner.Execute(ctx, input)
    // 4. Return result
}
```

Or add checkpoint logic directly to FileEdit/FileWrite. Search `cli.js` for `checkpoint`, `snapshot`, `stash` to understand the exact mechanism (stash? lightweight commit on a temp branch? reflog?).

---

## Shared infrastructure

### BackgroundTaskStore

The Agent, TaskOutput, and TaskStop tools share state about background tasks. Create a shared store:

```go
// internal/tools/background.go
type BackgroundTaskStore struct {
    mu    sync.Mutex
    tasks map[string]*BackgroundTask
}

type BackgroundTask struct {
    ID         string
    Ctx        context.Context
    Cancel     context.CancelFunc
    Done       chan struct{}
    Result     string
    Err        error
    OutputFile string
}

func NewBackgroundTaskStore() *BackgroundTaskStore
func (s *BackgroundTaskStore) Add(task *BackgroundTask)
func (s *BackgroundTaskStore) Get(id string) (*BackgroundTask, bool)
func (s *BackgroundTaskStore) Remove(id string)
```

Wire it in `main.go`:

```go
bgStore := tools.NewBackgroundTaskStore()
registry.Register(tools.NewAgentTool(client, system, registry.Definitions(), registry, bgStore))
registry.Register(tools.NewTaskOutputTool(bgStore))
registry.Register(tools.NewTaskStopTool(bgStore))
```

---

## Changes to existing files

| File | Change |
|------|--------|
| `internal/tools/todo.go` | **New.** TodoWrite tool. |
| `internal/tools/askuser.go` | **New.** AskUserQuestion tool. |
| `internal/tools/agent.go` | **New.** Agent/Task tool with sub-loop spawning. |
| `internal/tools/taskoutput.go` | **New.** TaskOutput tool. |
| `internal/tools/taskstop.go` | **New.** TaskStop tool. |
| `internal/tools/webfetch.go` | **New.** WebFetch tool. |
| `internal/tools/websearch.go` | **New.** WebSearch tool. |
| `internal/tools/notebook.go` | **New.** NotebookEdit tool. |
| `internal/tools/planmode.go` | **New.** ExitPlanMode tool. |
| `internal/tools/config_tool.go` | **New.** Config tool. |
| `internal/tools/worktree.go` | **New.** EnterWorktree tool. |
| `internal/tools/background.go` | **New.** BackgroundTaskStore shared by Agent/TaskOutput/TaskStop. |
| `internal/tools/fileread.go` | **Modify.** Add image, PDF, and notebook reading. |
| `internal/conversation/loop.go` | **Modify.** Add summary cases in `toolInputSummary` for new tools. |
| `cmd/claude/main.go` | **Modify.** Register all new tools (lines 139-149 expand significantly). |

---

## Registration order in main.go

After Phase 4, the registration block should look like:

```go
bgStore := tools.NewBackgroundTaskStore()

registry := tools.NewRegistry(permHandler)
registry.Register(tools.NewBashToolWithEnv(cwd, settings.Env))
registry.Register(tools.NewFileReadTool())
registry.Register(tools.NewFileEditTool())
registry.Register(tools.NewFileWriteTool())
registry.Register(tools.NewGlobTool(cwd))
registry.Register(tools.NewGrepTool(cwd))
registry.Register(tools.NewTodoWriteTool())
registry.Register(tools.NewAskUserTool())
registry.Register(tools.NewWebFetchTool(client))
registry.Register(tools.NewWebSearchTool())
registry.Register(tools.NewNotebookEditTool())
registry.Register(tools.NewConfigTool(cwd))
registry.Register(tools.NewWorktreeTool(cwd))
registry.Register(tools.NewExitPlanModeTool())
registry.Register(tools.NewTaskOutputTool(bgStore))
registry.Register(tools.NewTaskStopTool(bgStore))

// Agent tool registered last — gets tool definitions that include everything above.
agentTool := tools.NewAgentTool(client, system, registry.Definitions(), registry, bgStore)
registry.Register(agentTool)
```

---

## How to verify

For each tool, compare against `sdk-tools.d.ts`:

1. **Input schema** — the JSON Schema returned by `InputSchema()` must match the TypeScript types exactly (field names, types, required fields, enums, constraints).

2. **Output format** — call each tool and verify the response string is parseable as the expected output type. The model relies on this structure.

3. **Integration** — verify that the agentic loop correctly dispatches to each new tool, passes input, and threads results back into the conversation.

4. **Agent tool** — this is the hardest to test. Verify that sub-agents get their own history, can use all tools, and return results to the parent. Test background execution and resume.

When in doubt, search `cli.js` for the tool name and study the execution logic.
