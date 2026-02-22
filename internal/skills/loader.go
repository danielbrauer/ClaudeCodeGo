package skills

import (
	"os"
	"path/filepath"
	"strings"
)

// LoadSkills discovers and parses skill files from both user-level
// (~/.claude/skills/) and project-level (.claude/skills/) directories.
// Project-level skills take precedence over user-level skills with the
// same name.
func LoadSkills(cwd string) []Skill {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	var skills []Skill
	seen := make(map[string]bool)

	// Project-level skills first (higher priority).
	projectDir := filepath.Join(cwd, ".claude", "skills")
	projectSkills := loadSkillsFromDir(projectDir)
	for _, s := range projectSkills {
		skills = append(skills, s)
		seen[s.Name] = true
	}

	// User-level skills (lower priority — skip if name already seen).
	userDir := filepath.Join(home, ".claude", "skills")
	userSkills := loadSkillsFromDir(userDir)
	for _, s := range userSkills {
		if !seen[s.Name] {
			skills = append(skills, s)
			seen[s.Name] = true
		}
	}

	return skills
}

// ActiveSkillContent returns the combined content of all loaded skills
// for injection into the system prompt.
func ActiveSkillContent(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var parts []string
	for _, s := range skills {
		header := "## " + s.Name
		if s.Description != "" {
			header += " — " + s.Description
		}
		if s.Trigger != "" {
			header += " (trigger: " + s.Trigger + ")"
		}
		parts = append(parts, header+"\n\n"+s.Content)
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// loadSkillsFromDir reads all .md files from a directory and parses them as skills.
func loadSkillsFromDir(dir string) []Skill {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var skills []Skill
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		skill := parseSkill(string(data), path)
		if skill.Name == "" {
			// Use filename without extension as fallback name.
			skill.Name = strings.TrimSuffix(entry.Name(), ".md")
		}
		skills = append(skills, skill)
	}
	return skills
}

// parseSkill parses a markdown file with optional YAML frontmatter.
// Frontmatter is delimited by "---" lines at the top of the file.
func parseSkill(content, filePath string) Skill {
	s := Skill{FilePath: filePath}

	// Check for frontmatter.
	if !strings.HasPrefix(content, "---") {
		s.Content = strings.TrimSpace(content)
		return s
	}

	// Split on "---" to extract frontmatter.
	// Expected format: ---\nkey: value\n---\nbody
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		s.Content = strings.TrimSpace(content)
		return s
	}

	frontmatter := parts[1]
	body := parts[2]

	// Parse simple YAML frontmatter (key: value lines).
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch key {
		case "name":
			s.Name = value
		case "description":
			s.Description = value
		case "trigger":
			s.Trigger = value
		}
	}

	s.Content = strings.TrimSpace(body)
	return s
}
