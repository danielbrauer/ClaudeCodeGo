package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/anthropics/claude-code-go/internal/config"
)

// registerDoctorCommand registers /doctor.
func registerDoctorCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "doctor",
		Description: "Diagnose configuration and auth issues",
		Execute:     textCommand(doctorText),
	})
}

func doctorText(m *model) string {
	var b strings.Builder
	b.WriteString("Claude Code Doctor\n")
	b.WriteString("==================\n\n")

	// Check auth.
	b.WriteString("Authentication: ")
	if m.apiClient != nil {
		b.WriteString("OK (logged in)\n")
	} else {
		b.WriteString("WARNING - no API client\n")
	}

	// Check model.
	b.WriteString(fmt.Sprintf("Model: %s\n", m.modelName))

	// Check settings.
	if m.settings != nil {
		if len(m.settings.Permissions) > 0 {
			b.WriteString(fmt.Sprintf("Permission rules: %d configured\n", len(m.settings.Permissions)))
		} else {
			b.WriteString("Permission rules: none\n")
		}
		if m.settings.EditorMode != "" {
			b.WriteString(fmt.Sprintf("Editor mode: %s\n", m.settings.EditorMode))
		}
	} else {
		b.WriteString("Settings: not loaded\n")
	}

	// Check MCP.
	if m.mcpStatus != nil {
		b.WriteString("MCP: configured\n")
	} else {
		b.WriteString("MCP: not configured\n")
	}

	// Check version.
	b.WriteString(fmt.Sprintf("Version: %s\n", m.version))

	b.WriteString("\nNo issues detected.")
	return b.String()
}

// registerThemeCommand registers /theme.
func registerThemeCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "theme",
		Description: "Change color theme",
		Execute:     executeTheme,
	})
}

func executeTheme(m *model, args string) (tea.Model, tea.Cmd) {
	args = strings.TrimSpace(args)
	if args == "" {
		current := "default"
		if m.settings != nil && m.settings.Theme != "" {
			current = m.settings.Theme
		}
		return *m, tea.Println(fmt.Sprintf("Current theme: %s\nAvailable: default, dark, light\nUsage: /theme <name>", current))
	}

	// Validate theme name.
	switch args {
	case "default", "dark", "light":
		if m.settings != nil {
			m.settings.Theme = args
		}
		_ = config.SaveUserSetting("theme", args)
		return *m, tea.Println(fmt.Sprintf("Theme set to: %s", args))
	default:
		return *m, tea.Println(fmt.Sprintf("Unknown theme: %s. Available: default, dark, light", args))
	}
}

// registerVimCommand registers /vim.
func registerVimCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "vim",
		Description: "Toggle vim input mode",
		Execute:     executeVim,
	})
}

func executeVim(m *model, args string) (tea.Model, tea.Cmd) {
	current := "normal"
	if m.settings != nil && m.settings.EditorMode == "vim" {
		current = "vim"
	}

	newMode := "vim"
	if current == "vim" {
		newMode = "normal"
	}

	if m.settings != nil {
		m.settings.EditorMode = newMode
	}
	_ = config.SaveUserSetting("editorMode", newMode)

	if newMode == "vim" {
		return *m, tea.Println("Vim mode ON")
	}
	return *m, tea.Println("Vim mode OFF")
}

// registerPermissionsCommand registers /permissions.
func registerPermissionsCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "permissions",
		Description: "View/edit permission rules",
		Execute:     textCommand(permissionsText),
	})
}

func permissionsText(m *model) string {
	if m.settings == nil || len(m.settings.Permissions) == 0 {
		return "No permission rules configured.\n\nAdd rules in .claude/settings.json or ~/.claude/settings.json"
	}

	var b strings.Builder
	b.WriteString("Permission Rules\n")
	b.WriteString("================\n\n")
	for _, rule := range m.settings.Permissions {
		desc := config.FormatRuleString(rule)
		b.WriteString(fmt.Sprintf("  %s: %s\n", desc, rule.Action))
	}
	return b.String()
}

// registerHooksCommand registers /hooks.
func registerHooksCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "hooks",
		Description: "View configured hooks",
		Execute:     textCommand(hooksText),
	})
}

func hooksText(m *model) string {
	if m.settings == nil || m.settings.Hooks == nil {
		return "No hooks configured.\n\nAdd hooks in .claude/settings.json or ~/.claude/settings.json"
	}

	var hookConfig map[string]json.RawMessage
	if err := json.Unmarshal(m.settings.Hooks, &hookConfig); err != nil {
		return "Error parsing hooks configuration."
	}

	if len(hookConfig) == 0 {
		return "No hooks configured."
	}

	var b strings.Builder
	b.WriteString("Configured Hooks\n")
	b.WriteString("================\n\n")
	for event, defs := range hookConfig {
		var hooks []map[string]string
		if err := json.Unmarshal(defs, &hooks); err != nil {
			b.WriteString(fmt.Sprintf("  %s: (parse error)\n", event))
			continue
		}
		b.WriteString(fmt.Sprintf("  %s: %d hook(s)\n", event, len(hooks)))
		for _, h := range hooks {
			if cmd, ok := h["command"]; ok {
				b.WriteString(fmt.Sprintf("    - command: %s\n", cmd))
			} else if prompt, ok := h["prompt"]; ok {
				truncated := prompt
				if len(truncated) > 60 {
					truncated = truncated[:57] + "..."
				}
				b.WriteString(fmt.Sprintf("    - prompt: %s\n", truncated))
			}
		}
	}
	return b.String()
}

// registerStatusCommand registers /status.
func registerStatusCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "status",
		Description: "Show session status",
		Execute:     textCommand(statusText),
	})
}

func statusText(m *model) string {
	var b strings.Builder
	b.WriteString("Session Status\n")
	b.WriteString("==============\n\n")
	b.WriteString(fmt.Sprintf("Model: %s\n", m.modelName))

	if m.fastMode {
		b.WriteString("Fast mode: ON\n")
	} else {
		b.WriteString("Fast mode: OFF\n")
	}

	b.WriteString(fmt.Sprintf("Messages: %d\n", m.loop.History().Len()))
	b.WriteString(fmt.Sprintf("Tokens in: %d / out: %d\n", m.tokens.TotalInputTokens, m.tokens.TotalOutputTokens))

	if m.tokens.TotalCostUSD > 0 {
		b.WriteString(fmt.Sprintf("Cost: $%.4f\n", m.tokens.TotalCostUSD))
	}

	return b.String()
}
