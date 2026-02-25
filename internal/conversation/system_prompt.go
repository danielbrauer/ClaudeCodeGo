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

// PromptContext holds all data that prompt sections may need.
type PromptContext struct {
	CWD          string
	Settings     *config.Settings
	SkillContent string
	AgentMode    bool // toggle agent-specific sections
}

// PromptSection generates a portion of the system prompt.
// Return empty string to skip the section.
type PromptSection func(ctx *PromptContext) string

// coreSections are stable sections included in Block 1 (cache-friendly).
var coreSections = []PromptSection{
	sectionIdentity,
	sectionEnvironment,
}

// projectSections are project-specific sections included in Block 2.
var projectSections = []PromptSection{
	sectionClaudeMD,
	sectionSkills,
	sectionPermissions,
}

// RegisterCoreSection appends a section to Block 1 (identity/environment).
func RegisterCoreSection(s PromptSection) {
	coreSections = append(coreSections, s)
}

// RegisterProjectSection appends a section to Block 2 (project-specific).
func RegisterProjectSection(s PromptSection) {
	projectSections = append(projectSections, s)
}

// BuildSystemPrompt assembles the system prompt blocks from CLAUDE.md files,
// environment context, settings, and active skill content.
//
// The prompt is split into two blocks for prompt caching efficiency:
//   - Block 1: Core identity and environment (stable across projects/sessions)
//   - Block 2: Project-specific content (CLAUDE.md, skills, permissions)
//
// Cache control headers are applied separately by the caching layer.
func BuildSystemPrompt(cwd string, settings *config.Settings, skillContent string) []api.SystemBlock {
	ctx := &PromptContext{
		CWD:          cwd,
		Settings:     settings,
		SkillContent: skillContent,
	}

	var blocks []api.SystemBlock

	if coreText := renderSections(coreSections, ctx); coreText != "" {
		blocks = append(blocks, api.SystemBlock{Type: "text", Text: coreText})
	}

	if projectText := renderSections(projectSections, ctx); projectText != "" {
		blocks = append(blocks, api.SystemBlock{Type: "text", Text: projectText})
	}

	return blocks
}

// renderSections calls each section function and joins non-empty results.
func renderSections(sections []PromptSection, ctx *PromptContext) string {
	var parts []string
	for _, s := range sections {
		if text := s(ctx); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}

// --- Core sections (Block 1) ---

func sectionIdentity(_ *PromptContext) string {
	return "You are Claude Code, an interactive CLI tool that helps users with software engineering tasks.\n" +
		"You have access to tools that let you read files, write files, execute commands, and more."
}

func sectionEnvironment(ctx *PromptContext) string {
	return fmt.Sprintf(
		"Environment:\n- Working directory: %s\n- Platform: %s/%s\n- Date: %s",
		ctx.CWD, runtime.GOOS, runtime.GOARCH, time.Now().Format("2006-01-02"),
	)
}

// --- Project sections (Block 2) ---

func sectionClaudeMD(ctx *PromptContext) string {
	content := config.LoadClaudeMD(ctx.CWD)
	if content == "" {
		return ""
	}
	return "# Project Instructions (CLAUDE.md)\n\n" + content
}

func sectionSkills(ctx *PromptContext) string {
	if ctx.SkillContent == "" {
		return ""
	}
	return "# Active Skills\n\n" + ctx.SkillContent
}

func sectionPermissions(ctx *PromptContext) string {
	if ctx.Settings == nil || len(ctx.Settings.Permissions) == 0 {
		return ""
	}
	summary := formatPermissionRules(ctx.Settings.Permissions)
	if summary == "" {
		return ""
	}
	return "# Permission Rules\n\n" + summary
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
