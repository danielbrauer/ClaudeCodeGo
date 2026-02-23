package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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

	// Fast mode toggle.
	fastMode bool

	// Initial prompt to send on start.
	initialPrompt string

	// Whether we should quit.
	quitting bool

	// Exit action to signal the caller (e.g., re-run login after TUI exits).
	exitAction ExitAction
}

// newModel creates the initial Bubble Tea model.
func newModel(
	loop *conversation.Loop,
	ctx context.Context,
	cancelFn context.CancelFunc,
	modelName, version string,
	initialPrompt string,
	width int,
	mcpStatus MCPStatus,
	loadedSkills []skills.Skill,
	sessStore *session.Store,
	sess *session.Session,
	settings *config.Settings,
	onModelSwitch func(newModel string),
	logoutFunc func() error,
	fastMode bool,
) model {
	ti := newTextInput(width)
	sp := newSpinner()
	md := newMarkdownRenderer(width)
	slash := newSlashRegistry()

	// Phase 7: Register skill-based slash commands.
	if len(loadedSkills) > 0 {
		slash.registerSkills(loadedSkills)
	}

	return model{
		loop:          loop,
		ctx:           ctx,
		cancelFn:      cancelFn,
		modelName:     modelName,
		version:       version,
		mcpStatus:     mcpStatus,
		settings:      settings,
		onModelSwitch: onModelSwitch,
		mode:          modeInput,
		width:         width,
		height:        24,
		textInput:     ti,
		spinner:       sp,
		mdRenderer:    md,
		slashReg:      slash,
		logoutFunc:    logoutFunc,
		initialPrompt: initialPrompt,
		sessStore:     sessStore,
		session:       sess,
		fastMode:      fastMode,
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
		m.mode = modeInput
		m.activeTool = ""
		m.textInput.Focus()
		return m, tea.Batch(append(cmds, textarea.Blink)...)

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
	if m.mode == modeInput {
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// handleKey processes keyboard input based on the current mode.
func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {

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
		// Ctrl+C during streaming cancels the loop.
		if msg.Type == tea.KeyCtrlC {
			m.cancelFn()
			return m, nil
		}
		return m, nil

	case modeInput:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit

		case tea.KeyEnter:
			text := strings.TrimSpace(m.textInput.Value())
			if text == "" {
				return m, nil
			}
			m.textInput.Reset()
			return m.handleSubmit(text)

		default:
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd
		}
	}

	return m, nil
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

		if cmdName == "quit" || cmdName == "exit" {
			m.quitting = true
			return m, tea.Quit
		}

		if cmdName == "config" || cmdName == "settings" {
			if m.settings != nil {
				m.configPanel = newConfigPanel(m.settings)
				m.mode = modeConfig
				m.textInput.Blur()
			} else {
				cmds = append(cmds, tea.Println(errorStyle.Render("No settings loaded.")))
			}
			return m, tea.Batch(cmds...)
		}

		if cmdName == "login" {
			cmds = append(cmds, tea.Println("Exiting session for re-authentication..."))
			m.exitAction = ExitLogin
			m.quitting = true
			return m, tea.Batch(append(cmds, tea.Quit)...)
		}

		if cmdName == "logout" {
			if m.logoutFunc != nil {
				if err := m.logoutFunc(); err != nil {
					cmds = append(cmds, tea.Println(errorStyle.Render("Failed to log out.")))
					return m, tea.Batch(cmds...)
				}
			}
			cmds = append(cmds, tea.Println("Successfully logged out from your Anthropic account."))
			m.quitting = true
			return m, tea.Batch(append(cmds, tea.Quit)...)
		}

		if cmdName == "compact" {
			m.mode = modeStreaming
			return m, func() tea.Msg {
				err := m.loop.Compact(m.ctx)
				if err != nil {
					return LoopDoneMsg{Err: err}
				}
				return LoopDoneMsg{}
			}
		}

		if cmdName == "resume" {
			if m.sessStore == nil {
				cmds = append(cmds, tea.Println(errorStyle.Render("Session store not available.")))
				return m, tea.Batch(cmds...)
			}
			sessions, err := m.sessStore.List()
			if err != nil || len(sessions) == 0 {
				cmds = append(cmds, tea.Println(errorStyle.Render("No sessions found.")))
				return m, tea.Batch(cmds...)
			}
			m.resumeSessions = sessions
			m.resumeCursor = 0
			m.mode = modeResume
			m.textInput.Blur()
			return m, tea.Batch(cmds...)
		}

		if cmdName == "continue" {
			if m.sessStore == nil {
				cmds = append(cmds, tea.Println(errorStyle.Render("Session store not available.")))
				return m, tea.Batch(cmds...)
			}
			sess, err := m.sessStore.MostRecent()
			if err != nil {
				cmds = append(cmds, tea.Println(errorStyle.Render("No previous session found.")))
				return m, tea.Batch(cmds...)
			}
			// Switch to the most recent session directly.
			m.session.ID = sess.ID
			m.session.Model = sess.Model
			m.session.CWD = sess.CWD
			m.session.Messages = sess.Messages
			m.session.CreatedAt = sess.CreatedAt
			m.session.UpdatedAt = sess.UpdatedAt
			m.loop.History().SetMessages(sess.Messages)

			summary := sessionSummary(sess)
			line := resumeHeaderStyle.Render("Resumed session ") +
				resumeIDStyle.Render(sess.ID) +
				resumeHeaderStyle.Render(" ("+summary+")")
			cmds = append(cmds, tea.Println(line))
			return m, tea.Batch(cmds...)
		}

		if cmdName == "clear" || cmdName == "reset" || cmdName == "new" {
			// Clear conversation history.
			m.loop.Clear()

			// Reset token tracking.
			m.tokens = tokenTracker{}

			// Clear todo list.
			m.todos = nil

			// Create a new session, preserving the model and CWD.
			if m.session != nil {
				m.session = &session.Session{
					ID:    session.GenerateID(),
					Model: m.session.Model,
					CWD:   m.session.CWD,
				}

				// Update the turn-complete callback to reference the new session.
				newSess := m.session
				store := m.sessStore
				m.loop.SetOnTurnComplete(func(h *conversation.History) {
					if store != nil && newSess != nil {
						newSess.Messages = h.Messages()
						_ = store.Save(newSess)
					}
				})

				if m.sessStore != nil {
					if err := m.sessStore.Save(m.session); err != nil {
						errLine := errorStyle.Render("Warning: failed to save new session: " + err.Error())
						cmds = append(cmds, tea.Println(errLine))
					}
				}
			}

			cmds = append(cmds, tea.Println("Conversation cleared. Starting fresh."))
			return m, tea.Batch(cmds...)
		}

		if cmdName == "memory" {
			arg := ""
			if len(parts) > 1 {
				arg = strings.TrimSpace(parts[1])
			}
			cwd, _ := os.Getwd()
			filePath := memoryFilePath(arg, cwd)
			editorCmd, err := editorCommand(filePath)
			if err != nil {
				cmds = append(cmds, tea.Println("Error: "+err.Error()))
				return m, tea.Batch(cmds...)
			}
			execCb := func(err error) tea.Msg {
				return MemoryEditDoneMsg{Path: filePath, Err: err}
			}
			return m, tea.Batch(append(cmds, tea.ExecProcess(editorCmd, execCb))...)
		}

		if cmdName == "init" {
			m.mode = modeStreaming
			m.textInput.Blur()
			loopCmd := func() tea.Msg {
				err := m.loop.SendMessage(m.ctx, initPrompt)
				return LoopDoneMsg{Err: err}
			}
			cmds = append(cmds, loopCmd, m.spinner.Tick)
			return m, tea.Batch(cmds...)
		}

		if cmdName == "model" {
			return m.handleModelCommand(parts)
		}

		if cmdName == "diff" {
			m.mode = modeDiff
			m.diffData = nil
			return m, tea.Batch(
				func() tea.Msg {
					data := loadDiffData()
					return DiffLoadedMsg{Data: data}
				},
				m.spinner.Tick,
			)
		}

		if cmdName == "review" {
			// Extract optional argument (PR number).
			arg := ""
			if len(parts) > 1 {
				arg = strings.TrimSpace(parts[1])
			}
			reviewPrompt := buildReviewPrompt(arg)
			m.mode = modeStreaming
			m.textInput.Blur()
			loopCmd := func() tea.Msg {
				err := m.loop.SendMessage(m.ctx, reviewPrompt)
				return LoopDoneMsg{Err: err}
			}
			cmds = append(cmds, loopCmd, m.spinner.Tick)
			return m, tea.Batch(cmds...)
		}

		if cmd, ok := m.slashReg.lookup(cmdName); ok && cmd.Execute != nil {
			output := cmd.Execute(&m)
			// Phase 7: Skill slash commands return a sentinel prefix.
			// When detected, send the skill content as a user message.
			if strings.HasPrefix(output, skillCommandPrefix) {
				skillContent := strings.TrimPrefix(output, skillCommandPrefix)
				m.mode = modeStreaming
				m.textInput.Blur()
				loopCmd := func() tea.Msg {
					err := m.loop.SendMessage(m.ctx, skillContent)
					return LoopDoneMsg{Err: err}
				}
				cmds = append(cmds, loopCmd, m.spinner.Tick)
				return m, tea.Batch(cmds...)
			}
			cmds = append(cmds, tea.Println(output))
			return m, tea.Batch(cmds...)
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

	// 0. Diff dialog (takes over the entire view).
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

	// 7. Input area.
	if m.mode == modeInput {
		b.WriteString(m.textInput.View())
		b.WriteString("\n")
	}

	// 8. Status bar.
	b.WriteString(renderStatusBar(m.modelName, &m.tokens, m.width, m.fastMode))

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
