package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/config"
	"github.com/anthropics/claude-code-go/internal/conversation"
	"github.com/anthropics/claude-code-go/internal/session"
	"github.com/anthropics/claude-code-go/internal/skills"
)

// MCPStatus provides MCP server information to the TUI without importing
// the mcp package directly (avoiding import cycles).
type MCPStatus interface {
	Servers() []string
	ServerStatus(name string) string
}

// ExitAction indicates what the caller should do after the TUI exits.
type ExitAction int

const (
	ExitNone  ExitAction = iota
	ExitLogin            // The user requested /login; caller should run the login flow.
)

// AppConfig bundles everything the TUI needs from main.go.
type AppConfig struct {
	Loop          *conversation.Loop
	Session       *session.Session
	SessStore     *session.Store
	Version       string
	Model         string
	Cwd           string                             // working directory, shown in startup banner
	BillingType   string                             // subscription display name (e.g. "Claude Pro"); may be empty
	PrintMode     bool
	MCPManager    MCPStatus                          // *mcp.Manager; nil if no MCP servers configured
	Skills        []skills.Skill                     // Phase 7: loaded skills for slash command registration
	Hooks         conversation.HookRunner            // Phase 7: hook runner for SessionStart, etc.
	Settings      *config.Settings                   // live settings for config panel
	RuleHandler   *config.RuleBasedPermissionHandler // Rule-based permission handler from main; may be nil
	OnModelSwitch func(newModel string)              // called when user switches model via /model
	LogoutFunc    func() error                       // Called when the user types /logout to clear credentials.
	FastMode      bool                               // initial fast mode state from settings
	Client        *api.Client                        // API client for model switching
}

// App is the top-level TUI application. main.go creates it and calls Run.
type App struct {
	cfg           AppConfig
	initialPrompt string
	exitAction    ExitAction
}

// ExitAction returns the action the caller should take after Run() returns.
func (a *App) ExitAction() ExitAction {
	return a.exitAction
}

// New creates a new TUI application.
func New(cfg AppConfig) *App {
	return &App{cfg: cfg}
}

// SetInitialPrompt sets a prompt to be sent automatically on start.
func (a *App) SetInitialPrompt(prompt string) {
	a.initialPrompt = prompt
}

// Run starts the Bubble Tea program and blocks until it exits.
// It wires up the TUI stream handler and permission handler so that
// the agentic loop's events flow into the BT event loop.
func (a *App) Run(ctx context.Context) error {
	// Detect terminal width for initial layout.
	width := 80
	if w, _, err := term.GetSize(0); err == nil && w > 0 {
		width = w
	}

	// Create a cancellable context for the agentic loop.
	loopCtx, loopCancel := context.WithCancel(ctx)

	// Phase 7: Fire SessionStart hook before UI starts.
	if a.cfg.Hooks != nil {
		_ = a.cfg.Hooks.RunSessionStart(loopCtx)
	}

	// Create the Bubble Tea model.
	m := newModel(
		a.cfg.Loop,
		loopCtx,
		loopCancel,
		a.cfg.Model,
		a.cfg.Version,
		a.initialPrompt,
		width,
		a.cfg.MCPManager,
		a.cfg.Skills,
		a.cfg.SessStore,
		a.cfg.Session,
		a.cfg.Settings,
		a.cfg.OnModelSwitch,
		a.cfg.LogoutFunc,
		a.cfg.FastMode,
	)
	m.apiClient = a.cfg.Client

	// Create the BT program (inline mode, no alt screen).
	// Bracketed paste is enabled by default in bubbletea v1.x.
	p := tea.NewProgram(m)

	// Wire the TUI stream handler into the loop.
	handler := NewTUIStreamHandler(p)
	a.cfg.Loop.SetHandler(handler)

	// Wire the TUI permission handler into the loop's registry.
	// The TUI handler wraps the rule-based handler from main.go so that
	// rules are checked first, and only unmatched calls prompt the user.
	permHandler := NewTUIPermissionHandler(p, a.cfg.RuleHandler)
	a.cfg.Loop.SetPermissionHandler(permHandler)

	// Print the banner to scrollback before starting (matches JS version layout).
	modelDisplay := api.ModelDisplayName(a.cfg.Model)
	fmt.Println()
	fmt.Printf("\033[1m✻ Claude Code\033[0m v%s\n", a.cfg.Version)
	if a.cfg.BillingType != "" {
		fmt.Printf("  %s · %s\n", modelDisplay, a.cfg.BillingType)
	} else {
		fmt.Printf("  %s\n", modelDisplay)
	}
	cwdDisplay := shortenPath(a.cfg.Cwd)
	if cwdDisplay != "" {
		fmt.Printf("  %s\n", cwdDisplay)
	}
	fmt.Println()

	// Run the BT event loop (blocks until quit).
	finalModel, err := p.Run()

	loopCancel()

	// Check if the user requested a special exit action (e.g., /login).
	if fm, ok := finalModel.(model); ok {
		a.exitAction = fm.exitAction
	}

	return err
}
