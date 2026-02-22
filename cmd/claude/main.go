// Package main is the entry point for the claude CLI.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"golang.org/x/term"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/auth"
	"github.com/anthropics/claude-code-go/internal/config"
	"github.com/anthropics/claude-code-go/internal/conversation"
	"github.com/anthropics/claude-code-go/internal/hooks"
	"github.com/anthropics/claude-code-go/internal/mcp"
	"github.com/anthropics/claude-code-go/internal/session"
	"github.com/anthropics/claude-code-go/internal/skills"
	"github.com/anthropics/claude-code-go/internal/tools"
	"github.com/anthropics/claude-code-go/internal/tui"
)

var (
	version = "dev"
)

func main() {
	// CLI flags.
	modelFlag := flag.String("model", "", "Model to use (opus, sonnet, haiku, or full model ID)")
	printMode := flag.Bool("p", false, "Print mode: non-interactive, exit after response")
	continueFlag := flag.Bool("c", false, "Continue most recent session")
	resumeFlag := flag.String("r", "", "Resume specific session by ID")
	maxTokens := flag.Int("max-tokens", api.DefaultMaxTokens, "Maximum response tokens")
	versionFlag := flag.Bool("version", false, "Print version and exit")
	loginFlag := flag.Bool("login", false, "Log in with OAuth")
	dangerousNoPermissions := flag.Bool("dangerously-skip-permissions", false, "Skip all permission prompts (use with caution)")
	outputFormat := flag.String("output-format", "text", "Output format: text, json, stream-json")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("claude %s (Go)\n", version)
		os.Exit(0)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		cancel()
	}()

	// Credential store.
	store, err := auth.NewCredentialStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Handle --login.
	if *loginFlag {
		if err := doLogin(ctx, store); err != nil {
			fmt.Fprintf(os.Stderr, "Login failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Check authentication.
	tokenProvider := auth.NewTokenProvider(store)
	if _, err := tokenProvider.GetAccessToken(ctx); err != nil {
		fmt.Println("Not authenticated. Starting login flow...")
		if err := doLogin(ctx, store); err != nil {
			fmt.Fprintf(os.Stderr, "Login failed: %v\n", err)
			os.Exit(1)
		}
		// Reload after login.
		tokenProvider = auth.NewTokenProvider(store)
	}

	// Working directory.
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting working directory: %v\n", err)
		os.Exit(1)
	}

	// Load settings from all levels.
	settings, err := config.LoadSettings(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: error loading settings: %v\n", err)
		settings = &config.Settings{}
	}

	// Phase 7: Parse hook config from settings.
	var hookConfig hooks.HookConfig
	if settings.Hooks != nil {
		if err := json.Unmarshal(settings.Hooks, &hookConfig); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: invalid hooks config: %v\n", err)
		}
	}
	hookRunner := hooks.NewRunner(hookConfig)

	// Phase 7: Load skills.
	loadedSkills := skills.LoadSkills(cwd)
	skillContent := skills.ActiveSkillContent(loadedSkills)

	// Resolve model: CLI flag > settings > default.
	model := api.ModelClaude4Sonnet
	if settings.Model != "" {
		model = api.ResolveModelAlias(settings.Model)
	}
	if *modelFlag != "" {
		model = api.ResolveModelAlias(*modelFlag)
	}

	// Create API client.
	client := api.NewClient(
		tokenProvider,
		api.WithModel(model),
		api.WithMaxTokens(*maxTokens),
		api.WithVersion(version),
	)

	// Build system prompt with settings context and skill content.
	system := conversation.BuildSystemPrompt(cwd, settings, skillContent)

	// Set up permission handler with rule-based evaluation.
	var permHandler tools.PermissionHandler
	if *dangerousNoPermissions {
		permHandler = &tools.AlwaysAllowPermissionHandler{}
	} else {
		terminalHandler := tools.NewTerminalPermissionHandler()
		if len(settings.Permissions) > 0 {
			permHandler = config.NewRuleBasedPermissionHandler(
				settings.Permissions,
				terminalHandler,
			)
		} else {
			permHandler = terminalHandler
		}
	}

	// Background task store shared by Agent, TaskOutput, and TaskStop tools.
	bgStore := tools.NewBackgroundTaskStore()

	// Create tool registry with all tools.
	registry := tools.NewRegistry(permHandler)
	if len(settings.Env) > 0 {
		registry.Register(tools.NewBashToolWithEnv(cwd, settings.Env))
	} else {
		registry.Register(tools.NewBashTool(cwd))
	}
	registry.Register(tools.NewFileReadTool())
	registry.Register(tools.NewFileEditTool())
	registry.Register(tools.NewFileWriteTool())
	registry.Register(tools.NewGlobTool(cwd))
	registry.Register(tools.NewGrepTool(cwd))

	// Phase 4 tools.
	registry.Register(tools.NewTodoWriteTool())
	registry.Register(tools.NewAskUserTool())
	registry.Register(tools.NewWebFetchTool(nil))
	registry.Register(tools.NewWebSearchTool())
	registry.Register(tools.NewNotebookEditTool())
	registry.Register(tools.NewConfigTool(cwd))
	registry.Register(tools.NewWorktreeTool(cwd))
	registry.Register(tools.NewExitPlanModeTool())
	registry.Register(tools.NewTaskOutputTool(bgStore))
	registry.Register(tools.NewTaskStopTool(bgStore))

	// Phase 6: MCP server initialization.
	// Load MCP config and start servers before AgentTool so MCP tools
	// are visible to sub-agents via registry.Definitions().
	mcpConfig, err := mcp.LoadMCPConfig(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: MCP config error: %v\n", err)
	}

	var mcpManager *mcp.Manager
	if mcpConfig != nil && len(mcpConfig.MCPServers) > 0 {
		mcpManager = mcp.NewManager(cwd)
		if err := mcpManager.StartServers(ctx, mcpConfig.MCPServers, registry); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: MCP startup error: %v\n", err)
		}
		defer mcpManager.Shutdown()

		// Register MCP management tools (these need the manager reference).
		registry.Register(mcp.NewListMcpResourcesTool(mcpManager))
		registry.Register(mcp.NewReadMcpResourceTool(mcpManager))
		registry.Register(mcp.NewSubscribeMcpResourceTool(mcpManager))
		registry.Register(mcp.NewUnsubscribeMcpResourceTool(mcpManager))
		registry.Register(mcp.NewSubscribePollingTool(mcpManager))
		registry.Register(mcp.NewUnsubscribePollingTool(mcpManager))
	}

	// Agent tool registered last — gets tool definitions that include everything above.
	// Phase 7: Pass hookRunner so sub-agents inherit hooks.
	agentTool := tools.NewAgentTool(client, system, registry.Definitions(), registry, bgStore, hookRunner)
	registry.Register(agentTool)

	// Session management.
	sessionStore, err := session.NewStore(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: session store unavailable: %v\n", err)
	}

	// Check for session resume.
	var history *conversation.History
	var currentSession *session.Session

	if *continueFlag && sessionStore != nil {
		sess, err := sessionStore.MostRecent()
		if err != nil {
			fmt.Fprintf(os.Stderr, "No previous session found: %v\n", err)
		} else {
			history = conversation.NewHistoryFrom(sess.Messages)
			currentSession = sess
			fmt.Printf("Resuming session %s (%d messages)\n", sess.ID, len(sess.Messages))
		}
	}

	if *resumeFlag != "" && sessionStore != nil {
		sess, err := sessionStore.Load(*resumeFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cannot load session %s: %v\n", *resumeFlag, err)
			os.Exit(1)
		}
		history = conversation.NewHistoryFrom(sess.Messages)
		currentSession = sess
		fmt.Printf("Resuming session %s (%d messages)\n", sess.ID, len(sess.Messages))
	}

	// Create a new session if not resuming.
	if currentSession == nil {
		currentSession = &session.Session{
			ID:    session.GenerateID(),
			Model: model,
			CWD:   cwd,
		}
	}

	// Create compactor for auto-compaction.
	compactor := conversation.NewCompactor(client)

	// Create conversation loop with tools.
	// In TUI mode, the handler and permission handler will be replaced by app.Run().
	// In print mode, use the simple PrintStreamHandler.
	handler := &conversation.ToolAwareStreamHandler{}
	loop := conversation.NewLoop(conversation.LoopConfig{
		Client:    client,
		System:    system,
		Tools:     registry.Definitions(),
		ToolExec:  registry,
		Handler:   handler,
		History:   history,
		Compactor: compactor,
		Hooks:     hookRunner, // Phase 7: wire hooks into the loop
		OnTurnComplete: func(h *conversation.History) {
			// Save session after each turn.
			if sessionStore != nil && currentSession != nil {
				currentSession.Messages = h.Messages()
				if err := sessionStore.Save(currentSession); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to save session: %v\n", err)
				}
			}
		},
	})

	// Handle initial prompt from arguments.
	args := flag.Args()
	initialPrompt := ""
	if len(args) > 0 {
		initialPrompt = strings.Join(args, " ")
	}

	// Phase 7: Pipe/stdin support — if stdin is not a terminal, read prompt from stdin.
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		data, err := io.ReadAll(os.Stdin)
		if err == nil && len(data) > 0 {
			pipeInput := strings.TrimSpace(string(data))
			if initialPrompt != "" {
				// Combine: CLI prompt + piped content.
				initialPrompt = initialPrompt + "\n\n" + pipeInput
			} else {
				initialPrompt = pipeInput
			}
			*printMode = true // force print mode when piped
		}
	}

	// Print mode: use simple handler, no TUI.
	if *printMode {
		if initialPrompt != "" {
			// Phase 7: Select handler based on --output-format.
			switch *outputFormat {
			case "json":
				loop.SetHandler(conversation.NewJSONStreamHandler(os.Stdout))
			case "stream-json":
				loop.SetHandler(conversation.NewStreamJSONStreamHandler(os.Stdout))
			default:
				loop.SetHandler(&conversation.PrintStreamHandler{})
			}

			// Fire SessionStart hook in print mode.
			_ = hookRunner.RunSessionStart(ctx)

			if err := loop.SendMessage(ctx, initialPrompt); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}
		os.Exit(0)
	}

	// Interactive mode: launch the TUI.
	app := tui.New(tui.AppConfig{
		Loop:       loop,
		Session:    currentSession,
		SessStore:  sessionStore,
		Version:    version,
		Model:      model,
		MCPManager: mcpManager,
		Skills:     loadedSkills,  // Phase 7
		Hooks:      hookRunner,    // Phase 7
		OnModelSwitch: func(newModel string) {
			if currentSession != nil {
				currentSession.Model = newModel
			}
		},
	})

	if initialPrompt != "" {
		app.SetInitialPrompt(initialPrompt)
	}

	if err := app.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func doLogin(ctx context.Context, store *auth.CredentialStore) error {
	flow, err := auth.NewOAuthFlow()
	if err != nil {
		return fmt.Errorf("initializing OAuth flow: %w", err)
	}
	result, err := flow.Login(ctx)
	if err != nil {
		return err
	}

	if err := store.Save(result.Tokens); err != nil {
		return fmt.Errorf("saving tokens: %w", err)
	}

	// Issue 9: Store account metadata.
	if result.Account != nil {
		if err := store.SaveAccount(result.Account); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save account info: %v\n", err)
		}
	}

	// Issue 8: Store API key.
	if result.APIKey != "" {
		if err := store.SaveAPIKey(result.APIKey); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save API key: %v\n", err)
		}
	}

	fmt.Println("Login successful!")
	return nil
}
