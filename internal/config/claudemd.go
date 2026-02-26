package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ClaudeMDEntry represents a loaded CLAUDE.md file with its metadata.
type ClaudeMDEntry struct {
	Path    string // absolute path to the file
	Type    string // "User", "Project", "Local", "Managed"
	Content string // file content
}

// LoadClaudeMD loads and merges CLAUDE.md content from multiple locations.
// Returns a plain concatenation of all content (legacy behavior).
func LoadClaudeMD(cwd string) string {
	entries := LoadClaudeMDEntries(cwd)
	if len(entries) == 0 {
		return ""
	}
	var sections []string
	for _, e := range entries {
		sections = append(sections, e.Content)
	}
	return strings.Join(sections, "\n\n---\n\n")
}

// LoadClaudeMDEntries loads CLAUDE.md files with path and type annotations.
// This is used for the context injection format matching the JS CLI.
func LoadClaudeMDEntries(cwd string) []ClaudeMDEntry {
	var entries []ClaudeMDEntry

	// 1. User-level: ~/.claude/CLAUDE.md
	if home, err := os.UserHomeDir(); err == nil {
		path := filepath.Join(home, ".claude", "CLAUDE.md")
		if content := loadClaudeMDFile(path, nil); content != "" {
			entries = append(entries, ClaudeMDEntry{Path: path, Type: "User", Content: content})
		}
		// User-level rules: ~/.claude/rules/
		rulesDir := filepath.Join(home, ".claude", "rules")
		if rules := loadRulesDir(rulesDir); rules != "" {
			entries = append(entries, ClaudeMDEntry{Path: rulesDir, Type: "User", Content: rules})
		}
	}

	// 2. Walk from filesystem root to CWD, loading CLAUDE.md at each level.
	parts := strings.Split(filepath.Clean(cwd), string(filepath.Separator))
	for i := 1; i <= len(parts); i++ {
		dir := string(filepath.Separator) + filepath.Join(parts[1:i]...)
		path := filepath.Join(dir, "CLAUDE.md")
		if content := loadClaudeMDFile(path, nil); content != "" {
			entries = append(entries, ClaudeMDEntry{Path: path, Type: "Project", Content: content})
		}
	}

	// 3. Project-level: .claude/CLAUDE.md
	path := filepath.Join(cwd, ".claude", "CLAUDE.md")
	if content := loadClaudeMDFile(path, nil); content != "" {
		entries = append(entries, ClaudeMDEntry{Path: path, Type: "Project", Content: content})
	}

	// 4. Project-level rules: .claude/rules/
	rulesDir := filepath.Join(cwd, ".claude", "rules")
	if rules := loadRulesDir(rulesDir); rules != "" {
		entries = append(entries, ClaudeMDEntry{Path: rulesDir, Type: "Project", Content: rules})
	}

	return entries
}

// FormatClaudeMDForContext formats CLAUDE.md entries for injection into
// the <system-reminder> context block, matching the JS CLI's ls7() format.
func FormatClaudeMDForContext(entries []ClaudeMDEntry) string {
	if len(entries) == 0 {
		return ""
	}

	var parts []string
	for _, entry := range entries {
		if entry.Content == "" {
			continue
		}
		var annotation string
		switch entry.Type {
		case "Project":
			annotation = " (project instructions, checked into the codebase)"
		case "Local":
			annotation = " (user's private project instructions, not checked in)"
		case "User":
			annotation = " (user's private global instructions for all projects)"
		default:
			annotation = ""
		}
		parts = append(parts, fmt.Sprintf("Contents of %s%s:\n\n%s", entry.Path, annotation, entry.Content))
	}

	if len(parts) == 0 {
		return ""
	}

	const preamble = "Codebase and user instructions are shown below. Be sure to adhere to these instructions. IMPORTANT: These instructions OVERRIDE any default behavior and you MUST follow them exactly as written."
	return preamble + "\n\n" + strings.Join(parts, "\n\n")
}

// loadClaudeMDFile reads a CLAUDE.md file and resolves @path imports.
// The visited set prevents import cycles.
func loadClaudeMDFile(path string, visited map[string]bool) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ""
	}

	// Cycle detection.
	if visited == nil {
		visited = make(map[string]bool)
	}
	if visited[absPath] {
		return ""
	}
	visited[absPath] = true

	data, err := os.ReadFile(absPath)
	if err != nil {
		return ""
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		return ""
	}

	// Resolve @path imports. Each @path directive must be on its own line.
	dir := filepath.Dir(absPath)
	return resolveImports(content, dir, visited)
}

// resolveImports processes @path directives in CLAUDE.md content.
// Paths are resolved relative to the directory containing the file.
// Max depth is limited by cycle detection.
func resolveImports(content string, baseDir string, visited map[string]bool) string {
	lines := strings.Split(content, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for @path directive (line starts with @ followed by a path).
		if strings.HasPrefix(trimmed, "@") && len(trimmed) > 1 {
			importPath := trimmed[1:] // strip the @

			// Resolve relative to the file's directory.
			if !filepath.IsAbs(importPath) {
				importPath = filepath.Join(baseDir, importPath)
			}

			// Check if it's a file or directory.
			info, err := os.Stat(importPath)
			if err != nil {
				// Keep the line as-is if the path doesn't exist.
				result = append(result, line)
				continue
			}

			if info.IsDir() {
				// Import all .md files from the directory.
				dirContent := loadRulesDir(importPath)
				if dirContent != "" {
					result = append(result, dirContent)
				}
			} else {
				// Import the file.
				imported := loadClaudeMDFile(importPath, visited)
				if imported != "" {
					result = append(result, imported)
				}
			}
			continue
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// loadRulesDir loads all .md files from a rules directory, sorted alphabetically.
// It does not recurse into subdirectories.
func loadRulesDir(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	// Collect .md files, sorted alphabetically.
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	var sections []string
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content != "" {
			sections = append(sections, content)
		}
	}

	return strings.Join(sections, "\n\n")
}
