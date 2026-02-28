package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"

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
	modelName       string
	resolvedModelID string // full model ID from API response (e.g. "claude-sonnet-4-20250514")
	version         string
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

	// Ctrl-C double-press state: true after the first press, reset after
	// ctrlCTimeout (800 ms). A second press within the window exits.
	ctrlCPending bool

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

	m := model{
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
	m.tokens.setModel(cfg.ModelName)
	return m
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

// getPermissionMode returns the current permission mode from the loop's
// permission context. Returns ModeDefault if no context is available.
func (m *model) getPermissionMode() config.PermissionMode {
	permCtx := m.loop.GetPermissionContext()
	if permCtx != nil {
		return permCtx.GetMode()
	}
	return config.ModeDefault
}

// setPermissionMode changes the permission mode on the loop's permission
// context. Returns false if the mode is disabled by policy.
func (m *model) setPermissionMode(mode config.PermissionMode) bool {
	if m.settings != nil {
		if config.IsPermissionModeDisabled(mode, m.settings.DisableBypassPermissions) {
			return false
		}
	}
	permCtx := m.loop.GetPermissionContext()
	if permCtx != nil {
		permCtx.SetMode(mode)
	}
	return true
}

// cyclePermissionMode advances to the next permission mode.
func (m *model) cyclePermissionMode() config.PermissionMode {
	current := m.getPermissionMode()
	bypassEnabled := true
	if m.settings != nil && m.settings.DisableBypassPermissions == "disable" {
		bypassEnabled = false
	}
	next := config.CyclePermissionMode(current, bypassEnabled)
	m.setPermissionMode(next)
	return next
}
