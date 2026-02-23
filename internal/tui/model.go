package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/config"
	"github.com/anthropics/claude-code-go/internal/conversation"
	"github.com/anthropics/claude-code-go/internal/session"
	"github.com/anthropics/claude-code-go/internal/skills"
	"github.com/anthropics/claude-code-go/internal/tools"
)

// UI mode determines what the live region shows.
type uiMode int

const (
	modeInput      uiMode = iota // waiting for user text
	modeStreaming                 // receiving API response
	modePermission               // waiting for permission y/n
	modeAskUser                  // waiting for ask-user response
	modeResume                   // session picker for /resume
	modeModelPicker              // choosing a model via /model
	modeDiff                     // viewing diff dialog
	modeConfig                   // config panel open
	modeHelp                     // viewing help screen
)

// model is the Bubble Tea model for the TUI.
type model struct {
	// Core references.
	loop      *conversation.Loop
	ctx       context.Context
	cancelFn  context.CancelFunc
	modelName string
	version   string
	mcpStatus MCPStatus   // MCP manager for /mcp command; may be nil
	apiClient *api.Client // API client for model switching

	// UI state.
	mode          uiMode
	width, height int
	textInput     textarea.Model
	spinner       spinner.Model
	mdRenderer    *markdownRenderer
	slashReg      *slashRegistry

	// Streaming state.
	streamingText string // accumulated markdown text during streaming
	activeTool    string // name of tool currently executing (shown with spinner)
	toolSummary   string // short description of the active tool call

	// Token tracking.
	tokens tokenTracker

	// Permission prompt.
	permissionPending *PermissionRequestMsg

	// AskUser state.
	askUserPending *tools.AskUserRequestMsg
	askCursor      int    // selected option index
	askAnswers     map[string]string
	askQuestionIdx int    // current question being answered
	askCustomInput bool   // currently typing custom "Other" input
	askCustomText  string // accumulated custom text

	// Model picker state.
	modelPickerCursor int

	// Callback invoked when the model is switched.
	onModelSwitch func(newModel string)

	// Todo list.
	todos []tools.TodoItem

	// Session management.
	sessStore *session.Store   // session persistence store; may be nil
	session   *session.Session // current active session; shared with main.go callback

	// Resume session picker state.
	resumeSessions []*session.Session // loaded session list for picker
	resumeCursor   int                // selected index in session list

	// Auth callbacks.
	logoutFunc func() error // Clears credentials; nil if not available.

	// Diff dialog state.
	diffData     *diffData
	diffSelected int    // selected file index
	diffViewMode string // "list" or "detail"

	// Config panel state.
	configPanel *configPanel
	settings    *config.Settings // reference to live settings

	// Tab completion state for slash commands.
	completions    []string // current fuzzy-matched completions
	completionIdx  int      // selected index in completions (-1 = none)
	completionBase string   // the original typed text (without leading /)

	// Help screen state.
	helpTab       int // 0=general, 1=commands, 2=custom-commands
	helpScrollOff int // scroll offset (first visible line of tab content)

	// Fast mode toggle.
	fastMode bool

	// Command queueing: users can type and submit messages while the agent
	// is busy. These are stored here and automatically sent when the current
	// turn completes.
	queue inputQueue

	// Initial prompt to send on start.
	initialPrompt string

	// Input section.
	promptSuggestion string // cached suggestion text (e.g., `Try "edit app.go to..."`)
	submitCount      int    // number of user messages sent in the session

	// Dynamic prompt suggestion (generated after each turn via API).
	dynSuggestion          string // suggested next prompt text (shown as placeholder)
	dynSuggestionGenerating bool  // true while an API call is in-flight

	// Status line (custom command-based status bar).
	statusLineText string // last output from the status line command

	// Whether we should quit.
	quitting bool

	// Exit action to signal the caller (e.g., re-run login after TUI exits).
	exitAction ExitAction
}

// ModelConfig bundles the parameters for creating a new TUI model.
// This avoids a long parameter list in newModel and makes it easier to add
// new fields without changing the function signature.
type ModelConfig struct {
	Loop          *conversation.Loop
	Ctx           context.Context
	CancelFn      context.CancelFunc
	ModelName     string
	Version       string
	InitialPrompt string
	Width         int
	MCPStatus     MCPStatus
	Skills        []skills.Skill
	SessStore     *session.Store
	Session       *session.Session
	Settings      *config.Settings
	OnModelSwitch func(newModel string)
	LogoutFunc    func() error
	FastMode      bool
}

// newModel creates the initial Bubble Tea model.
func newModel(cfg ModelConfig) model {
	ti := newTextInput(cfg.Width)
	sp := newSpinner()
	md := newMarkdownRenderer(cfg.Width)
	slash := newSlashRegistry()

	// Phase 7: Register skill-based slash commands.
	if len(cfg.Skills) > 0 {
		slash.registerSkills(cfg.Skills)
	}

	return model{
		loop:             cfg.Loop,
		ctx:              cfg.Ctx,
		cancelFn:         cfg.CancelFn,
		modelName:        cfg.ModelName,
		version:          cfg.Version,
		mcpStatus:        cfg.MCPStatus,
		settings:         cfg.Settings,
		onModelSwitch:    cfg.OnModelSwitch,
		mode:             modeInput,
		width:            cfg.Width,
		height:           24,
		textInput:        ti,
		spinner:          sp,
		mdRenderer:       md,
		slashReg:         slash,
		logoutFunc:       cfg.LogoutFunc,
		initialPrompt:    cfg.InitialPrompt,
		sessStore:        cfg.SessStore,
		session:          cfg.Session,
		fastMode:         cfg.FastMode,
		promptSuggestion: generatePromptSuggestion(),
	}
}

// applyFastMode synchronizes fast mode state across the model, settings,
// and conversation loop. It does not persist to disk — the caller is
// responsible for calling config.SaveUserSetting when appropriate.
func applyFastMode(m *model, enabled bool) {
	m.fastMode = enabled
	if m.settings != nil {
		m.settings.FastMode = config.BoolPtr(enabled)
	}
	m.loop.SetFastMode(enabled)

	if enabled && !api.IsOpus46Model(m.modelName) {
		resolved := api.ModelAliases[api.FastModeModelAlias]
		m.modelName = resolved
		m.loop.SetModel(resolved)
	}
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		textarea.Blink,
	}
	// If there's an initial prompt, send it immediately.
	if m.initialPrompt != "" {
		cmds = append(cmds, func() tea.Msg {
			return SubmitInputMsg{Text: m.initialPrompt}
		})
	}
	// Kick off the initial status line refresh.
	if cmd := m.refreshStatusLine(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	// ── Terminal resize ──
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textInput.SetWidth(msg.Width)
		m.mdRenderer.updateWidth(msg.Width)
		return m, nil

	// ── Key events ──
	case tea.KeyMsg:
		return m.handleKey(msg)

	// ── User submits input ──
	case SubmitInputMsg:
		return m.handleSubmit(msg.Text)

	// ── Stream handler messages ──
	case MessageStartMsg:
		m.tokens.addInput(msg.Usage.InputTokens,
			msg.Usage.CacheReadInputTokens, msg.Usage.CacheCreationInputTokens)
		return m, nil

	case ContentBlockStartMsg:
		if msg.Block.Type == api.ContentTypeToolUse {
			m.activeTool = msg.Block.Name
			m.toolSummary = ""
			return m, m.spinner.Tick
		}
		return m, nil

	case TextDeltaMsg:
		m.streamingText += msg.Text
		return m, nil

	case InputJSONDeltaMsg:
		// We don't need to track JSON here (stream handler does it).
		return m, nil

	case ContentBlockStopMsg:
		if msg.Name != "" {
			// Tool call block completed. Print the tool line to scrollback.
			toolLine := renderToolComplete(msg.Name, msg.Input)
			cmds = append(cmds, tea.Println(toolLine))
			m.activeTool = ""
			m.toolSummary = ""
		} else if m.streamingText != "" {
			// Text block completed. Flush to scrollback.
			rendered := m.mdRenderer.render(m.streamingText)
			cmds = append(cmds, tea.Println(rendered))
			m.streamingText = ""
		}
		return m, tea.Batch(cmds...)

	case MessageDeltaMsg:
		if msg.Usage != nil {
			m.tokens.addOutput(msg.Usage.OutputTokens)
		}
		return m, nil

	case MessageStopMsg:
		// Message finished but the loop may continue (tool results → next API call).
		// Don't switch to input mode here; wait for LoopDoneMsg.
		return m, nil

	case StreamErrorMsg:
		errLine := errorStyle.Render("Error: " + msg.Err.Error())
		cmds = append(cmds, tea.Println(errLine))
		return m, tea.Batch(cmds...)

	case LoopDoneMsg:
		// Agentic loop finished. Flush any remaining streaming text.
		if m.streamingText != "" {
			rendered := m.mdRenderer.render(m.streamingText)
			cmds = append(cmds, tea.Println(rendered))
			m.streamingText = ""
		}
		if msg.Err != nil && m.ctx.Err() == nil {
			errLine := errorStyle.Render("Error: " + msg.Err.Error())
			cmds = append(cmds, tea.Println(errLine))
		}
		m.activeTool = ""
		// Clear any previous dynamic suggestion.
		m.dynSuggestion = ""
		// Refresh the custom status line after each assistant turn.
		if cmd := m.refreshStatusLine(); cmd != nil {
			cmds = append(cmds, cmd)
		}

		// If there are queued messages, automatically send the next one
		// instead of returning to input mode.
		if text, ok := m.queue.Dequeue(); ok {
			// Stay in streaming mode and submit the queued message.
			return m.handleSubmit(text)
		}

		m.mode = modeInput
		m.textInput.Focus()
		// Generate a dynamic prompt suggestion for the next turn.
		if m.apiClient != nil && msg.Err == nil && m.ctx.Err() == nil {
			m.dynSuggestionGenerating = true
			msgs := m.loop.History().Messages()
			// Copy messages to avoid races with the main loop.
			msgsCopy := make([]api.Message, len(msgs))
			copy(msgsCopy, msgs)
			cmds = append(cmds, generatePromptSuggestionCmd(m.ctx, m.apiClient, msgsCopy))
		}
		return m, tea.Batch(append(cmds, textarea.Blink)...)

	case promptSuggestionResult:
		m.dynSuggestionGenerating = false
		m.dynSuggestion = msg.text
		return m, nil

	// ── Memory edit done ──
	case MemoryEditDoneMsg:
		var output string
		if msg.Err != nil {
			output = "Editor exited with error: " + msg.Err.Error()
		} else {
			output = editorHintMessage(msg.Path)
		}
		m.mode = modeInput
		m.textInput.Focus()
		return m, tea.Batch(tea.Println(output), textarea.Blink)

	// ── Permission prompt ──
	case PermissionRequestMsg:
		m.permissionPending = &msg
		m.mode = modePermission
		return m, nil

	// ── Status line update ──
	case statusLineUpdateMsg:
		m.statusLineText = msg.Text
		return m, nil

	// ── Todo list update ──
	case tools.TodoUpdateMsg:
		m.todos = msg.Todos
		return m, nil

	// ── AskUser request ──
	case tools.AskUserRequestMsg:
		m.askUserPending = &msg
		m.askCursor = 0
		m.askAnswers = make(map[string]string)
		m.askQuestionIdx = 0
		m.askCustomInput = false
		m.askCustomText = ""
		m.mode = modeAskUser
		return m, nil

	// ── Diff dialog loaded ──
	case DiffLoadedMsg:
		m.diffData = &msg.Data
		m.diffSelected = 0
		m.diffViewMode = "list"
		m.mode = modeDiff
		return m, nil

	// ── Spinner tick ──
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	// Pass other messages to the text input.
	// During both modeInput and modeStreaming, forward to the textarea so
	// users can type ahead while the agent is working.
	if m.mode == modeInput || m.mode == modeStreaming {
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// handleKey processes keyboard input based on the current mode.
func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {

	case modeHelp:
		return m.handleHelpKey(msg)

	case modeResume:
		return m.handleResumeKey(msg)

	case modeConfig:
		return m.handleConfigKey(msg)

	case modePermission:
		return m.handlePermissionKey(msg)

	case modeAskUser:
		return m.handleAskUserKey(msg)

	case modeModelPicker:
		return m.handleModelPickerKey(msg)

	case modeDiff:
		return m.handleDiffKey(msg)

	case modeStreaming:
		return m.handleStreamingKey(msg)

	case modeInput:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit

		case tea.KeyTab:
			// If the input is empty and we have a dynamic suggestion,
			// accept it by filling it into the text input.
			if strings.TrimSpace(m.textInput.Value()) == "" && m.dynSuggestion != "" {
				m.textInput.SetValue(m.dynSuggestion)
				m.textInput.CursorEnd()
				return m, nil
			}
			return m.handleTabComplete()

		case tea.KeyShiftTab:
			return m.handleTabCompletePrev()

		case tea.KeyEscape:
			if len(m.completions) > 0 {
				m.clearCompletions()
				return m, nil
			}
			// Escape clears the dynamic suggestion.
			if m.dynSuggestion != "" {
				m.dynSuggestion = ""
				return m, nil
			}
			return m, nil

		case tea.KeyEnter:
			m.clearCompletions()
			text := strings.TrimSpace(m.textInput.Value())
			// If input is empty but we have a dynamic suggestion,
			// submit the suggestion directly.
			if text == "" && m.dynSuggestion != "" {
				text = m.dynSuggestion
				m.dynSuggestion = ""
				m.textInput.Reset()
				return m.handleSubmit(text)
			}
			if text == "" {
				return m, nil
			}
			m.dynSuggestion = "" // clear suggestion on any submit
			m.textInput.Reset()
			return m.handleSubmit(text)

		default:
			// Open help screen when '?' is pressed with empty input.
			if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == '?' {
				if strings.TrimSpace(m.textInput.Value()) == "" {
					m.helpTab = 0
					m.helpScrollOff = 0
					m.mode = modeHelp
					m.textInput.Blur()
					return m, nil
				}
			}
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			// Clear completions when the user types — they'll re-trigger on Tab.
			if len(m.completions) > 0 {
				m.clearCompletions()
			}
			// Clear the dynamic suggestion once the user starts typing
			// their own text.
			if m.dynSuggestion != "" && m.textInput.Value() != "" {
				m.dynSuggestion = ""
			}
			return m, cmd
		}
	}

	return m, nil
}

// handleStreamingKey processes key events while the agent is working.
// Users can type ahead and press Enter to queue messages for when the
// current turn finishes.
func (m model) handleStreamingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		// Ctrl+C cancels the running loop and clears the queue.
		m.queue.Clear()
		m.cancelFn()
		return m, nil

	case tea.KeyEnter:
		text := strings.TrimSpace(m.textInput.Value())
		if text == "" {
			return m, nil
		}
		m.textInput.Reset()

		// Enqueue the message for processing after the current turn.
		m.queue.Enqueue(text)

		// Echo queued message to scrollback with a "queued" indicator.
		userLine := queuedLabelStyle.Render("> ") + permHintStyle.Render(text) +
			"  " + queuedBadgeStyle.Render("(queued)")
		return m, tea.Println(userLine)

	case tea.KeyEscape:
		// Escape clears the current input being typed during streaming,
		// or removes the last queued message if input is empty.
		if strings.TrimSpace(m.textInput.Value()) != "" {
			m.textInput.Reset()
			return m, nil
		}
		if text, ok := m.queue.RemoveLast(); ok {
			hint := permHintStyle.Render("Removed queued message: " + truncateText(text, 60))
			return m, tea.Println(hint)
		}
		return m, nil

	default:
		// All other keys are forwarded to the textarea by the Update fallthrough.
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}
}

// isExitCommand returns true if the input is a bare exit command.
// The JS CLI recognizes these without a slash prefix.
func isExitCommand(text string) bool {
	switch text {
	case "exit", "quit", ":q", ":q!", ":wq", ":wq!":
		return true
	}
	return false
}

// handleSubmit processes submitted text (user message or slash command).
func (m model) handleSubmit(text string) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	m.submitCount++

	// Echo user input to scrollback.
	userLine := userLabelStyle.Render("> ") + text
	cmds = append(cmds, tea.Println(userLine))

	// Check for bare exit commands (exit, quit, :q, :q!, :wq, :wq!).
	if isExitCommand(text) {
		m.quitting = true
		return m, tea.Batch(append(cmds, tea.Quit)...)
	}

	// Check for slash commands.
	if strings.HasPrefix(text, "/") {
		cmdName := strings.TrimPrefix(text, "/")
		parts := strings.SplitN(cmdName, " ", 2)
		cmdName = parts[0]
		cmdArgs := ""
		if len(parts) > 1 {
			cmdArgs = parts[1]
		}

		// Fuzzy auto-correct: if the command isn't an exact match, try to
		// find the best fuzzy match and silently correct it.
		if _, exact := m.slashReg.lookup(cmdName); !exact {
			if best, ok := m.slashReg.fuzzyBest(cmdName); ok {
				hint := permHintStyle.Render(fmt.Sprintf("  (corrected /%s → /%s)", cmdName, best))
				cmds = append(cmds, tea.Println(hint))
				cmdName = best
			}
		}

		if cmd, ok := m.slashReg.lookup(cmdName); ok && cmd.Execute != nil {
			result, cmdCmd := cmd.Execute(&m, cmdArgs)
			if cmdCmd != nil {
				cmds = append(cmds, cmdCmd)
			}
			return result, tea.Batch(cmds...)
		}

		errMsg := "Unknown command: /" + cmdName + " (type /help for available commands)"
		cmds = append(cmds, tea.Println(errMsg))
		return m, tea.Batch(cmds...)
	}

	// Regular message: send to the agentic loop.
	m.mode = modeStreaming
	m.textInput.Blur()

	loopCmd := func() tea.Msg {
		err := m.loop.SendMessage(m.ctx, text)
		return LoopDoneMsg{Err: err}
	}

	cmds = append(cmds, loopCmd, m.spinner.Tick)
	return m, tea.Batch(cmds...)
}

// handleTabComplete triggers or cycles forward through fuzzy slash command completions.
func (m model) handleTabComplete() (tea.Model, tea.Cmd) {
	text := m.textInput.Value()

	// Only complete when the input starts with "/".
	if !strings.HasPrefix(text, "/") {
		return m, nil
	}

	// Extract the command portion (no leading slash, no args).
	raw := strings.TrimPrefix(text, "/")
	parts := strings.SplitN(raw, " ", 2)
	typed := parts[0]

	// If we already have completions, cycle to the next one.
	if len(m.completions) > 0 && m.completionBase == typed || len(m.completions) > 0 {
		m.completionIdx = (m.completionIdx + 1) % len(m.completions)
		m.applyCompletion()
		return m, nil
	}

	// Build new completions.
	matches := m.slashReg.fuzzyComplete(typed)
	if len(matches) == 0 {
		return m, nil
	}

	m.completionBase = typed
	m.completions = matches
	m.completionIdx = 0
	m.applyCompletion()
	return m, nil
}

// handleTabCompletePrev cycles backward through completions (Shift+Tab).
func (m model) handleTabCompletePrev() (tea.Model, tea.Cmd) {
	if len(m.completions) == 0 {
		// Start fresh same as Tab.
		return m.handleTabComplete()
	}
	m.completionIdx--
	if m.completionIdx < 0 {
		m.completionIdx = len(m.completions) - 1
	}
	m.applyCompletion()
	return m, nil
}

// applyCompletion replaces the text input content with the selected completion.
func (m *model) applyCompletion() {
	if m.completionIdx < 0 || m.completionIdx >= len(m.completions) {
		return
	}
	completed := "/" + m.completions[m.completionIdx]
	m.textInput.Reset()
	m.textInput.SetValue(completed)
	// Move cursor to end.
	m.textInput.CursorEnd()
}

// clearCompletions resets completion state.
func (m *model) clearCompletions() {
	m.completions = nil
	m.completionIdx = -1
	m.completionBase = ""
}

// handlePermissionKey processes key events during a permission prompt.
func (m model) handlePermissionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.permissionPending == nil {
		m.mode = modeInput
		return m, nil
	}

	var cmds []tea.Cmd

	switch msg.String() {
	case "y", "Y":
		m.permissionPending.ResultCh <- PermissionAllow
		line := renderPermissionResultLine(m.permissionPending.ToolName, m.permissionPending.Summary, PermissionAllow)
		cmds = append(cmds, tea.Println(line))
		m.permissionPending = nil
		m.mode = modeStreaming
		return m, tea.Batch(cmds...)

	case "n", "N":
		m.permissionPending.ResultCh <- PermissionDeny
		line := renderPermissionResultLine(m.permissionPending.ToolName, m.permissionPending.Summary, PermissionDeny)
		cmds = append(cmds, tea.Println(line))
		m.permissionPending = nil
		m.mode = modeStreaming
		return m, tea.Batch(cmds...)

	case "a", "A":
		// "Always allow" — only available when suggestions exist.
		if len(m.permissionPending.Suggestions) > 0 {
			m.permissionPending.ResultCh <- PermissionAlwaysAllow
			line := renderPermissionResultLine(m.permissionPending.ToolName, m.permissionPending.Summary, PermissionAlwaysAllow)
			cmds = append(cmds, tea.Println(line))
			m.permissionPending = nil
			m.mode = modeStreaming
			return m, tea.Batch(cmds...)
		}

	case "ctrl+c":
		m.permissionPending.ResultCh <- PermissionDeny
		m.permissionPending = nil
		m.cancelFn()
		return m, nil
	}

	return m, nil
}

// handleAskUserKey processes key events during an ask-user prompt.
func (m model) handleAskUserKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.askUserPending == nil || len(m.askUserPending.Questions) == 0 {
		m.mode = modeStreaming
		return m, nil
	}

	q := m.askUserPending.Questions[m.askQuestionIdx]
	numOptions := len(q.Options) + 1 // +1 for "Other"

	if m.askCustomInput {
		// Typing custom text for "Other" option.
		switch msg.Type {
		case tea.KeyEnter:
			m.askAnswers[q.Question] = m.askCustomText
			m.askCustomInput = false
			m.askCustomText = ""
			return m.advanceAskUser()
		case tea.KeyBackspace:
			if len(m.askCustomText) > 0 {
				m.askCustomText = m.askCustomText[:len(m.askCustomText)-1]
			}
			return m, nil
		case tea.KeyCtrlC:
			m.askUserPending.ResponseCh <- m.askAnswers
			m.askUserPending = nil
			m.mode = modeStreaming
			return m, nil
		default:
			if msg.Type == tea.KeyRunes {
				m.askCustomText += string(msg.Runes)
			} else if msg.Type == tea.KeySpace {
				m.askCustomText += " "
			}
			return m, nil
		}
	}

	switch msg.Type {
	case tea.KeyUp:
		if m.askCursor > 0 {
			m.askCursor--
		}
		return m, nil

	case tea.KeyDown:
		if m.askCursor < numOptions-1 {
			m.askCursor++
		}
		return m, nil

	case tea.KeyEnter:
		if m.askCursor == numOptions-1 {
			// "Other" selected: switch to custom text input.
			m.askCustomInput = true
			m.askCustomText = ""
			return m, nil
		}
		// Regular option selected.
		m.askAnswers[q.Question] = q.Options[m.askCursor].Label
		return m.advanceAskUser()

	case tea.KeyCtrlC:
		// Cancel: send whatever answers we have.
		m.askUserPending.ResponseCh <- m.askAnswers
		m.askUserPending = nil
		m.mode = modeStreaming
		return m, nil
	}

	return m, nil
}

// advanceAskUser moves to the next question or completes the ask-user flow.
func (m model) advanceAskUser() (tea.Model, tea.Cmd) {
	m.askQuestionIdx++
	m.askCursor = 0

	if m.askQuestionIdx >= len(m.askUserPending.Questions) {
		// All questions answered. Print summary to scrollback.
		var lines []string
		for _, q := range m.askUserPending.Questions {
			answer := m.askAnswers[q.Question]
			lines = append(lines, askHeaderStyle.Render("["+q.Header+"]")+" "+q.Question+" "+askSelectedStyle.Render(answer))
		}
		m.askUserPending.ResponseCh <- m.askAnswers
		m.askUserPending = nil
		m.mode = modeStreaming

		var cmds []tea.Cmd
		for _, line := range lines {
			cmds = append(cmds, tea.Println(line))
		}
		return m, tea.Batch(cmds...)
	}

	return m, nil
}

// handleDiffKey processes key events in the diff dialog.
func (m model) handleDiffKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.diffData == nil {
		m.mode = modeInput
		m.textInput.Focus()
		return m, textarea.Blink
	}

	switch msg.Type {
	case tea.KeyEscape, tea.KeyCtrlC:
		// Close diff dialog.
		m.diffData = nil
		m.mode = modeInput
		m.textInput.Focus()
		return m, textarea.Blink

	case tea.KeyUp:
		if m.diffViewMode == "list" && m.diffSelected > 0 {
			m.diffSelected--
		}
		return m, nil

	case tea.KeyDown:
		if m.diffViewMode == "list" && m.diffSelected < len(m.diffData.files)-1 {
			m.diffSelected++
		}
		return m, nil

	case tea.KeyEnter:
		if m.diffViewMode == "list" && m.diffSelected < len(m.diffData.files) {
			m.diffViewMode = "detail"
		}
		return m, nil

	case tea.KeyLeft:
		if m.diffViewMode == "detail" {
			m.diffViewMode = "list"
		}
		return m, nil

	default:
		// Also handle 'q' to close.
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == 'q' {
			m.diffData = nil
			m.mode = modeInput
			m.textInput.Focus()
			return m, textarea.Blink
		}
		return m, nil
	}
}

// View renders the live region of the TUI.
func (m model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// 0a. Help screen (takes over the entire view).
	if m.mode == modeHelp {
		b.WriteString(m.renderHelpScreen())
		return b.String()
	}

	// 0b. Diff dialog (takes over the entire view).
	if m.mode == modeDiff && m.diffData != nil {
		b.WriteString(renderDiffView(m.diffData, m.diffSelected, m.diffViewMode, m.width))
		return b.String()
	}

	// Also show a loading indicator while diff is loading.
	if m.mode == modeDiff && m.diffData == nil {
		b.WriteString(m.spinner.View() + " Loading diff...\n")
		return b.String()
	}

	// 1. Streaming text (during API response).
	if m.streamingText != "" {
		rendered := m.mdRenderer.render(m.streamingText)
		b.WriteString(rendered)
		b.WriteString("\n")
	}

	// 2. Active tool spinner.
	if m.activeTool != "" {
		b.WriteString(m.spinner.View())
		b.WriteString(" ")
		b.WriteString(toolNameStyle.Render(m.activeTool))
		if m.toolSummary != "" {
			b.WriteString("  " + toolSummaryStyle.Render(m.toolSummary))
		}
		b.WriteString("\n")
	} else if m.mode == modeStreaming && m.streamingText == "" {
		// Show a general "thinking" spinner when waiting for the API.
		b.WriteString(m.spinner.View())
		b.WriteString(" Thinking...\n")
	}

	// 3. Config panel.
	if m.mode == modeConfig && m.configPanel != nil {
		b.WriteString(m.renderConfigPanel())
		b.WriteString("\n")
		// Status bar.
		b.WriteString(renderStatusBar(m.modelName, &m.tokens, m.width, m.fastMode))
		return b.String()
	}

	// 4. Permission prompt.
	if m.permissionPending != nil {
		b.WriteString(renderPermissionPrompt(m.permissionPending.ToolName, m.permissionPending.Summary, m.permissionPending.Suggestions))
		b.WriteString("\n")
	}

	// 4. AskUser prompt.
	if m.askUserPending != nil && m.askQuestionIdx < len(m.askUserPending.Questions) {
		b.WriteString(m.renderAskUserPrompt())
		b.WriteString("\n")
	}

	// 5. Resume session picker.
	if m.mode == modeResume && len(m.resumeSessions) > 0 {
		b.WriteString(m.renderResumePicker())
  }
  
	// 5. Model picker.
	if m.mode == modeModelPicker {
		b.WriteString(m.renderModelPicker())
		b.WriteString("\n")
	}

	// 6. Todo list.
	if len(m.todos) > 0 {
		b.WriteString(renderTodoList(m.todos))
		b.WriteString("\n")
	}

	// 7. Completion suggestions (shown above the input).
	if m.mode == modeInput && len(m.completions) > 0 {
		b.WriteString(m.renderCompletions())
	}

	// 8. Input area with borders.
	if m.mode == modeInput || (m.mode == modeStreaming && m.textInput.Value() != "") {
		// Top border.
		b.WriteString(renderInputBorder(m.width))
		b.WriteString("\n")

		if m.mode == modeInput {
			// Set placeholder dynamically: show a suggestion when the input
			// field is blank. Prioritize dynamic suggestions, then static
			// template suggestions before the first submit.
			if m.textInput.Value() == "" {
				if m.dynSuggestion != "" {
					m.textInput.Placeholder = m.dynSuggestion
				} else if m.submitCount < 1 {
					if m.queue.Len() > 0 {
						m.textInput.Placeholder = "Press Esc to remove queued messages"
					} else {
						m.textInput.Placeholder = m.promptSuggestion
					}
				} else {
					m.textInput.Placeholder = ""
				}
			} else {
				m.textInput.Placeholder = ""
			}
		} else {
			// Streaming mode — show a hint that input will be queued.
			m.textInput.Placeholder = ""
		}

		b.WriteString(m.textInput.View())
		b.WriteString("\n")

		// Bottom border.
		b.WriteString(renderInputBorder(m.width))
		b.WriteString("\n")

		// Hints line below the input area.
		if m.mode == modeInput && len(m.completions) == 0 {
			if m.dynSuggestion != "" && m.textInput.Value() == "" {
				b.WriteString("  " + shortcutsHintStyle.Render("enter to send, tab to edit, esc to dismiss"))
				b.WriteString("\n")
			} else {
				b.WriteString("  " + shortcutsHintStyle.Render("? for shortcuts"))
				b.WriteString("\n")
			}
		} else if m.mode == modeStreaming {
			hint := "Enter to queue message"
			if m.queue.Len() > 0 {
				hint += fmt.Sprintf(" · %d queued", m.queue.Len())
			}
			b.WriteString("  " + shortcutsHintStyle.Render(hint))
			b.WriteString("\n")
		}
	}

	// 8b. Queue indicator when streaming and not actively typing.
	if m.mode == modeStreaming && m.textInput.Value() == "" && m.queue.Len() > 0 {
		b.WriteString(queuedBadgeStyle.Render(fmt.Sprintf("  %d message%s queued",
			m.queue.Len(), pluralS(m.queue.Len()))))
		b.WriteString("\n")
	}

	// 9. Status line (custom command output) or default status bar.
	if m.statusLineText != "" {
		b.WriteString(statusBarStyle.Render(m.statusLineText))
	} else {
		b.WriteString(renderStatusBar(m.modelName, &m.tokens, m.width, m.fastMode))
	}

	return b.String()
}

// renderCompletions renders the inline completion suggestions.
func (m model) renderCompletions() string {
	var b strings.Builder
	maxShow := 8
	if len(m.completions) < maxShow {
		maxShow = len(m.completions)
	}

	for i := 0; i < maxShow; i++ {
		name := m.completions[i]
		desc := ""
		if cmd, ok := m.slashReg.lookup(name); ok {
			desc = cmd.Description
		}
		if i == m.completionIdx {
			b.WriteString(askSelectedStyle.Render("  > /"+name) + " " + permHintStyle.Render(desc) + "\n")
		} else {
			b.WriteString(permHintStyle.Render("    /"+name+" "+desc) + "\n")
		}
	}

	if len(m.completions) > maxShow {
		b.WriteString(permHintStyle.Render(fmt.Sprintf("    ... and %d more", len(m.completions)-maxShow)) + "\n")
	}

	return b.String()
}

// handleResumeKey processes key events during the session picker.
func (m model) handleResumeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.resumeSessions) == 0 {
		m.mode = modeInput
		m.textInput.Focus()
		return m, textarea.Blink
	}

	switch msg.Type {
	case tea.KeyUp:
		if m.resumeCursor > 0 {
			m.resumeCursor--
		}
		return m, nil

	case tea.KeyDown:
		if m.resumeCursor < len(m.resumeSessions)-1 {
			m.resumeCursor++
		}
		return m, nil

	case tea.KeyEnter:
		sess := m.resumeSessions[m.resumeCursor]
		// Switch the current session to the selected one.
		m.session.ID = sess.ID
		m.session.Model = sess.Model
		m.session.CWD = sess.CWD
		m.session.Messages = sess.Messages
		m.session.CreatedAt = sess.CreatedAt
		m.session.UpdatedAt = sess.UpdatedAt

		// Replace the loop's history with the resumed session's messages.
		m.loop.History().SetMessages(sess.Messages)

		// Clear picker state.
		m.resumeSessions = nil
		m.resumeCursor = 0
		m.mode = modeInput
		m.textInput.Focus()

		summary := sessionSummary(sess)
		line := resumeHeaderStyle.Render("Resumed session ") +
			resumeIDStyle.Render(sess.ID) +
			resumeHeaderStyle.Render(" ("+summary+")")
		return m, tea.Batch(tea.Println(line), textarea.Blink)

	case tea.KeyEsc, tea.KeyCtrlC:
		m.resumeSessions = nil
		m.resumeCursor = 0
		m.mode = modeInput
		m.textInput.Focus()
		return m, textarea.Blink
	}

	return m, nil
}

// renderResumePicker renders the session selection list.
func (m model) renderResumePicker() string {
	var b strings.Builder
	b.WriteString(resumeHeaderStyle.Render("Select a session to resume:") + "\n")

	// Show at most 10 sessions.
	maxVisible := 10
	if len(m.resumeSessions) < maxVisible {
		maxVisible = len(m.resumeSessions)
	}

	// Calculate scroll window.
	start := 0
	if m.resumeCursor >= maxVisible {
		start = m.resumeCursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(m.resumeSessions) {
		end = len(m.resumeSessions)
		start = end - maxVisible
		if start < 0 {
			start = 0
		}
	}

	for i := start; i < end; i++ {
		sess := m.resumeSessions[i]
		timeStr := relativeTime(sess.UpdatedAt)
		msgCount := len(sess.Messages)
		firstMsg := firstUserMessage(sess)
		if len(firstMsg) > 60 {
			firstMsg = firstMsg[:57] + "..."
		}

		desc := timeStr + " | " + pluralize(msgCount, "message", "messages")
		if firstMsg != "" {
			desc += " | " + firstMsg
		}

		if i == m.resumeCursor {
			b.WriteString(askSelectedStyle.Render("  > "+desc) + "\n")
		} else {
			b.WriteString(askOptionStyle.Render("    "+desc) + "\n")
		}
	}

	if len(m.resumeSessions) > maxVisible {
		b.WriteString(permHintStyle.Render("  (showing " + pluralize(maxVisible, "session", "sessions") +
			" of " + pluralize(len(m.resumeSessions), "", "") + ")") + "\n")
	}

	b.WriteString(permHintStyle.Render("  Use arrow keys to navigate, Enter to select, Esc to cancel"))
	return b.String()
}

// relativeTime formats a time as a human-readable relative string.
func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("%dh ago", h)
	case d < 30*24*time.Hour:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	default:
		return t.Format("Jan 2")
	}
}

// firstUserMessage extracts the text of the first user message in a session.
func firstUserMessage(sess *session.Session) string {
	for _, msg := range sess.Messages {
		if msg.Role != api.RoleUser {
			continue
		}
		// Content can be a JSON string or []ContentBlock.
		var text string
		if err := json.Unmarshal(msg.Content, &text); err == nil {
			return strings.TrimSpace(text)
		}
		var blocks []api.ContentBlock
		if err := json.Unmarshal(msg.Content, &blocks); err == nil {
			for _, b := range blocks {
				if b.Type == api.ContentTypeText && b.Text != "" {
					return strings.TrimSpace(b.Text)
				}
			}
		}
		break
	}
	return ""
}

// sessionSummary returns a short summary string for a session.
func sessionSummary(sess *session.Session) string {
	parts := []string{
		relativeTime(sess.UpdatedAt),
		pluralize(len(sess.Messages), "message", "messages"),
	}
	return strings.Join(parts, ", ")
}

// pluralize returns "N item" or "N items" based on count.
func pluralize(n int, singular, plural string) string {
	if singular == "" && plural == "" {
		return fmt.Sprintf("%d", n)
	}
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}

// renderAskUserPrompt renders the current ask-user question.
func (m model) renderAskUserPrompt() string {
	if m.askUserPending == nil || m.askQuestionIdx >= len(m.askUserPending.Questions) {
		return ""
	}

	q := m.askUserPending.Questions[m.askQuestionIdx]
	var b strings.Builder

	b.WriteString(askHeaderStyle.Render("["+q.Header+"]") + " " + askQuestionStyle.Render(q.Question) + "\n")

	for i, opt := range q.Options {
		prefix := "  "
		if i == m.askCursor && !m.askCustomInput {
			b.WriteString(askSelectedStyle.Render(prefix+"> "+opt.Label) + " " + askOptionStyle.Render(opt.Description) + "\n")
		} else {
			b.WriteString(askOptionStyle.Render(prefix+"  "+opt.Label+" "+opt.Description) + "\n")
		}
	}

	// "Other" option.
	otherIdx := len(q.Options)
	if m.askCursor == otherIdx && !m.askCustomInput {
		b.WriteString(askSelectedStyle.Render("  > Other (custom input)") + "\n")
	} else if m.askCustomInput {
		b.WriteString(askSelectedStyle.Render("  > Other: "+m.askCustomText+"_") + "\n")
	} else {
		b.WriteString(askOptionStyle.Render("    Other (custom input)") + "\n")
	}

	b.WriteString(permHintStyle.Render("  Use arrow keys to navigate, Enter to select"))

	return b.String()
}

// handleModelCommand processes /model with optional argument.
func (m model) handleModelCommand(parts []string) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	if len(parts) < 2 {
		// No argument: open interactive model picker.
		m.modelPickerCursor = 0
		// Pre-select the current model.
		for i, opt := range api.AvailableModels {
			if opt.ID == m.modelName || opt.Alias == m.modelName {
				m.modelPickerCursor = i
				break
			}
		}
		m.mode = modeModelPicker
		return m, nil
	}

	// Argument provided: switch directly.
	arg := strings.TrimSpace(parts[1])
	resolved := api.ResolveModelAlias(arg)

	return m.switchModel(resolved, cmds)
}

// switchModel updates the model across the loop, TUI state, and session.
func (m model) switchModel(newModel string, cmds []tea.Cmd) (tea.Model, tea.Cmd) {
	m.loop.SetModel(newModel)
	m.modelName = newModel

	if m.onModelSwitch != nil {
		m.onModelSwitch(newModel)
	}

	display := api.ModelDisplayName(newModel)
	msg := fmt.Sprintf("Switched to model: %s (%s)", display, newModel)
	cmds = append(cmds, tea.Println(msg))
	return m, tea.Batch(cmds...)
}

// handleModelPickerKey processes key events during the model picker.
func (m model) handleModelPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	numOptions := len(api.AvailableModels)

	switch msg.Type {
	case tea.KeyUp:
		if m.modelPickerCursor > 0 {
			m.modelPickerCursor--
		}
		return m, nil

	case tea.KeyDown:
		if m.modelPickerCursor < numOptions-1 {
			m.modelPickerCursor++
		}
		return m, nil

	case tea.KeyEnter:
		selected := api.AvailableModels[m.modelPickerCursor]
		m.mode = modeInput
		m.textInput.Focus()
		return m.switchModel(selected.ID, []tea.Cmd{textarea.Blink})

	case tea.KeyEsc, tea.KeyCtrlC:
		m.mode = modeInput
		m.textInput.Focus()
		return m, tea.Println("Model selection cancelled.")
	}

	return m, nil
}

// renderModelPicker renders the model selection UI.
func (m model) renderModelPicker() string {
	var b strings.Builder

	b.WriteString(askHeaderStyle.Render("[Model]") + " " + askQuestionStyle.Render("Select a model:") + "\n")

	for i, opt := range api.AvailableModels {
		current := ""
		if opt.ID == m.modelName {
			current = " (current)"
		}
		if i == m.modelPickerCursor {
			b.WriteString(askSelectedStyle.Render(fmt.Sprintf("  > %s%s", opt.DisplayName, current)) + " " + askOptionStyle.Render(opt.Description) + "\n")
		} else {
			b.WriteString(askOptionStyle.Render(fmt.Sprintf("    %s%s %s", opt.DisplayName, current, opt.Description)) + "\n")
		}
	}

	b.WriteString(permHintStyle.Render("  Use arrow keys to navigate, Enter to select, Esc to cancel"))

	return b.String()
}
