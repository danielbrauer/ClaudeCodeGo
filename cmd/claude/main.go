// Package main is the entry point for the claude CLI.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/auth"
	"github.com/anthropics/claude-code-go/internal/conversation"
	"github.com/anthropics/claude-code-go/internal/tools"
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

	// Resolve model.
	model := api.ModelClaude4Sonnet
	if *modelFlag != "" {
		if resolved, ok := api.ModelAliases[*modelFlag]; ok {
			model = resolved
		} else {
			model = *modelFlag
		}
	}

	// Create API client.
	client := api.NewClient(
		tokenProvider,
		api.WithModel(model),
		api.WithMaxTokens(*maxTokens),
	)

	// Working directory.
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting working directory: %v\n", err)
		os.Exit(1)
	}

	// Build system prompt.
	system := conversation.BuildSystemPrompt(cwd)

	// Set up permission handler.
	var permHandler tools.PermissionHandler
	if *dangerousNoPermissions {
		permHandler = &tools.AlwaysAllowPermissionHandler{}
	} else {
		permHandler = tools.NewTerminalPermissionHandler()
	}

	// Create tool registry with all Phase 2 tools.
	registry := tools.NewRegistry(permHandler)
	registry.Register(tools.NewBashTool(cwd))
	registry.Register(tools.NewFileReadTool())
	registry.Register(tools.NewFileEditTool())
	registry.Register(tools.NewFileWriteTool())
	registry.Register(tools.NewGlobTool(cwd))
	registry.Register(tools.NewGrepTool(cwd))

	// Create conversation loop with tools.
	handler := &conversation.ToolAwareStreamHandler{}
	loop := conversation.NewLoop(conversation.LoopConfig{
		Client:   client,
		System:   system,
		Tools:    registry.Definitions(),
		ToolExec: registry,
		Handler:  handler,
	})

	// Handle initial prompt from arguments.
	args := flag.Args()
	if len(args) > 0 {
		prompt := strings.Join(args, " ")
		if err := loop.SendMessage(ctx, prompt); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if *printMode {
			os.Exit(0)
		}
	}

	// Silence unused flag warnings for future features.
	_ = continueFlag
	_ = resumeFlag

	// Interactive REPL.
	if *printMode {
		os.Exit(0)
	}

	fmt.Printf("\nclaude %s (Go) | model: %s\n", version, client.Model())
	fmt.Println("Type your message. Press Ctrl+C to exit.")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Basic slash commands.
		if strings.HasPrefix(line, "/") {
			switch {
			case line == "/help":
				printHelp()
				continue
			case line == "/model":
				fmt.Printf("Current model: %s\n", client.Model())
				continue
			case line == "/quit", line == "/exit":
				fmt.Println("Goodbye.")
				os.Exit(0)
			case line == "/version":
				fmt.Printf("claude %s (Go)\n", version)
				continue
			default:
				fmt.Printf("Unknown command: %s (type /help for available commands)\n", line)
				continue
			}
		}

		if err := loop.SendMessage(ctx, line); err != nil {
			if ctx.Err() != nil {
				fmt.Println("\nInterrupted.")
				os.Exit(0)
			}
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		fmt.Println()
	}
}

func doLogin(ctx context.Context, store *auth.CredentialStore) error {
	flow := auth.NewOAuthFlow()
	tokens, err := flow.Login(ctx)
	if err != nil {
		return err
	}

	if err := store.Save(tokens); err != nil {
		return fmt.Errorf("saving tokens: %w", err)
	}

	fmt.Println("Login successful!")
	return nil
}

func printHelp() {
	fmt.Println(`Available commands:
  /help     - Show this help
  /model    - Show current model
  /version  - Show version
  /quit     - Exit

CLI flags:
  --model <model>                   Model to use (opus, sonnet, haiku)
  --max-tokens <n>                  Maximum response tokens
  -p                                Print mode (non-interactive)
  -c                                Continue most recent session
  -r <id>                           Resume session by ID
  --login                           Log in with OAuth
  --version                         Print version
  --dangerously-skip-permissions    Skip all permission prompts`)
}
