package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestLoadSettingsJSPermissions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cwd := t.TempDir()
	projDir := filepath.Join(cwd, ".claude")
	os.MkdirAll(projDir, 0755)

	// JS format permissions.
	os.WriteFile(filepath.Join(projDir, "settings.json"), []byte(`{
		"permissions": {
			"allow": ["Bash(npm:*)", "Read(src/**)"],
			"deny": ["Bash(rm *)"],
			"ask": ["WebFetch(domain:unknown.com)"]
		}
	}`), 0644)

	settings, err := LoadSettings(cwd)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}

	if len(settings.Permissions) != 4 {
		t.Fatalf("Permissions len = %d, want 4", len(settings.Permissions))
	}

	// Check that rules are parsed correctly.
	var allowCount, denyCount, askCount int
	for _, rule := range settings.Permissions {
		switch rule.Action {
		case "allow":
			allowCount++
		case "deny":
			denyCount++
		case "ask":
			askCount++
		}
	}
	if allowCount != 2 {
		t.Errorf("allow count = %d, want 2", allowCount)
	}
	if denyCount != 1 {
		t.Errorf("deny count = %d, want 1", denyCount)
	}
	if askCount != 1 {
		t.Errorf("ask count = %d, want 1", askCount)
	}
}

func TestLoadSettingsJSAndGoPermissionsMerge(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cwd := t.TempDir()
	projDir := filepath.Join(cwd, ".claude")
	os.MkdirAll(projDir, 0755)

	// User-level with Go format.
	userDir := filepath.Join(home, ".claude")
	os.MkdirAll(userDir, 0755)
	os.WriteFile(filepath.Join(userDir, "settings.json"), []byte(`{
		"permissions": [
			{"tool": "Bash", "action": "ask"}
		]
	}`), 0644)

	// Project-level with JS format.
	os.WriteFile(filepath.Join(projDir, "settings.json"), []byte(`{
		"permissions": {
			"allow": ["Bash(npm:*)"]
		}
	}`), 0644)

	settings, err := LoadSettings(cwd)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}

	// Should have both rules: project first, then user.
	if len(settings.Permissions) != 2 {
		t.Fatalf("Permissions len = %d, want 2", len(settings.Permissions))
	}
	// Project rule (JS format) first.
	if settings.Permissions[0].Tool != "Bash" || settings.Permissions[0].Pattern != "npm:*" {
		t.Errorf("First rule: %+v", settings.Permissions[0])
	}
	// User rule (Go format) second.
	if settings.Permissions[1].Tool != "Bash" || settings.Permissions[1].Action != "ask" {
		t.Errorf("Second rule: %+v", settings.Permissions[1])
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

func TestMergeSettingsFastMode(t *testing.T) {
	boolPtr := func(v bool) *bool { return &v }

	tests := []struct {
		name     string
		base     *bool
		overlay  *bool
		wantNil  bool
		wantVal  bool
	}{
		{"both nil", nil, nil, true, false},
		{"base set, overlay nil", boolPtr(true), nil, false, true},
		{"base nil, overlay set", nil, boolPtr(true), false, true},
		{"overlay overrides base true→false", boolPtr(true), boolPtr(false), false, false},
		{"overlay overrides base false→true", boolPtr(false), boolPtr(true), false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := &Settings{FastMode: tt.base}
			overlay := &Settings{FastMode: tt.overlay}
			result := mergeSettings(base, overlay)

			if tt.wantNil {
				if result.FastMode != nil {
					t.Errorf("FastMode = %v, want nil", *result.FastMode)
				}
			} else {
				if result.FastMode == nil {
					t.Fatalf("FastMode is nil, want %v", tt.wantVal)
				}
				if *result.FastMode != tt.wantVal {
					t.Errorf("FastMode = %v, want %v", *result.FastMode, tt.wantVal)
				}
			}
		})
	}
}

func TestBoolVal(t *testing.T) {
	tests := []struct {
		name string
		p    *bool
		def  bool
		want bool
	}{
		{"nil_default_true", nil, true, true},
		{"nil_default_false", nil, false, false},
		{"true_ptr", BoolPtr(true), false, true},
		{"false_ptr", BoolPtr(false), true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BoolVal(tt.p, tt.def)
			if got != tt.want {
				t.Errorf("BoolVal = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBoolPtr(t *testing.T) {
	p := BoolPtr(true)
	if p == nil || *p != true {
		t.Errorf("BoolPtr(true) = %v, want &true", p)
	}
	p = BoolPtr(false)
	if p == nil || *p != false {
		t.Errorf("BoolPtr(false) = %v, want &false", p)
	}
}

func TestMergeSettingsUserPreferences(t *testing.T) {
	base := &Settings{
		AutoCompactEnabled: BoolPtr(true),
		EditorMode:         "normal",
		Theme:              "dark",
	}
	overlay := &Settings{
		AutoCompactEnabled: BoolPtr(false),
		FastMode:           BoolPtr(true),
		EditorMode:         "vim",
	}

	result := mergeSettings(base, overlay)

	// AutoCompact: overlay wins.
	if result.AutoCompactEnabled == nil || *result.AutoCompactEnabled != false {
		t.Errorf("AutoCompactEnabled = %v, want false", result.AutoCompactEnabled)
	}
	// FastMode: overlay has it, base doesn't.
	if result.FastMode == nil || *result.FastMode != true {
		t.Errorf("FastMode = %v, want true", result.FastMode)
	}
	// EditorMode: overlay wins.
	if result.EditorMode != "vim" {
		t.Errorf("EditorMode = %q, want %q", result.EditorMode, "vim")
	}
	// Theme: base preserved since overlay is empty.
	if result.Theme != "dark" {
		t.Errorf("Theme = %q, want %q", result.Theme, "dark")
	}
}

func TestMergeSettingsUserPreferencesBaseOnly(t *testing.T) {
	base := &Settings{
		Verbose:          BoolPtr(true),
		ThinkingEnabled:  BoolPtr(false),
		RespectGitignore: BoolPtr(false),
		DiffTool:         "terminal",
		NotifChannel:     "iterm2",
	}
	overlay := &Settings{} // empty overlay

	result := mergeSettings(base, overlay)

	if result.Verbose == nil || *result.Verbose != true {
		t.Errorf("Verbose = %v, want true", result.Verbose)
	}
	if result.ThinkingEnabled == nil || *result.ThinkingEnabled != false {
		t.Errorf("ThinkingEnabled = %v, want false", result.ThinkingEnabled)
	}
	if result.RespectGitignore == nil || *result.RespectGitignore != false {
		t.Errorf("RespectGitignore = %v, want false", result.RespectGitignore)
	}
	if result.DiffTool != "terminal" {
		t.Errorf("DiffTool = %q, want %q", result.DiffTool, "terminal")
	}
	if result.NotifChannel != "iterm2" {
		t.Errorf("NotifChannel = %q, want %q", result.NotifChannel, "iterm2")
	}
}

func TestFastModeSerialization(t *testing.T) {
	boolPtr := func(v bool) *bool { return &v }

	// Test that FastMode round-trips through JSON.
	s := &Settings{
		Model:    "opus",
		FastMode: boolPtr(true),
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var s2 Settings
	if err := json.Unmarshal(data, &s2); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if s2.FastMode == nil || !*s2.FastMode {
		t.Errorf("round-tripped FastMode = %v, want true", s2.FastMode)
	}

	// nil FastMode should be omitted from JSON.
	s3 := &Settings{Model: "sonnet"}
	data3, _ := json.Marshal(s3)
	if strings.Contains(string(data3), "fastMode") {
		t.Errorf("nil FastMode should be omitted from JSON, got: %s", data3)
	}
}

func TestSaveUserSetting_NewFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	err := SaveUserSetting("fastMode", true)
	if err != nil {
		t.Fatalf("SaveUserSetting: %v", err)
	}

	// Read back and verify.
	path := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if val, ok := settings["fastMode"]; !ok {
		t.Error("fastMode key not found in saved settings")
	} else if val != true {
		t.Errorf("fastMode = %v, want true", val)
	}
}

func TestSaveUserSetting_ExistingFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudeDir := filepath.Join(home, ".claude")
	os.MkdirAll(claudeDir, 0755)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{
  "model": "opus",
  "verbose": false
}`), 0644)

	err := SaveUserSetting("verbose", true)
	if err != nil {
		t.Fatalf("SaveUserSetting: %v", err)
	}

	// Read back.
	data, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var settings map[string]interface{}
	json.Unmarshal(data, &settings)

	// verbose should be updated.
	if val := settings["verbose"]; val != true {
		t.Errorf("verbose = %v, want true", val)
	}
	// model should be preserved.
	if val := settings["model"]; val != "opus" {
		t.Errorf("model = %v, want opus", val)
	}
}

func TestSaveUserSetting_CorruptFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudeDir := filepath.Join(home, ".claude")
	os.MkdirAll(claudeDir, 0755)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{corrupt json`), 0644)

	// Should not error; starts fresh.
	err := SaveUserSetting("theme", "light")
	if err != nil {
		t.Fatalf("SaveUserSetting on corrupt file: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	var settings map[string]interface{}
	json.Unmarshal(data, &settings)

	if val := settings["theme"]; val != "light" {
		t.Errorf("theme = %v, want light", val)
	}
}

func TestLoadSettingsUserPreferences(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudeDir := filepath.Join(home, ".claude")
	os.MkdirAll(claudeDir, 0755)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{
		"autoCompactEnabled": false,
		"editorMode": "vim",
		"theme": "light",
		"fastMode": true
	}`), 0644)

	settings, err := LoadSettings(t.TempDir())
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}

	if settings.AutoCompactEnabled == nil || *settings.AutoCompactEnabled != false {
		t.Errorf("AutoCompactEnabled = %v, want false", settings.AutoCompactEnabled)
	}
	if settings.EditorMode != "vim" {
		t.Errorf("EditorMode = %q, want %q", settings.EditorMode, "vim")
	}
	if settings.Theme != "light" {
		t.Errorf("Theme = %q, want %q", settings.Theme, "light")
	}
	if settings.FastMode == nil || *settings.FastMode != true {
		t.Errorf("FastMode = %v, want true", settings.FastMode)
	}
}

func TestLoadSettingsFastModeProjectOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cwd := t.TempDir()

	// User level: fastMode true.
	userDir := filepath.Join(home, ".claude")
	os.MkdirAll(userDir, 0755)
	os.WriteFile(filepath.Join(userDir, "settings.json"), []byte(`{
		"fastMode": true
	}`), 0644)

	// Project level: fastMode false.
	projDir := filepath.Join(cwd, ".claude")
	os.MkdirAll(projDir, 0755)
	os.WriteFile(filepath.Join(projDir, "settings.json"), []byte(`{
		"fastMode": false
	}`), 0644)

	settings, err := LoadSettings(cwd)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if settings.FastMode == nil {
		t.Fatal("FastMode is nil, want false")
	}
	if *settings.FastMode {
		t.Errorf("FastMode = true, want false (project override)")
	}
}

func TestUserSettingsPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := UserSettingsPath()
	if err != nil {
		t.Fatalf("UserSettingsPath: %v", err)
	}
	expected := filepath.Join(home, ".claude", "settings.json")
	if path != expected {
		t.Errorf("UserSettingsPath = %q, want %q", path, expected)
	}
}
