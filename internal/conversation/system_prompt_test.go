package conversation

import (
	"strings"
	"testing"

	"github.com/anthropics/claude-code-go/internal/config"
)

// withCleanSections saves and restores the global section registries.
// This allows tests to register custom sections without polluting other tests.
func withCleanSections(t *testing.T) {
	t.Helper()
	sectionsMu.Lock()
	savedCore := coreSections
	savedProject := projectSections
	sectionsMu.Unlock()

	t.Cleanup(func() {
		sectionsMu.Lock()
		coreSections = savedCore
		projectSections = savedProject
		sectionsMu.Unlock()
	})

	resetSections()
}

func TestBuildSystemPrompt_DefaultSections(t *testing.T) {
	// Use the real init-registered sections.
	blocks := BuildSystemPrompt("/tmp/test", nil, "")

	if len(blocks) < 1 {
		t.Fatal("expected at least 1 block")
	}

	// Block 1 should contain identity + environment.
	core := blocks[0].Text
	if !strings.Contains(core, "Claude Code") {
		t.Error("core block missing identity text")
	}
	if !strings.Contains(core, "/tmp/test") {
		t.Error("core block missing working directory")
	}
	if !strings.Contains(core, "Environment:") {
		t.Error("core block missing environment section")
	}
}

func TestBuildSystemPrompt_WithSkills(t *testing.T) {
	blocks := BuildSystemPrompt("/tmp/test", nil, "skill: do-something")

	if len(blocks) < 2 {
		t.Fatal("expected 2 blocks when skills are present")
	}

	project := blocks[1].Text
	if !strings.Contains(project, "# Active Skills") {
		t.Error("project block missing skills heading")
	}
	if !strings.Contains(project, "skill: do-something") {
		t.Error("project block missing skill content")
	}
}

func TestBuildSystemPrompt_WithPermissions(t *testing.T) {
	settings := &config.Settings{
		Permissions: []config.PermissionRule{
			{Tool: "Bash", Pattern: "npm *", Action: "allow"},
		},
	}
	blocks := BuildSystemPrompt("/tmp/test", settings, "")

	if len(blocks) < 2 {
		t.Fatal("expected 2 blocks when permissions are present")
	}

	project := blocks[1].Text
	if !strings.Contains(project, "# Permission Rules") {
		t.Error("project block missing permission rules heading")
	}
	if !strings.Contains(project, "Bash(npm *)") {
		t.Error("project block missing formatted permission rule")
	}
}

func TestBuildSystemPrompt_NoProjectBlock(t *testing.T) {
	// No skills, no permissions, and LoadClaudeMD from a temp dir returns "".
	blocks := BuildSystemPrompt(t.TempDir(), nil, "")

	// Should only have the core block (no project block).
	if len(blocks) != 1 {
		t.Errorf("expected 1 block (core only), got %d", len(blocks))
	}
}

func TestBuildSystemPrompt_AllBlocksHaveTextType(t *testing.T) {
	settings := &config.Settings{
		Permissions: []config.PermissionRule{
			{Tool: "Read", Action: "allow"},
		},
	}
	blocks := BuildSystemPrompt("/tmp/test", settings, "some skill")

	for i, b := range blocks {
		if b.Type != "text" {
			t.Errorf("block %d has type %q, want %q", i, b.Type, "text")
		}
	}
}

func TestRegisterCoreSection(t *testing.T) {
	withCleanSections(t)

	RegisterCoreSection("test-core", func(_ PromptContext) string {
		return "custom core content"
	})

	blocks := BuildSystemPrompt("/tmp/test", nil, "")

	if len(blocks) < 1 {
		t.Fatal("expected at least 1 block")
	}
	if !strings.Contains(blocks[0].Text, "custom core content") {
		t.Error("core block should contain custom registered section")
	}
}

func TestRegisterProjectSection(t *testing.T) {
	withCleanSections(t)

	RegisterProjectSection("test-project", func(_ PromptContext) string {
		return "custom project content"
	})

	blocks := BuildSystemPrompt("/tmp/test", nil, "")

	if len(blocks) < 2 {
		t.Fatal("expected 2 blocks with project section registered")
	}
	if !strings.Contains(blocks[1].Text, "custom project content") {
		t.Error("project block should contain custom registered section")
	}
}

func TestRegisterSection_EmptyContentOmitted(t *testing.T) {
	withCleanSections(t)

	RegisterProjectSection("empty", func(_ PromptContext) string {
		return ""
	})

	blocks := BuildSystemPrompt("/tmp/test", nil, "")

	// Empty section should not create a project block.
	if len(blocks) != 1 {
		t.Errorf("expected 1 block (empty project section omitted), got %d", len(blocks))
	}
}

func TestRegisterSection_OrderPreserved(t *testing.T) {
	withCleanSections(t)

	RegisterProjectSection("first", func(_ PromptContext) string {
		return "FIRST"
	})
	RegisterProjectSection("second", func(_ PromptContext) string {
		return "SECOND"
	})
	RegisterProjectSection("third", func(_ PromptContext) string {
		return "THIRD"
	})

	blocks := BuildSystemPrompt("/tmp/test", nil, "")

	if len(blocks) < 2 {
		t.Fatal("expected 2 blocks")
	}

	text := blocks[1].Text
	firstIdx := strings.Index(text, "FIRST")
	secondIdx := strings.Index(text, "SECOND")
	thirdIdx := strings.Index(text, "THIRD")

	if firstIdx == -1 || secondIdx == -1 || thirdIdx == -1 {
		t.Fatal("expected all three sections in output")
	}
	if firstIdx >= secondIdx || secondIdx >= thirdIdx {
		t.Error("sections should appear in registration order")
	}
}

func TestPromptContext_PassedToSections(t *testing.T) {
	withCleanSections(t)

	var captured PromptContext
	RegisterCoreSection("capture", func(ctx PromptContext) string {
		captured = ctx
		return "ok"
	})

	settings := &config.Settings{
		Permissions: []config.PermissionRule{
			{Tool: "Bash", Action: "allow"},
		},
	}

	BuildSystemPrompt("/my/dir", settings, "my-skills")

	if captured.CWD != "/my/dir" {
		t.Errorf("CWD = %q, want %q", captured.CWD, "/my/dir")
	}
	if captured.SkillContent != "my-skills" {
		t.Errorf("SkillContent = %q, want %q", captured.SkillContent, "my-skills")
	}
	if captured.Settings == nil || len(captured.Settings.Permissions) != 1 {
		t.Error("Settings not passed correctly to section")
	}
}

func TestRenderSections_Separator(t *testing.T) {
	sections := []namedSection{
		{name: "a", fn: func(_ PromptContext) string { return "A" }},
		{name: "b", fn: func(_ PromptContext) string { return "B" }},
	}

	result := renderSections(sections, PromptContext{}, "||")
	if result != "A||B" {
		t.Errorf("got %q, want %q", result, "A||B")
	}
}

func TestRenderSections_SkipsEmpty(t *testing.T) {
	sections := []namedSection{
		{name: "a", fn: func(_ PromptContext) string { return "A" }},
		{name: "empty", fn: func(_ PromptContext) string { return "" }},
		{name: "c", fn: func(_ PromptContext) string { return "C" }},
	}

	result := renderSections(sections, PromptContext{}, "\n")
	if result != "A\nC" {
		t.Errorf("got %q, want %q", result, "A\nC")
	}
}

func TestIdentitySection(t *testing.T) {
	content := identitySection(PromptContext{})
	if !strings.Contains(content, "Claude Code") {
		t.Error("identity section should mention Claude Code")
	}
	if !strings.Contains(content, "tools") {
		t.Error("identity section should mention tools")
	}
}

func TestEnvironmentSection(t *testing.T) {
	content := environmentSection(PromptContext{CWD: "/test/dir"})
	if !strings.Contains(content, "/test/dir") {
		t.Error("environment section should contain CWD")
	}
	if !strings.Contains(content, "Platform:") {
		t.Error("environment section should contain platform info")
	}
	if !strings.Contains(content, "Date:") {
		t.Error("environment section should contain date")
	}
}

func TestClaudeMDSection_Empty(t *testing.T) {
	content := claudeMDSection(PromptContext{CWD: t.TempDir()})
	if content != "" {
		t.Error("claudemd section should be empty when no CLAUDE.md exists")
	}
}

func TestSkillsSection(t *testing.T) {
	content := skillsSection(PromptContext{SkillContent: "my skill"})
	if !strings.Contains(content, "# Active Skills") {
		t.Error("skills section should have heading")
	}
	if !strings.Contains(content, "my skill") {
		t.Error("skills section should contain skill content")
	}
}

func TestSkillsSection_Empty(t *testing.T) {
	content := skillsSection(PromptContext{})
	if content != "" {
		t.Error("skills section should be empty when no skills")
	}
}

func TestPermissionsSection(t *testing.T) {
	ctx := PromptContext{
		Settings: &config.Settings{
			Permissions: []config.PermissionRule{
				{Tool: "Bash", Pattern: "ls", Action: "allow"},
				{Tool: "Read", Action: "deny"},
			},
		},
	}
	content := permissionsSection(ctx)
	if !strings.Contains(content, "# Permission Rules") {
		t.Error("permissions section should have heading")
	}
	if !strings.Contains(content, "allow") {
		t.Error("permissions section should contain rule actions")
	}
}

func TestPermissionsSection_NilSettings(t *testing.T) {
	content := permissionsSection(PromptContext{})
	if content != "" {
		t.Error("permissions section should be empty with nil settings")
	}
}

func TestPermissionsSection_EmptyRules(t *testing.T) {
	ctx := PromptContext{
		Settings: &config.Settings{
			Permissions: []config.PermissionRule{},
		},
	}
	content := permissionsSection(ctx)
	if content != "" {
		t.Error("permissions section should be empty with no rules")
	}
}

func TestFormatPermissionRules(t *testing.T) {
	rules := []config.PermissionRule{
		{Tool: "Bash", Pattern: "npm *", Action: "allow"},
		{Tool: "Read", Action: "ask"},
	}

	result := formatPermissionRules(rules)
	if !strings.Contains(result, "Bash(npm *)") {
		t.Error("should format tool with pattern")
	}
	if !strings.Contains(result, "Read: ask") {
		t.Error("should format tool without pattern")
	}
}

func TestFormatPermissionRules_Empty(t *testing.T) {
	result := formatPermissionRules(nil)
	if result != "" {
		t.Error("should return empty for nil rules")
	}
}
