// Package conversation manages the agentic conversation loop.
package conversation

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/anthropics/claude-code-go/internal/api"
)

// BuildSystemPrompt assembles the system prompt blocks from CLAUDE.md files
// and environment context.
func BuildSystemPrompt(cwd string) []api.SystemBlock {
	var parts []string

	// Core identity.
	parts = append(parts, "You are Claude Code, an interactive CLI tool that helps users with software engineering tasks.")
	parts = append(parts, "You have access to tools that let you read files, write files, execute commands, and more.")

	// Environment info.
	parts = append(parts, fmt.Sprintf(
		"\nEnvironment:\n- Working directory: %s\n- Platform: %s/%s\n- Date: %s",
		cwd, runtime.GOOS, runtime.GOARCH, "today",
	))

	// Load CLAUDE.md content from various locations.
	claudeMDContent := loadClaudeMD(cwd)
	if claudeMDContent != "" {
		parts = append(parts, "\n# Project Instructions (CLAUDE.md)\n\n"+claudeMDContent)
	}

	return []api.SystemBlock{
		{
			Type: "text",
			Text: strings.Join(parts, "\n"),
		},
	}
}

// loadClaudeMD loads CLAUDE.md from multiple locations and merges them.
func loadClaudeMD(cwd string) string {
	var sections []string

	// User-level: ~/.claude/CLAUDE.md
	if home, err := os.UserHomeDir(); err == nil {
		if content := readClaudeMDFile(filepath.Join(home, ".claude", "CLAUDE.md")); content != "" {
			sections = append(sections, content)
		}
	}

	// Walk from filesystem root to CWD.
	parts := strings.Split(filepath.Clean(cwd), string(filepath.Separator))
	for i := 1; i <= len(parts); i++ {
		dir := string(filepath.Separator) + filepath.Join(parts[1:i]...)
		if content := readClaudeMDFile(filepath.Join(dir, "CLAUDE.md")); content != "" {
			sections = append(sections, content)
		}
	}

	// Project-level: .claude/CLAUDE.md
	if content := readClaudeMDFile(filepath.Join(cwd, ".claude", "CLAUDE.md")); content != "" {
		sections = append(sections, content)
	}

	return strings.Join(sections, "\n\n---\n\n")
}

func readClaudeMDFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
