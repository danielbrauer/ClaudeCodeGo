package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/anthropics/claude-code-go/internal/conversation"
	"github.com/anthropics/claude-code-go/internal/session"
)

// MCPStatus provides MCP server information to the TUI without importing
// the mcp package directly (avoiding import cycles).
type MCPStatus interface {
	Servers() []string
	ServerStatus(name string) string
}

// AppConfig bundles everything the TUI needs from main.go.
type AppConfig struct {
	Loop       *conversation.Loop
	Session    *session.Session
	SessStore  *session.Store
	Version    string
	Model      string
	PrintMode  bool
	MCPManager MCPStatus // *mcp.Manager; nil if no MCP servers configured
}

// App is the top-level TUI application. main.go creates it and calls Run.
type App struct {
	cfg           AppConfig
	initialPrompt string
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
	)

	// Create the BT program (inline mode, no alt screen).
	// Bracketed paste is enabled by default in bubbletea v1.x.
	p := tea.NewProgram(m)

	// Wire the TUI stream handler into the loop.
	handler := NewTUIStreamHandler(p)
	a.cfg.Loop.SetHandler(handler)

	// Wire the TUI permission handler into the loop's registry.
	// The registry's permission handler is set from main.go; we replace it here.
	permHandler := NewTUIPermissionHandler(p)
	a.cfg.Loop.SetPermissionHandler(permHandler)

	// Print the banner to scrollback before starting.
	fmt.Printf("\nclaude %s (Go) | model: %s\n", a.cfg.Version, a.cfg.Model)
	fmt.Println("Type your message. Press Ctrl+C to exit.")
	fmt.Println()

	// Run the BT event loop (blocks until quit).
	_, err := p.Run()

	loopCancel()
	return err
}
