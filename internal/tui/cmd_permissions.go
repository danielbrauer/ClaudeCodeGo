package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/anthropics/claude-code-go/internal/config"
)

// registerPermissionsCommand registers /permissions (and alias /mode).
func registerPermissionsCommand(r *slashRegistry) {
	r.register(SlashCommand{
		Name:        "permissions",
		Description: "Show or change permission mode",
		Execute:     executePermissions,
	})
	// Alias: /mode
	r.register(SlashCommand{
		Name:        "mode",
		Description: "Show or change permission mode",
		IsAlias:     true,
		Execute:     executePermissions,
	})
}

func executePermissions(m *model, args string) (tea.Model, tea.Cmd) {
	args = strings.TrimSpace(args)

	// No args: show current mode and help text.
	if args == "" {
		return *m, tea.Println(renderPermissionModeHelp(m))
	}

	// Check for explicit mode name.
	target := config.PermissionMode(args)
	if !config.ValidPermissionMode(args) {
		msg := fmt.Sprintf("Unknown permission mode: %q\nValid modes: default, plan, acceptEdits, bypassPermissions", args)
		return *m, tea.Println(msg)
	}

	// Check if trying to enter bypass mode without it being available.
	if target == config.ModeBypassPermissions && !m.isBypassAvailable() {
		msg := "Bypass permissions mode is not available. Use --allow-dangerously-skip-permissions to enable it."
		return *m, tea.Println(msg)
	}

	m.setPermissionMode(target)
	info := config.ModeInfoMap[target]
	line := renderModeChangeLine(info)
	return *m, tea.Println(line)
}

// handleCyclePermissionMode is called by Shift+Tab to cycle through modes.
func (m model) handleCyclePermissionMode() (tea.Model, tea.Cmd) {
	if m.ruleHandler == nil {
		return m, nil
	}

	next := m.cyclePermissionMode()
	info := config.ModeInfoMap[next]
	line := renderModeChangeLine(info)
	return m, tea.Println(line)
}

// renderModeChangeLine produces the scrollback output when the mode changes.
func renderModeChangeLine(info config.PermissionModeInfo) string {
	symbol := info.Symbol
	if symbol != "" {
		symbol += " "
	}

	var style func(string) string
	switch info.Mode {
	case config.ModeBypassPermissions:
		style = func(s string) string { return errorStyle.Render(s) }
	case config.ModePlan:
		style = func(s string) string { return planModeStyle.Render(s) }
	case config.ModeAcceptEdits:
		style = func(s string) string { return autoAcceptStyle.Render(s) }
	default:
		style = func(s string) string { return permHintStyle.Render(s) }
	}

	return style(fmt.Sprintf("%sPermission mode: %s", symbol, info.Title))
}

// renderPermissionModeHelp produces detailed help about the current mode.
func renderPermissionModeHelp(m *model) string {
	current := m.getPermissionMode()
	info := config.ModeInfoMap[current]

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Current permission mode: %s", info.Title))
	if info.Symbol != "" {
		b.WriteString(fmt.Sprintf(" %s", info.Symbol))
	}
	b.WriteString("\n")

	b.WriteString("\nAvailable modes:\n")
	b.WriteString("  default          - Ask for approval before dangerous operations\n")
	b.WriteString("  acceptEdits      - Auto-approve file edits, still ask for other tools\n")
	b.WriteString("  plan             - Read-only mode; Claude can only plan, not execute\n")
	if m.isBypassAvailable() {
		b.WriteString("  bypassPermissions - Skip all permission checks (dangerous)\n")
	}

	b.WriteString("\nUsage: /permissions <mode>")
	b.WriteString("\nKeyboard: Shift+Tab to cycle through modes")

	return b.String()
}
