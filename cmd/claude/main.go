// Package main is the entry point for the claude CLI.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"os/user"
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

// subcommand defines a CLI subcommand (e.g. `claude login`).
type subcommand struct {
	Name string
	Run  func(args []string) // args is everything after the subcommand name
}

// subcommandRegistry holds all registered CLI subcommands.
var subcommandRegistry []subcommand

func registerSubcommand(cmd subcommand) {
	subcommandRegistry = append(subcommandRegistry, cmd)
}

func init() {
	registerSubcommand(subcommand{Name: "login", Run: func(args []string) { runLogin(args) }})
	registerSubcommand(subcommand{Name: "logout", Run: func(args []string) { runLogout() }})
	registerSubcommand(subcommand{Name: "status", Run: func(args []string) { runStatus(args) }})
	registerSubcommand(subcommand{Name: "update", Run: func(args []string) { runUpdate(args) }})
	registerSubcommand(subcommand{Name: "mcp", Run: func(args []string) { runMCP(args) }})
	registerSubcommand(subcommand{Name: "agents", Run: func(args []string) { runAgents() }})
}

// dispatchSubcommand checks os.Args for a registered subcommand and runs it.
// Returns true if a subcommand was dispatched.
func dispatchSubcommand() bool {
	args := os.Args[1:]
	if len(args) == 0 {
		return false
	}

	// Match "claude <subcmd> [flags]".
	for _, cmd := range subcommandRegistry {
		if args[0] == cmd.Name {
			cmd.Run(args[1:])
			return true
		}
	}

	// Match "claude auth status [flags]" (compound subcommand).
	if args[0] == "auth" && len(args) > 1 && args[1] == "status" {
		runStatus(args[2:])
		return true
	}

	return false
}

func main() {
	// Check for subcommands before flag parsing.
	if dispatchSubcommand() {
		return
	}

	// CLI flags.
	modelFlag := flag.String("model", "", "Model to use (opus, sonnet, haiku, or full model ID)")
	printMode := flag.Bool("p", false, "Print mode: non-interactive, exit after response")
	continueFlag := flag.Bool("c", false, "Continue most recent session")
	resumeFlag := flag.String("r", "", "Resume specific session by ID")
	maxTokens := flag.Int("max-tokens", api.DefaultMaxTokens, "Maximum response tokens")
	versionFlag := flag.Bool("version", false, "Print version and exit")
	loginFlag := flag.Bool("login", false, "Log in with OAuth")
	dangerousNoPermissions := flag.Bool("dangerously-skip-permissions", false, "Skip all permission prompts (use with caution)")
	permissionModeFlag := flag.String("permission-mode", "", "Set session permission mode: default, plan, acceptEdits, bypassPermissions")
	outputFormat := flag.String("output-format", "text", "Output format: text, json, stream-json")

	// Session management flags.
	sessionIDFlag := flag.String("session-id", "", "Specify session UUID")

	// Model/thinking control flags.
	effortFlag := flag.String("effort", "", "Effort level: low, medium, high, max")
	thinkingFlag := flag.String("thinking", "", "Thinking mode: enabled, adaptive, disabled")
	maxThinkingTokens := flag.Int("max-thinking-tokens", 0, "Maximum thinking tokens")
	betasFlag := flag.String("betas", "", "Additional beta headers (comma-separated)")

	// System prompt override flags.
	systemPromptFlag := flag.String("system-prompt", "", "Custom system prompt (replaces default)")
	appendSystemPromptFlag := flag.String("append-system-prompt", "", "Append to default system prompt")

	// Agent/print mode control flags.
	maxTurnsFlag := flag.Int("max-turns", 0, "Maximum agentic turns (print mode)")

	// Permission control flags.
	allowedToolsFlag := flag.String("allowedTools", "", "Comma-separated list of tools to allow")
	disallowedToolsFlag := flag.String("disallowedTools", "", "Comma-separated list of tools to deny")

	// Debug flags.
	verboseFlag := flag.Bool("verbose", false, "Enable verbose output")

	// Other flags.
	addDirFlag := flag.String("add-dir", "", "Additional directories (comma-separated)")

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

	// Handle --login (legacy flag, same as `claude login` subcommand).
	if *loginFlag {
		if err := doLogin(ctx, store, auth.LoginOptions{}); err != nil {
			fmt.Fprintf(os.Stderr, "Login failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Check authentication.
	tokenProvider := auth.NewTokenProvider(store)
	if _, err := tokenProvider.GetAccessToken(ctx); err != nil {
		fmt.Println("Not authenticated. Starting login flow...")
		if err := doLogin(ctx, store, auth.LoginOptions{}); err != nil {
			fmt.Fprintf(os.Stderr, "Login failed: %v\n", err)
			os.Exit(1)
		}
		// Reload after login.
		tokenProvider = auth.NewTokenProvider(store)
	}

	// Determine billing/subscription display name for the startup banner.
	var billingType string
	if tokens, err := store.Load(); err == nil && tokens.SubscriptionType != "" {
		billingType = auth.SubscriptionDisplayName(tokens.SubscriptionType)
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
	model := api.ModelClaude46Opus
	if settings.Model != "" {
		model = api.ResolveModelAlias(settings.Model)
	}
	if *modelFlag != "" {
		model = api.ResolveModelAlias(*modelFlag)
	}

	// Apply verbose flag to settings.
	if *verboseFlag {
		settings.Verbose = config.BoolPtr(true)
	}

	// Create API client.
	client := api.NewClient(
		tokenProvider,
		api.WithModel(model),
		api.WithMaxTokens(*maxTokens),
		api.WithVersion(version),
	)

	// Collect context for system prompt and user message injection.
	claudeMDEntries := config.LoadClaudeMDEntries(cwd)
	claudeMDFormatted := config.FormatClaudeMDForContext(claudeMDEntries)
	gitStatus := conversation.CollectGitStatus(cwd)

	// Build system prompt with settings context, skill content, and git status.
	// Git status is appended to the system prompt (matching JS owq() pattern).
	system := conversation.BuildSystemPrompt(&conversation.PromptContext{
		CWD:          cwd,
		Model:        model,
		Settings:     settings,
		SkillContent: skillContent,
		Version:      version,
		GitStatus:    gitStatus,
	})

	// CLAUDE.md and date are injected as user message context (matching JS TN1 pattern).
	userContext := conversation.UserContext{
		ClaudeMD:    claudeMDFormatted,
		CurrentDate: conversation.FormatCurrentDate(),
	}
	contextMessage := conversation.BuildContextMessage(userContext)

	// Apply system prompt overrides from CLI flags.
	if *systemPromptFlag != "" {
		// Replace entire system prompt with custom prompt.
		system = []api.SystemBlock{{Type: "text", Text: *systemPromptFlag}}
	}
	if *appendSystemPromptFlag != "" {
		// Append to existing system prompt.
		system = append(system, api.SystemBlock{Type: "text", Text: *appendSystemPromptFlag})
	}

	// Apply --betas flag: additional beta headers passed to client via env.
	if *betasFlag != "" {
		existing := os.Getenv("ANTHROPIC_BETAS")
		if existing != "" {
			os.Setenv("ANTHROPIC_BETAS", existing+","+*betasFlag)
		} else {
			os.Setenv("ANTHROPIC_BETAS", *betasFlag)
		}
	}

	// Apply --add-dir flag: additional directories to include.
	if *addDirFlag != "" {
		for _, dir := range strings.Split(*addDirFlag, ",") {
			dir = strings.TrimSpace(dir)
			if dir != "" {
				// Load CLAUDE.md from additional directories.
				extraContent := config.LoadClaudeMD(dir)
				if extraContent != "" {
					system = append(system, api.SystemBlock{
						Type: "text",
						Text: fmt.Sprintf("# Additional Directory Instructions (%s)\n\n%s", dir, extraContent),
					})
				}
			}
		}
	}

	// Determine the initial permission mode.
	// Priority: --dangerously-skip-permissions > --permission-mode > settings > default.
	initialPermMode := config.ModeDefault
	if settings.DefaultPermissionMode != "" {
		initialPermMode = config.ValidatePermissionMode(settings.DefaultPermissionMode)
	}
	if *permissionModeFlag != "" {
		initialPermMode = config.ValidatePermissionMode(*permissionModeFlag)
	}
	if *dangerousNoPermissions {
		initialPermMode = config.ModeBypassPermissions
	}

	// Enforce bypass-permissions restrictions.
	if initialPermMode == config.ModeBypassPermissions {
		// Cannot use bypass with root/sudo.
		if u, err := user.Current(); err == nil && u.Uid == "0" {
			fmt.Fprintf(os.Stderr, "Error: --dangerously-skip-permissions cannot be used with root/sudo privileges for security reasons.\n")
			os.Exit(1)
		}

		// Cannot use bypass if disabled by policy.
		if config.IsPermissionModeDisabled(config.ModeBypassPermissions, settings.DisableBypassPermissions) {
			fmt.Fprintf(os.Stderr, "Error: Bypass permissions mode is disabled by settings or configuration.\n")
			os.Exit(1)
		}

		// Show warning dialog for bypass mode (interactive only).
		if !*printMode && term.IsTerminal(int(os.Stdin.Fd())) {
			if !showBypassPermissionsWarning() {
				fmt.Println("Bypass permissions mode declined. Exiting.")
				os.Exit(0)
			}
		}
	}

	// Apply --allowedTools / --disallowedTools to permission rules.
	if *allowedToolsFlag != "" {
		for _, t := range strings.Split(*allowedToolsFlag, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				settings.Permissions = append([]config.PermissionRule{{
					Tool: t, Action: "allow",
				}}, settings.Permissions...)
			}
		}
	}
	if *disallowedToolsFlag != "" {
		for _, t := range strings.Split(*disallowedToolsFlag, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				settings.Permissions = append([]config.PermissionRule{{
					Tool: t, Action: "deny",
				}}, settings.Permissions...)
			}
		}
	}

	// Set up permission handler with rule-based evaluation.
	var permHandler tools.PermissionHandler
	var ruleHandler *config.RuleBasedPermissionHandler
	terminalHandler := tools.NewTerminalPermissionHandler()
	ruleHandler = config.NewRuleBasedPermissionHandler(
		settings.Permissions,
		terminalHandler,
	)
	// Set the initial permission mode.
	ruleHandler.GetPermissionContext().SetMode(initialPermMode)
	permHandler = ruleHandler

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
		sid := session.GenerateID()
		if *sessionIDFlag != "" {
			sid = *sessionIDFlag
		}
		currentSession = &session.Session{
			ID:    sid,
			Model: model,
			CWD:   cwd,
		}
	}

	// Create compactor for auto-compaction (unless disabled).
	var compactor *conversation.Compactor
	disableCompact := os.Getenv("DISABLE_COMPACT") != ""
	if !disableCompact {
		compactor = conversation.NewCompactor(client)
	}

	// Resolve fast mode from settings.
	fastMode := settings.FastMode != nil && *settings.FastMode
	if fastMode && !api.IsOpus46Model(model) {
		// Fast mode requires Opus 4.6; switch if needed.
		model = api.ModelAliases[api.FastModeModelAlias]
		client.SetModel(model)
	}

	// Create conversation loop with tools.
	// In TUI mode, the handler and permission handler will be replaced by app.Run().
	// In print mode, use the simple PrintStreamHandler.
	handler := &conversation.ToolAwareStreamHandler{}
	loop := conversation.NewLoop(conversation.LoopConfig{
		Client:         client,
		System:         system,
		Tools:          registry.Definitions(),
		ToolExec:       registry,
		Handler:        handler,
		History:        history,
		Compactor:      compactor,
		Hooks:          hookRunner, // Phase 7: wire hooks into the loop
		ContextMessage: contextMessage,
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
	loop.SetFastMode(fastMode)

	// Apply thinking/effort configuration from CLI flags.
	thinkingMode := ""
	if *thinkingFlag != "" {
		thinkingMode = *thinkingFlag
	} else if *effortFlag != "" {
		// Effort maps to thinking: low/medium = disabled, high/max = enabled.
		switch *effortFlag {
		case "low", "medium":
			thinkingMode = "disabled"
		case "high", "max":
			thinkingMode = "enabled"
		}
	} else if settings.ThinkingEnabled != nil && *settings.ThinkingEnabled {
		thinkingMode = "enabled"
	}
	if thinkingMode == "enabled" {
		thinkingTokens := *maxThinkingTokens
		if thinkingTokens == 0 {
			thinkingTokens = 10000 // default thinking budget
		}
		loop.SetThinking(&api.ThinkingConfig{
			Type:         "enabled",
			BudgetTokens: thinkingTokens,
		})
	}

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

	// Apply max-turns for print mode.
	if *maxTurnsFlag > 0 {
		loop.SetMaxTurns(*maxTurnsFlag)
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
		Loop:        loop,
		Session:     currentSession,
		SessStore:   sessionStore,
		Version:     version,
		Model:       model,
		Cwd:         cwd,
		BillingType: billingType,
		MCPManager:  mcpManager,
		Skills:      loadedSkills,  // Phase 7
		Hooks:       hookRunner,    // Phase 7
		Settings:    settings,
		RuleHandler: ruleHandler,
		OnModelSwitch: func(newModel string) {
			if currentSession != nil {
				currentSession.Model = newModel
			}
		},
		LogoutFunc: func() error { return store.Delete() },
		FastMode:   fastMode,
		Client:     client,
	})

	if initialPrompt != "" {
		app.SetInitialPrompt(initialPrompt)
	}

	if err := app.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Handle /login: the TUI exited requesting a re-authentication flow.
	if app.ExitAction() == tui.ExitLogin {
		loginCtx, loginCancel := context.WithCancel(context.Background())
		defer loginCancel()
		if err := doLogin(loginCtx, store, auth.LoginOptions{}); err != nil {
			fmt.Fprintf(os.Stderr, "Login failed: %v\n", err)
			os.Exit(1)
		}
	}
}

// runStatus executes the status subcommand. Output is JSON by default (matching
// the JS version); use --text for human-readable output.
func runStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	jsonFlag := fs.Bool("json", false, "Output as JSON (default)")
	textFlag := fs.Bool("text", false, "Output as human-readable text")
	fs.Parse(args)

	store, err := auth.NewCredentialStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	status := auth.GetAuthStatus(store)

	if *textFlag {
		fmt.Println(auth.FormatStatusText(status))
	} else {
		// JSON is the default (--json flag is accepted but optional).
		_ = jsonFlag
		output, err := auth.FormatStatusJSON(status)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(output)
	}

	if !status.LoggedIn {
		os.Exit(1)
	}
}

// runLogin handles the `claude login` subcommand.
// Matches the JS: claude login [--email <email>] [--sso]
func runLogin(args []string) {
	loginFS := flag.NewFlagSet("login", flag.ExitOnError)
	loginFS.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: claude login [--email <email>] [--sso]\n\nSign in to your Anthropic account.\n\nOptions:\n")
		loginFS.PrintDefaults()
	}
	email := loginFS.String("email", "", "Pre-populate email address on the login page")
	sso := loginFS.Bool("sso", false, "Force SSO login flow")
	loginFS.Parse(args)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		cancel()
	}()

	store, err := auth.NewCredentialStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	opts := auth.LoginOptions{
		Email: *email,
		SSO:   *sso,
	}
	if err := doLogin(ctx, store, opts); err != nil {
		fmt.Fprintf(os.Stderr, "Login failed: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

// runLogout handles the `claude logout` subcommand.
// Matches the JS: claude logout
func runLogout() {
	store, err := auth.NewCredentialStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := doLogout(store); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to log out.\n")
		os.Exit(1)
	}
	fmt.Println("Successfully logged out from your Anthropic account.")
	os.Exit(0)
}

func doLogin(ctx context.Context, store *auth.CredentialStore, opts auth.LoginOptions) error {
	flow, err := auth.NewOAuthFlow()
	if err != nil {
		return fmt.Errorf("initializing OAuth flow: %w", err)
	}
	result, err := flow.Login(ctx, opts)
	if err != nil {
		return err
	}

	if err := store.Save(result.Tokens); err != nil {
		return fmt.Errorf("saving tokens: %w", err)
	}

	// Store account metadata.
	if result.Account != nil {
		if err := store.SaveAccount(result.Account); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save account info: %v\n", err)
		}
	}

	// Store API key.
	if result.APIKey != "" {
		if err := store.SaveAPIKey(result.APIKey); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save API key: %v\n", err)
		}
	}

	fmt.Println("Login successful.")
	return nil
}

func doLogout(store *auth.CredentialStore) error {
	return store.Delete()
}

// runUpdate handles the `claude update` subcommand.
// Since this is a Go binary, we point users to the package manager or release page.
func runUpdate(args []string) {
	fmt.Println("Claude Code (Go)")
	fmt.Printf("Current version: %s\n\n", version)
	fmt.Println("To update, download the latest release from the project repository")
	fmt.Println("or use your package manager to update the binary.")
	fmt.Println()
	fmt.Println("If installed via go install:")
	fmt.Println("  go install github.com/anthropics/claude-code-go/cmd/claude@latest")
}

// runMCP handles the `claude mcp` subcommand for MCP server management.
func runMCP(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: claude mcp <command> [options]")
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("  list              List configured MCP servers")
		fmt.Println("  add <name> <cmd>  Add an MCP server")
		fmt.Println("  remove <name>     Remove an MCP server")
		return
	}

	cwd, _ := os.Getwd()

	switch args[0] {
	case "list":
		mcpCfg, err := mcp.LoadMCPConfig(cwd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading MCP config: %v\n", err)
			os.Exit(1)
		}
		if mcpCfg == nil || len(mcpCfg.MCPServers) == 0 {
			fmt.Println("No MCP servers configured.")
			return
		}
		fmt.Println("Configured MCP servers:")
		for name, cfg := range mcpCfg.MCPServers {
			fmt.Printf("  %s: %s %v\n", name, cfg.Command, cfg.Args)
		}

	case "add":
		if len(args) < 3 {
			fmt.Println("Usage: claude mcp add <name> <command> [args...]")
			os.Exit(1)
		}
		name := args[1]
		command := args[2]
		cmdArgs := args[3:]
		if err := mcp.AddServerToConfig(cwd, name, command, cmdArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Error adding MCP server: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Added MCP server: %s\n", name)

	case "remove":
		if len(args) < 2 {
			fmt.Println("Usage: claude mcp remove <name>")
			os.Exit(1)
		}
		name := args[1]
		if err := mcp.RemoveServerFromConfig(cwd, name); err != nil {
			fmt.Fprintf(os.Stderr, "Error removing MCP server: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Removed MCP server: %s\n", name)

	default:
		fmt.Fprintf(os.Stderr, "Unknown mcp command: %s\n", args[0])
		os.Exit(1)
	}
}

// runAgents handles the `claude agents` subcommand.
func runAgents() {
	fmt.Println("Configured agents:")
	fmt.Println("  (No custom agents configured)")
	fmt.Println()
	fmt.Println("Agents can be configured in .claude/settings.json")
}

// showBypassPermissionsWarning displays a warning dialog for bypass permissions mode.
// Returns true if the user accepts, false if they decline.
func showBypassPermissionsWarning() bool {
	// Red/bold warning header.
	fmt.Println()
	fmt.Println("\033[1;31mWARNING: Claude Code running in Bypass Permissions mode\033[0m")
	fmt.Println()
	fmt.Println("In Bypass Permissions mode, Claude Code will not ask for your")
	fmt.Println("approval before running potentially dangerous commands.")
	fmt.Println()
	fmt.Println("This mode should only be used in a sandboxed container/VM that")
	fmt.Println("has restricted internet access and can easily be restored if damaged.")
	fmt.Println()
	fmt.Println("By proceeding, you accept all responsibility for actions taken while")
	fmt.Println("running in Bypass Permissions mode.")
	fmt.Println()
	fmt.Print("Accept and proceed? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes"
}
