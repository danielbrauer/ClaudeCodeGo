// Package skills implements skill discovery, parsing, and slash command
// registration for the Claude Code CLI.
//
// Skills are markdown files with YAML frontmatter located in:
//   - ~/.claude/skills/ (user-level, all projects)
//   - .claude/skills/  (project-level)
package skills

// Skill represents a loaded skill definition.
type Skill struct {
	Name        string // skill name from frontmatter
	Description string // short description from frontmatter
	Trigger     string // slash command trigger, e.g. "/commit"
	Content     string // markdown body (instructions/prompt)
	FilePath    string // source file path for debugging
}
