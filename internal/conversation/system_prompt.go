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
	sectionSecurityGuardrails,
	sectionTaskPhilosophy,
	sectionActionCare,
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

func sectionSecurityGuardrails(_ *PromptContext) string {
	return "IMPORTANT: Assist with authorized security testing, defensive security, CTF challenges, and educational contexts. " +
		"Refuse requests for destructive techniques, DoS attacks, mass targeting, supply chain compromise, or detection evasion for malicious purposes. " +
		"Dual-use security tools (C2 frameworks, credential testing, exploit development) require clear authorization context: " +
		"pentesting engagements, CTF competitions, security research, or defensive use cases."
}

func sectionTaskPhilosophy(_ *PromptContext) string {
	return `# Doing tasks
- Avoid giving time estimates or predictions for how long tasks will take, whether for your own work or for users planning projects. Focus on what needs to be done, not how long it might take.
- Be careful not to introduce security vulnerabilities such as command injection, XSS, SQL injection, and other OWASP top 10 vulnerabilities. If you notice that you wrote insecure code, immediately fix it. Prioritize writing safe, secure, and correct code.
- Avoid over-engineering. Only make changes that are directly requested or clearly necessary. Keep solutions simple and focused.
  - Don't add features, refactor code, or make "improvements" beyond what was asked. A bug fix doesn't need surrounding code cleaned up. A simple feature doesn't need extra configurability. Don't add docstrings, comments, or type annotations to code you didn't change. Only add comments where the logic isn't self-evident.
  - Don't add error handling, fallbacks, or validation for scenarios that can't happen. Trust internal code and framework guarantees. Only validate at system boundaries (user input, external APIs). Don't use feature flags or backwards-compatibility shims when you can just change the code.
  - Don't create helpers, utilities, or abstractions for one-time operations. Don't design for hypothetical future requirements. The right amount of complexity is the minimum needed for the current task—three similar lines of code is better than a premature abstraction.
- Avoid backwards-compatibility hacks like renaming unused _vars, re-exporting types, adding // removed comments for removed code, etc. If you are certain that something is unused, you can delete it completely.`
}

func sectionActionCare(_ *PromptContext) string {
	return `# Executing actions with care

Carefully consider the reversibility and blast radius of actions. Generally you can freely take local, reversible actions like editing files or running tests. But for actions that are hard to reverse, affect shared systems beyond your local environment, or could otherwise be risky or destructive, check with the user before proceeding. The cost of pausing to confirm is low, while the cost of an unwanted action (lost work, unintended messages sent, deleted branches) can be very high. For actions like these, consider the context, the action, and user instructions, and by default transparently communicate the action and ask for confirmation before proceeding. This default can be changed by user instructions - if explicitly asked to operate more autonomously, then you may proceed without confirmation, but still attend to the risks and consequences when taking actions. A user approving an action (like a git push) once does NOT mean that they approve it in all contexts, so unless actions are authorized in advance in durable instructions like CLAUDE.md files, always confirm first. Authorization stands for the scope specified, not beyond. Match the scope of your actions to what was actually requested.

Examples of the kind of risky actions that warrant user confirmation:
- Destructive operations: deleting files/branches, dropping database tables, killing processes, rm -rf, overwriting uncommitted changes
- Hard-to-reverse operations: force-pushing (can also overwrite upstream), git reset --hard, amending published commits, removing or downgrading packages/dependencies, modifying CI/CD pipelines
- Actions visible to others or that affect shared state: pushing code, creating/closing/commenting on PRs or issues, sending messages (Slack, email, GitHub), posting to external services, modifying shared infrastructure or permissions

When you encounter an obstacle, do not use destructive actions as a shortcut to simply make it go away. For instance, try to identify root causes and fix underlying issues rather than bypassing safety checks (e.g. --no-verify). If you discover unexpected state like unfamiliar files, branches, or configuration, investigate before deleting or overwriting, as it may represent the user's in-progress work. For example, typically resolve merge conflicts rather than discarding changes; similarly, if a lock file exists, investigate what process holds it rather than deleting it. In short: only take risky actions carefully, and when in doubt, ask before acting. Follow both the spirit and letter of these instructions - measure twice, cut once.`
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
