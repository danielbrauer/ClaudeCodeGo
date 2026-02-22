package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadClaudeMDBasic(t *testing.T) {
	dir := t.TempDir()

	// Create a CLAUDE.md in the CWD.
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Test Rules\nDo this."), 0644)

	result := LoadClaudeMD(dir)
	if !strings.Contains(result, "# Test Rules") {
		t.Errorf("expected CLAUDE.md content in result, got: %s", result)
	}
}

func TestLoadClaudeMDProjectLevel(t *testing.T) {
	dir := t.TempDir()

	// Create .claude/CLAUDE.md
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0755)
	os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte("Project rules"), 0644)

	result := LoadClaudeMD(dir)
	if !strings.Contains(result, "Project rules") {
		t.Errorf("expected .claude/CLAUDE.md content, got: %s", result)
	}
}

func TestLoadClaudeMDAtPathImport(t *testing.T) {
	dir := t.TempDir()

	// Create an imported file.
	os.WriteFile(filepath.Join(dir, "extra-rules.md"), []byte("Extra rule content"), 0644)

	// Create CLAUDE.md with @path import.
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("Main rules\n@extra-rules.md\nMore rules"), 0644)

	result := LoadClaudeMD(dir)
	if !strings.Contains(result, "Main rules") {
		t.Errorf("expected main content, got: %s", result)
	}
	if !strings.Contains(result, "Extra rule content") {
		t.Errorf("expected imported content, got: %s", result)
	}
	if !strings.Contains(result, "More rules") {
		t.Errorf("expected content after import, got: %s", result)
	}
}

func TestLoadClaudeMDAtPathCycleDetection(t *testing.T) {
	dir := t.TempDir()

	// Create two files that import each other.
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("File A\n@b.md"), 0644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("File B\n@a.md"), 0644)

	// Create CLAUDE.md that imports a.md
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("@a.md"), 0644)

	// Should not panic or infinite loop.
	result := LoadClaudeMD(dir)
	if !strings.Contains(result, "File A") {
		t.Errorf("expected File A content, got: %s", result)
	}
	if !strings.Contains(result, "File B") {
		t.Errorf("expected File B content, got: %s", result)
	}
}

func TestLoadClaudeMDRulesDir(t *testing.T) {
	dir := t.TempDir()

	// Create .claude/rules/ directory with files.
	rulesDir := filepath.Join(dir, ".claude", "rules")
	os.MkdirAll(rulesDir, 0755)
	os.WriteFile(filepath.Join(rulesDir, "01-style.md"), []byte("Use Go conventions"), 0644)
	os.WriteFile(filepath.Join(rulesDir, "02-testing.md"), []byte("Write table-driven tests"), 0644)
	os.WriteFile(filepath.Join(rulesDir, "not-md.txt"), []byte("Should be ignored"), 0644)

	result := LoadClaudeMD(dir)
	if !strings.Contains(result, "Use Go conventions") {
		t.Errorf("expected rules/01-style.md content, got: %s", result)
	}
	if !strings.Contains(result, "Write table-driven tests") {
		t.Errorf("expected rules/02-testing.md content, got: %s", result)
	}
	if strings.Contains(result, "Should be ignored") {
		t.Errorf("non-.md files should be ignored")
	}
}

func TestLoadClaudeMDRulesDirAlphabetical(t *testing.T) {
	dir := t.TempDir()

	rulesDir := filepath.Join(dir, ".claude", "rules")
	os.MkdirAll(rulesDir, 0755)
	os.WriteFile(filepath.Join(rulesDir, "b-rule.md"), []byte("BRULE"), 0644)
	os.WriteFile(filepath.Join(rulesDir, "a-rule.md"), []byte("ARULE"), 0644)

	result := LoadClaudeMD(dir)
	aIdx := strings.Index(result, "ARULE")
	bIdx := strings.Index(result, "BRULE")
	if aIdx == -1 || bIdx == -1 {
		t.Fatalf("missing rule content: %s", result)
	}
	if aIdx > bIdx {
		t.Errorf("rules should be sorted alphabetically (a before b)")
	}
}

func TestLoadClaudeMDAtPathDirectory(t *testing.T) {
	dir := t.TempDir()

	// Create a directory to import.
	importDir := filepath.Join(dir, "extra-rules")
	os.MkdirAll(importDir, 0755)
	os.WriteFile(filepath.Join(importDir, "rule1.md"), []byte("Extra rule 1"), 0644)
	os.WriteFile(filepath.Join(importDir, "rule2.md"), []byte("Extra rule 2"), 0644)

	// Create CLAUDE.md with @path pointing to directory.
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("Main\n@extra-rules"), 0644)

	result := LoadClaudeMD(dir)
	if !strings.Contains(result, "Extra rule 1") {
		t.Errorf("expected imported dir content, got: %s", result)
	}
	if !strings.Contains(result, "Extra rule 2") {
		t.Errorf("expected imported dir content, got: %s", result)
	}
}

func TestLoadClaudeMDEmpty(t *testing.T) {
	dir := t.TempDir()
	result := LoadClaudeMD(dir)
	if result != "" {
		t.Errorf("expected empty result for dir without CLAUDE.md, got: %s", result)
	}
}

func TestLoadClaudeMDAtPathNonExistent(t *testing.T) {
	dir := t.TempDir()

	// @path to a nonexistent file should be kept as-is.
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("Before\n@nonexistent.md\nAfter"), 0644)

	result := LoadClaudeMD(dir)
	if !strings.Contains(result, "@nonexistent.md") {
		t.Errorf("expected @nonexistent.md preserved, got: %s", result)
	}
}
