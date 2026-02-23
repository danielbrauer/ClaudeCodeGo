// Package conversation manages the agentic conversation loop.
package conversation

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/config"
)

// BuildSystemPrompt assembles the system prompt blocks from CLAUDE.md files,
// environment context, settings, and active skill content.
func BuildSystemPrompt(cwd string, settings *config.Settings, skillContent string) []api.SystemBlock {
	var parts []string

	// Core identity.
	parts = append(parts, "You are Claude Code, an interactive CLI tool that helps users with software engineering tasks.")
	parts = append(parts, "You have access to tools that let you read files, write files, execute commands, and more.")

	// Environment info.
	parts = append(parts, fmt.Sprintf(
		"\nEnvironment:\n- Working directory: %s\n- Platform: %s/%s\n- Date: %s",
		cwd, runtime.GOOS, runtime.GOARCH, time.Now().Format("2006-01-02"),
	))

	// Load CLAUDE.md content using the enhanced loader.
	claudeMDContent := config.LoadClaudeMD(cwd)
	if claudeMDContent != "" {
		parts = append(parts, "\n# Project Instructions (CLAUDE.md)\n\n"+claudeMDContent)
	}

	// Phase 7: Inject skill instructions.
	if skillContent != "" {
		parts = append(parts, "\n# Active Skills\n\n"+skillContent)
	}

	// Inject permission rule summary if any rules are configured.
	if settings != nil && len(settings.Permissions) > 0 {
		rulesSummary := formatPermissionRules(settings.Permissions)
		if rulesSummary != "" {
			parts = append(parts, "\n# Permission Rules\n\n"+rulesSummary)
		}
	}

	return []api.SystemBlock{
		{
			Type: "text",
			Text: strings.Join(parts, "\n"),
		},
	}
}

// formatPermissionRules creates a human-readable summary of permission rules
// for inclusion in the system prompt. Uses the JS string format for
// compatibility (e.g., "Bash(npm:*): allow").
func formatPermissionRules(rules []config.PermissionRule) string {
	if len(rules) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, "The following permission rules are configured:")
	for _, rule := range rules {
		desc := config.FormatRuleString(rule)
		desc += ": " + rule.Action
		lines = append(lines, "- "+desc)
	}
	return strings.Join(lines, "\n")
}
