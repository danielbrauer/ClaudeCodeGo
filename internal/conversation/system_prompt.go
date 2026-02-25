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

// PromptContext carries all the data that prompt sections may need.
type PromptContext struct {
	CWD          string
	Settings     *config.Settings
	SkillContent string
}

// PromptSection is a function that returns a section of the system prompt.
// It returns empty string if the section should be omitted.
type PromptSection func(ctx PromptContext) string

// namedSection pairs a section function with its registration name.
type namedSection struct {
	name string
	fn   PromptSection
}

var (
	coreSections    []namedSection
	projectSections []namedSection
	sectionsMu      sync.Mutex
)

// RegisterCoreSection registers a prompt section into the core (identity) block.
// Core sections are stable across projects/sessions and form the first system block.
// The name parameter is used for identification and testing.
func RegisterCoreSection(name string, fn PromptSection) {
	sectionsMu.Lock()
	defer sectionsMu.Unlock()
	coreSections = append(coreSections, namedSection{name: name, fn: fn})
}

// RegisterProjectSection registers a prompt section into the project-specific block.
// Project sections contain dynamic, per-project content and form the second system block.
// The name parameter is used for identification and testing.
func RegisterProjectSection(name string, fn PromptSection) {
	sectionsMu.Lock()
	defer sectionsMu.Unlock()
	projectSections = append(projectSections, namedSection{name: name, fn: fn})
}

// resetSections clears all registered sections. Used only in tests.
func resetSections() {
	sectionsMu.Lock()
	defer sectionsMu.Unlock()
	coreSections = nil
	projectSections = nil
}

// renderSections evaluates a set of named sections and joins non-empty results.
func renderSections(sections []namedSection, ctx PromptContext, sep string) string {
	var parts []string
	for _, s := range sections {
		if content := s.fn(ctx); content != "" {
			parts = append(parts, content)
		}
	}
	return strings.Join(parts, sep)
}

// BuildSystemPrompt assembles the system prompt blocks from registered sections.
//
// The prompt is split into two blocks for prompt caching efficiency:
//   - Block 1: Core identity and environment (stable across projects/sessions)
//   - Block 2: Project-specific content (CLAUDE.md, skills, permissions)
//
// Cache control headers are applied separately by the caching layer.
func BuildSystemPrompt(cwd string, settings *config.Settings, skillContent string) []api.SystemBlock {
	ctx := PromptContext{
		CWD:          cwd,
		Settings:     settings,
		SkillContent: skillContent,
	}

	sectionsMu.Lock()
	core := make([]namedSection, len(coreSections))
	copy(core, coreSections)
	project := make([]namedSection, len(projectSections))
	copy(project, projectSections)
	sectionsMu.Unlock()

	blocks := []api.SystemBlock{
		{Type: "text", Text: renderSections(core, ctx, "\n")},
	}

	projectText := renderSections(project, ctx, "\n\n")
	if projectText != "" {
		blocks = append(blocks, api.SystemBlock{
			Type: "text",
			Text: projectText,
		})
	}

	return blocks
}

// --- Core sections (registered at init time) ---

func init() {
	RegisterCoreSection("identity", identitySection)
	RegisterCoreSection("environment", environmentSection)

	RegisterProjectSection("claudemd", claudeMDSection)
	RegisterProjectSection("skills", skillsSection)
	RegisterProjectSection("permissions", permissionsSection)
}

func identitySection(_ PromptContext) string {
	return "You are Claude Code, an interactive CLI tool that helps users with software engineering tasks.\n" +
		"You have access to tools that let you read files, write files, execute commands, and more."
}

func environmentSection(ctx PromptContext) string {
	return fmt.Sprintf(
		"\nEnvironment:\n- Working directory: %s\n- Platform: %s/%s\n- Date: %s",
		ctx.CWD, runtime.GOOS, runtime.GOARCH, time.Now().Format("2006-01-02"),
	)
}

func claudeMDSection(ctx PromptContext) string {
	content := config.LoadClaudeMD(ctx.CWD)
	if content == "" {
		return ""
	}
	return "# Project Instructions (CLAUDE.md)\n\n" + content
}

func skillsSection(ctx PromptContext) string {
	if ctx.SkillContent == "" {
		return ""
	}
	return "# Active Skills\n\n" + ctx.SkillContent
}

func permissionsSection(ctx PromptContext) string {
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
