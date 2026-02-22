package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSkill_WithFrontmatter(t *testing.T) {
	content := `---
name: commit
description: Create a git commit
trigger: /commit
---

# Commit Skill

Instructions for creating commits...`

	skill := parseSkill(content, "test.md")

	if skill.Name != "commit" {
		t.Errorf("expected name 'commit', got %q", skill.Name)
	}
	if skill.Description != "Create a git commit" {
		t.Errorf("expected description 'Create a git commit', got %q", skill.Description)
	}
	if skill.Trigger != "/commit" {
		t.Errorf("expected trigger '/commit', got %q", skill.Trigger)
	}
	if skill.Content != "# Commit Skill\n\nInstructions for creating commits..." {
		t.Errorf("unexpected content: %q", skill.Content)
	}
}

func TestParseSkill_NoFrontmatter(t *testing.T) {
	content := "Just some markdown content"
	skill := parseSkill(content, "test.md")

	if skill.Name != "" {
		t.Errorf("expected empty name, got %q", skill.Name)
	}
	if skill.Content != "Just some markdown content" {
		t.Errorf("unexpected content: %q", skill.Content)
	}
}

func TestParseSkill_EmptyFrontmatter(t *testing.T) {
	content := `---
---
Body content here`

	skill := parseSkill(content, "test.md")
	if skill.Content != "Body content here" {
		t.Errorf("unexpected content: %q", skill.Content)
	}
}

func TestParseSkill_PartialFrontmatter(t *testing.T) {
	content := `---
name: myskill
---
Body`

	skill := parseSkill(content, "test.md")
	if skill.Name != "myskill" {
		t.Errorf("expected name 'myskill', got %q", skill.Name)
	}
	if skill.Description != "" {
		t.Errorf("expected empty description, got %q", skill.Description)
	}
}

func TestLoadSkillsFromDir(t *testing.T) {
	// Create a temporary directory with skill files.
	dir := t.TempDir()

	skillContent := `---
name: test-skill
description: A test skill
trigger: /test
---
Test instructions`

	if err := os.WriteFile(filepath.Join(dir, "test.md"), []byte(skillContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Write a non-md file that should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a skill"), 0644); err != nil {
		t.Fatal(err)
	}

	skills := loadSkillsFromDir(dir)
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	if skills[0].Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got %q", skills[0].Name)
	}
	if skills[0].Trigger != "/test" {
		t.Errorf("expected trigger '/test', got %q", skills[0].Trigger)
	}
}

func TestLoadSkillsFromDir_FallbackName(t *testing.T) {
	dir := t.TempDir()

	// A markdown file without a name in frontmatter.
	if err := os.WriteFile(filepath.Join(dir, "review.md"), []byte("Review instructions"), 0644); err != nil {
		t.Fatal(err)
	}

	skills := loadSkillsFromDir(dir)
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	if skills[0].Name != "review" {
		t.Errorf("expected fallback name 'review', got %q", skills[0].Name)
	}
}

func TestLoadSkillsFromDir_NonexistentDir(t *testing.T) {
	skills := loadSkillsFromDir("/nonexistent/path")
	if skills != nil {
		t.Errorf("expected nil skills for nonexistent dir, got %v", skills)
	}
}

func TestActiveSkillContent_Empty(t *testing.T) {
	content := ActiveSkillContent(nil)
	if content != "" {
		t.Errorf("expected empty content, got %q", content)
	}
}

func TestActiveSkillContent_MultipleSkills(t *testing.T) {
	skills := []Skill{
		{Name: "skill1", Description: "First", Trigger: "/s1", Content: "Body 1"},
		{Name: "skill2", Description: "Second", Content: "Body 2"},
	}

	content := ActiveSkillContent(skills)

	if content == "" {
		t.Fatal("expected non-empty content")
	}

	// Should contain both skills.
	if !contains(content, "skill1") {
		t.Error("content should contain 'skill1'")
	}
	if !contains(content, "skill2") {
		t.Error("content should contain 'skill2'")
	}
	if !contains(content, "Body 1") {
		t.Error("content should contain 'Body 1'")
	}
	if !contains(content, "Body 2") {
		t.Error("content should contain 'Body 2'")
	}
}

func TestLoadSkills_ProjectOverridesUser(t *testing.T) {
	// Create a temp cwd with project skills.
	cwd := t.TempDir()
	projDir := filepath.Join(cwd, ".claude", "skills")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}

	projSkill := `---
name: shared-skill
description: Project version
---
Project content`
	if err := os.WriteFile(filepath.Join(projDir, "shared.md"), []byte(projSkill), 0644); err != nil {
		t.Fatal(err)
	}

	skills := LoadSkills(cwd)

	// Should have the project-level version.
	found := false
	for _, s := range skills {
		if s.Name == "shared-skill" {
			found = true
			if s.Description != "Project version" {
				t.Errorf("expected 'Project version', got %q", s.Description)
			}
		}
	}
	if !found {
		t.Error("expected to find 'shared-skill'")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
