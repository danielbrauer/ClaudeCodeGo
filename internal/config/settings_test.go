package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSettingsEmpty(t *testing.T) {
	// No settings files exist.
	dir := t.TempDir()
	settings, err := LoadSettings(dir)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if settings == nil {
		t.Fatal("expected non-nil settings")
	}
	if settings.Model != "" {
		t.Errorf("Model = %q, want empty", settings.Model)
	}
}

func TestLoadSettingsUserLevel(t *testing.T) {
	// Create a user-level settings file.
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudeDir := filepath.Join(home, ".claude")
	os.MkdirAll(claudeDir, 0755)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{
		"model": "opus",
		"env": {"FOO": "bar"}
	}`), 0644)

	settings, err := LoadSettings(t.TempDir())
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if settings.Model != "opus" {
		t.Errorf("Model = %q, want %q", settings.Model, "opus")
	}
	if settings.Env["FOO"] != "bar" {
		t.Errorf("Env[FOO] = %q, want %q", settings.Env["FOO"], "bar")
	}
}

func TestLoadSettingsProjectOverridesUser(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cwd := t.TempDir()

	// User level.
	userDir := filepath.Join(home, ".claude")
	os.MkdirAll(userDir, 0755)
	os.WriteFile(filepath.Join(userDir, "settings.json"), []byte(`{
		"model": "sonnet",
		"env": {"FOO": "user", "EXTRA": "keep"}
	}`), 0644)

	// Project level (higher priority).
	projDir := filepath.Join(cwd, ".claude")
	os.MkdirAll(projDir, 0755)
	os.WriteFile(filepath.Join(projDir, "settings.json"), []byte(`{
		"model": "opus",
		"env": {"FOO": "project"}
	}`), 0644)

	settings, err := LoadSettings(cwd)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}

	// Project should override user.
	if settings.Model != "opus" {
		t.Errorf("Model = %q, want %q", settings.Model, "opus")
	}
	// Project FOO overrides user FOO.
	if settings.Env["FOO"] != "project" {
		t.Errorf("Env[FOO] = %q, want %q", settings.Env["FOO"], "project")
	}
	// User-only EXTRA should be preserved.
	if settings.Env["EXTRA"] != "keep" {
		t.Errorf("Env[EXTRA] = %q, want %q", settings.Env["EXTRA"], "keep")
	}
}

func TestLoadSettingsLocalOverridesProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cwd := t.TempDir()
	projDir := filepath.Join(cwd, ".claude")
	os.MkdirAll(projDir, 0755)

	// Project level.
	os.WriteFile(filepath.Join(projDir, "settings.json"), []byte(`{
		"model": "sonnet"
	}`), 0644)

	// Local level (higher priority).
	os.WriteFile(filepath.Join(projDir, "settings.local.json"), []byte(`{
		"model": "haiku"
	}`), 0644)

	settings, err := LoadSettings(cwd)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}

	if settings.Model != "haiku" {
		t.Errorf("Model = %q, want %q", settings.Model, "haiku")
	}
}

func TestPermissionRulesMerge(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cwd := t.TempDir()
	projDir := filepath.Join(cwd, ".claude")
	os.MkdirAll(projDir, 0755)

	// User level with a permission rule.
	userDir := filepath.Join(home, ".claude")
	os.MkdirAll(userDir, 0755)
	os.WriteFile(filepath.Join(userDir, "settings.json"), []byte(`{
		"permissions": [
			{"tool": "Bash", "action": "ask"}
		]
	}`), 0644)

	// Project level with a permission rule.
	os.WriteFile(filepath.Join(projDir, "settings.json"), []byte(`{
		"permissions": [
			{"tool": "Bash", "pattern": "npm run *", "action": "allow"}
		]
	}`), 0644)

	settings, err := LoadSettings(cwd)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}

	// Permissions should be concatenated: project first (higher priority), then user.
	if len(settings.Permissions) != 2 {
		t.Fatalf("Permissions len = %d, want 2", len(settings.Permissions))
	}
	// Project rule comes first.
	if settings.Permissions[0].Pattern != "npm run *" {
		t.Errorf("First rule pattern = %q, want %q", settings.Permissions[0].Pattern, "npm run *")
	}
	// User rule comes second.
	if settings.Permissions[1].Action != "ask" {
		t.Errorf("Second rule action = %q, want %q", settings.Permissions[1].Action, "ask")
	}
}

func TestMergeSettings(t *testing.T) {
	base := &Settings{
		Model: "sonnet",
		Env:   map[string]string{"A": "1", "B": "2"},
		Permissions: []PermissionRule{
			{Tool: "Bash", Action: "ask"},
		},
	}
	overlay := &Settings{
		Model: "opus",
		Env:   map[string]string{"B": "override", "C": "3"},
		Permissions: []PermissionRule{
			{Tool: "Bash", Pattern: "npm *", Action: "allow"},
		},
	}

	result := mergeSettings(base, overlay)

	if result.Model != "opus" {
		t.Errorf("Model = %q, want %q", result.Model, "opus")
	}
	if result.Env["A"] != "1" {
		t.Errorf("Env[A] = %q, want %q", result.Env["A"], "1")
	}
	if result.Env["B"] != "override" {
		t.Errorf("Env[B] = %q, want %q", result.Env["B"], "override")
	}
	if result.Env["C"] != "3" {
		t.Errorf("Env[C] = %q, want %q", result.Env["C"], "3")
	}
	// Permissions: overlay first, then base.
	if len(result.Permissions) != 2 {
		t.Fatalf("Permissions len = %d, want 2", len(result.Permissions))
	}
	if result.Permissions[0].Pattern != "npm *" {
		t.Errorf("Perm[0].Pattern = %q, want %q", result.Permissions[0].Pattern, "npm *")
	}
}
