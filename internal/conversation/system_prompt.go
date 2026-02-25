// Package conversation manages the agentic conversation loop.
package conversation

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/config"
)

// PromptContext carries all inputs that section generators may need.
// Adding new fields here makes them available to all sections without
// changing the PromptSection signature.
type PromptContext struct {
	Cwd          string
	Settings     *config.Settings
	SkillContent string
	IsAgent      bool // true when building prompt for a sub-agent
}

// PromptSection is a function that produces one section of the system prompt.
// Return "" to omit the section. Sections are joined with "\n" within each block.
type PromptSection func(ctx *PromptContext) string

// sectionEntry pairs a section function with a name (for debugging/testing).
type sectionEntry struct {
	Name    string
	Section PromptSection
}

var (
	coreSectionsMu sync.Mutex
	coreSections   []sectionEntry

	projectSectionsMu sync.Mutex
	projectSections   []sectionEntry
)

// RegisterCoreSection adds a section to the core prompt block (block 1).
// Core sections produce stable, project-independent content that benefits
// from prompt caching. Order of registration determines order in the prompt.
func RegisterCoreSection(name string, s PromptSection) {
	coreSectionsMu.Lock()
	defer coreSectionsMu.Unlock()
	coreSections = append(coreSections, sectionEntry{Name: name, Section: s})
}

// RegisterProjectSection adds a section to the project prompt block (block 2).
// Project sections produce content that varies by project/session.
// Order of registration determines order in the prompt.
func RegisterProjectSection(name string, s PromptSection) {
	projectSectionsMu.Lock()
	defer projectSectionsMu.Unlock()
	projectSections = append(projectSections, sectionEntry{Name: name, Section: s})
}

// renderSections evaluates a slice of section entries and returns the non-empty results.
func renderSections(sections []sectionEntry, ctx *PromptContext) []string {
	var parts []string
	for _, entry := range sections {
		if text := entry.Section(ctx); text != "" {
			parts = append(parts, text)
		}
	}
	return parts
}

// BuildSystemPrompt assembles the system prompt blocks from registered sections.
//
// The prompt is split into two blocks for prompt caching efficiency:
//   - Block 1: Core identity and environment (stable across projects/sessions)
//   - Block 2: Project-specific content (CLAUDE.md, skills, permissions)
//
// Cache control headers are applied separately by the caching layer.
func BuildSystemPrompt(cwd string, settings *config.Settings, skillContent string) []api.SystemBlock {
	ctx := &PromptContext{
		Cwd:          cwd,
		Settings:     settings,
		SkillContent: skillContent,
	}

	var blocks []api.SystemBlock

	// Block 1: Core sections.
	if parts := renderSections(coreSections, ctx); len(parts) > 0 {
		blocks = append(blocks, api.SystemBlock{
			Type: "text",
			Text: strings.Join(parts, "\n"),
		})
	}

	// Block 2: Project sections.
	if parts := renderSections(projectSections, ctx); len(parts) > 0 {
		blocks = append(blocks, api.SystemBlock{
			Type: "text",
			Text: strings.Join(parts, "\n\n"),
		})
	}

	return blocks
}

// resetSections clears all registered sections. Only for testing.
func resetSections() {
	coreSectionsMu.Lock()
	coreSections = nil
	coreSectionsMu.Unlock()

	projectSectionsMu.Lock()
	projectSections = nil
	projectSectionsMu.Unlock()
}

// --- Built-in section functions ---
// Each init() registers the default sections in the correct order.

func init() {
	// Core sections (block 1).
	RegisterCoreSection("identity", identitySection)
	RegisterCoreSection("environment", environmentSection)

	// Project sections (block 2).
	RegisterProjectSection("claudemd", claudeMDSection)
	RegisterProjectSection("skills", skillsSection)
	RegisterProjectSection("permissions", permissionsSection)
}

// identitySection produces the agent identity preamble.
func identitySection(_ *PromptContext) string {
	return "You are Claude Code, an interactive CLI tool that helps users with software engineering tasks.\n" +
		"You have access to tools that let you read files, write files, execute commands, and more."
}

// environmentSection produces the runtime environment block.
func environmentSection(ctx *PromptContext) string {
	return fmt.Sprintf(
		"\nEnvironment:\n- Working directory: %s\n- Platform: %s/%s\n- Date: %s",
		ctx.Cwd, runtime.GOOS, runtime.GOARCH, time.Now().Format("2006-01-02"),
	)
}

// claudeMDSection loads and injects CLAUDE.md content from the project.
func claudeMDSection(ctx *PromptContext) string {
	content := config.LoadClaudeMD(ctx.Cwd)
	if content == "" {
		return ""
	}
	return "# Project Instructions (CLAUDE.md)\n\n" + content
}

// skillsSection injects active skill content.
func skillsSection(ctx *PromptContext) string {
	if ctx.SkillContent == "" {
		return ""
	}
	return "# Active Skills\n\n" + ctx.SkillContent
}

// permissionsSection formats configured permission rules for the prompt.
func permissionsSection(ctx *PromptContext) string {
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
