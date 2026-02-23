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
//
// The prompt is split into two blocks for prompt caching efficiency:
//   - Block 1: Core identity and environment (stable across projects/sessions)
//   - Block 2: Project-specific content (CLAUDE.md, skills, permissions)
//
// Cache control headers are applied separately by the caching layer.
func BuildSystemPrompt(cwd string, settings *config.Settings, skillContent string) []api.SystemBlock {
	// Block 1: Core identity + environment (mostly static).
	var identityParts []string
	identityParts = append(identityParts, "You are Claude Code, an interactive CLI tool that helps users with software engineering tasks.")
	identityParts = append(identityParts, "You have access to tools that let you read files, write files, execute commands, and more.")
	identityParts = append(identityParts, fmt.Sprintf(
		"\nEnvironment:\n- Working directory: %s\n- Platform: %s/%s\n- Date: %s",
		cwd, runtime.GOOS, runtime.GOARCH, time.Now().Format("2006-01-02"),
	))

	blocks := []api.SystemBlock{
		{Type: "text", Text: strings.Join(identityParts, "\n")},
	}

	// Block 2: Project-specific content (CLAUDE.md, skills, permissions).
	var projectParts []string

	claudeMDContent := config.LoadClaudeMD(cwd)
	if claudeMDContent != "" {
		projectParts = append(projectParts, "# Project Instructions (CLAUDE.md)\n\n"+claudeMDContent)
	}

	if skillContent != "" {
		projectParts = append(projectParts, "# Active Skills\n\n"+skillContent)
	}

	if settings != nil && len(settings.Permissions) > 0 {
		rulesSummary := formatPermissionRules(settings.Permissions)
		if rulesSummary != "" {
			projectParts = append(projectParts, "# Permission Rules\n\n"+rulesSummary)
		}
	}

	if len(projectParts) > 0 {
		blocks = append(blocks, api.SystemBlock{
			Type: "text",
			Text: strings.Join(projectParts, "\n\n"),
		})
	}

	return blocks
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
