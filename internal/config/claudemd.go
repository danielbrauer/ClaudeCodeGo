package config

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadClaudeMD loads and merges CLAUDE.md content from multiple locations.
// It supports:
//   - User-level: ~/.claude/CLAUDE.md
//   - Directory walk from filesystem root to CWD
//   - Project-level: .claude/CLAUDE.md
//   - @path imports for including content from other files
//   - .claude/rules/ directory for additional rule files
func LoadClaudeMD(cwd string) string {
	var sections []string

	// 1. User-level: ~/.claude/CLAUDE.md
	if home, err := os.UserHomeDir(); err == nil {
		if content := loadClaudeMDFile(filepath.Join(home, ".claude", "CLAUDE.md"), nil); content != "" {
			sections = append(sections, content)
		}
		// User-level rules: ~/.claude/rules/
		if rules := loadRulesDir(filepath.Join(home, ".claude", "rules")); rules != "" {
			sections = append(sections, rules)
		}
	}

	// 2. Walk from filesystem root to CWD, loading CLAUDE.md at each level.
	parts := strings.Split(filepath.Clean(cwd), string(filepath.Separator))
	for i := 1; i <= len(parts); i++ {
		dir := string(filepath.Separator) + filepath.Join(parts[1:i]...)
		if content := loadClaudeMDFile(filepath.Join(dir, "CLAUDE.md"), nil); content != "" {
			sections = append(sections, content)
		}
	}

	// 3. Project-level: .claude/CLAUDE.md
	if content := loadClaudeMDFile(filepath.Join(cwd, ".claude", "CLAUDE.md"), nil); content != "" {
		sections = append(sections, content)
	}

	// 4. Project-level rules: .claude/rules/
	if rules := loadRulesDir(filepath.Join(cwd, ".claude", "rules")); rules != "" {
		sections = append(sections, rules)
	}

	return strings.Join(sections, "\n\n---\n\n")
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
