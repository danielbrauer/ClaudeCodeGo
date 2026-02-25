package conversation

import (
	"strings"
	"testing"

	"github.com/anthropics/claude-code-go/internal/config"
)

func TestIdentitySection(t *testing.T) {
	ctx := &PromptContext{Cwd: "/tmp"}
	text := identitySection(ctx)
	if text == "" {
		t.Fatal("identity section should not be empty")
	}
	if !strings.Contains(text, "Claude Code") {
		t.Error("identity section should mention Claude Code")
	}
}

func TestEnvironmentSection(t *testing.T) {
	ctx := &PromptContext{Cwd: "/home/user/project"}
	text := environmentSection(ctx)
	if !strings.Contains(text, "/home/user/project") {
		t.Error("environment section should include cwd")
	}
	if !strings.Contains(text, "Platform:") {
		t.Error("environment section should include platform info")
	}
}

func TestClaudeMDSection_Empty(t *testing.T) {
	ctx := &PromptContext{Cwd: "/nonexistent/path/that/has/no/claudemd"}
	text := claudeMDSection(ctx)
	if text != "" {
		t.Errorf("claudemd section should be empty for missing CLAUDE.md, got %q", text)
	}
}

func TestSkillsSection(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		ctx := &PromptContext{SkillContent: ""}
		if text := skillsSection(ctx); text != "" {
			t.Errorf("expected empty, got %q", text)
		}
	})
	t.Run("with content", func(t *testing.T) {
		ctx := &PromptContext{SkillContent: "Use gofmt for formatting."}
		text := skillsSection(ctx)
		if !strings.Contains(text, "Active Skills") {
			t.Error("should have Active Skills header")
		}
		if !strings.Contains(text, "gofmt") {
			t.Error("should include skill content")
		}
	})
}

func TestPermissionsSection(t *testing.T) {
	t.Run("nil settings", func(t *testing.T) {
		ctx := &PromptContext{Settings: nil}
		if text := permissionsSection(ctx); text != "" {
			t.Errorf("expected empty, got %q", text)
		}
	})
	t.Run("empty permissions", func(t *testing.T) {
		ctx := &PromptContext{Settings: &config.Settings{}}
		if text := permissionsSection(ctx); text != "" {
			t.Errorf("expected empty, got %q", text)
		}
	})
	t.Run("with rules", func(t *testing.T) {
		ctx := &PromptContext{
			Settings: &config.Settings{
				Permissions: []config.PermissionRule{
					{Tool: "Bash", Pattern: "npm run *", Action: "allow"},
				},
			},
		}
		text := permissionsSection(ctx)
		if !strings.Contains(text, "Permission Rules") {
			t.Error("should have Permission Rules header")
		}
		if !strings.Contains(text, "allow") {
			t.Error("should include rule action")
		}
	})
}

func TestRegisterAndRenderSections(t *testing.T) {
	// Save and restore global state.
	resetSections()
	defer func() {
		resetSections()
		// Re-register defaults so other tests aren't affected.
		RegisterCoreSection("identity", identitySection)
		RegisterCoreSection("environment", environmentSection)
		RegisterProjectSection("claudemd", claudeMDSection)
		RegisterProjectSection("skills", skillsSection)
		RegisterProjectSection("permissions", permissionsSection)
	}()

	called := false
	RegisterCoreSection("test-core", func(_ *PromptContext) string {
		called = true
		return "core-output"
	})
	RegisterProjectSection("test-proj", func(_ *PromptContext) string {
		return "proj-output"
	})

	ctx := &PromptContext{Cwd: "/tmp"}

	coreParts := renderSections(coreSections, ctx)
	if !called {
		t.Error("core section should have been called")
	}
	if len(coreParts) != 1 || coreParts[0] != "core-output" {
		t.Errorf("unexpected core parts: %v", coreParts)
	}

	projParts := renderSections(projectSections, ctx)
	if len(projParts) != 1 || projParts[0] != "proj-output" {
		t.Errorf("unexpected project parts: %v", projParts)
	}
}

func TestRenderSections_SkipsEmpty(t *testing.T) {
	entries := []sectionEntry{
		{Name: "a", Section: func(_ *PromptContext) string { return "hello" }},
		{Name: "b", Section: func(_ *PromptContext) string { return "" }},
		{Name: "c", Section: func(_ *PromptContext) string { return "world" }},
	}
	ctx := &PromptContext{}
	parts := renderSections(entries, ctx)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[0] != "hello" || parts[1] != "world" {
		t.Errorf("unexpected parts: %v", parts)
	}
}

func TestBuildSystemPrompt_DefaultSections(t *testing.T) {
	// Use default registered sections (from init).
	blocks := BuildSystemPrompt("/tmp/test", nil, "")
	if len(blocks) == 0 {
		t.Fatal("expected at least one block")
	}
	// Block 1 should contain identity and environment.
	block1 := blocks[0].Text
	if !strings.Contains(block1, "Claude Code") {
		t.Error("block 1 should contain identity")
	}
	if !strings.Contains(block1, "/tmp/test") {
		t.Error("block 1 should contain environment with cwd")
	}
}

func TestBuildSystemPrompt_WithSkillsAndPermissions(t *testing.T) {
	settings := &config.Settings{
		Permissions: []config.PermissionRule{
			{Tool: "Bash", Action: "allow"},
		},
	}
	blocks := BuildSystemPrompt("/tmp", settings, "my skill content")
	if len(blocks) < 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	block2 := blocks[1].Text
	if !strings.Contains(block2, "my skill content") {
		t.Error("block 2 should contain skill content")
	}
	if !strings.Contains(block2, "Permission Rules") {
		t.Error("block 2 should contain permission rules")
	}
}

func TestBuildSystemPrompt_NoProjectBlock_WhenEmpty(t *testing.T) {
	// With no CLAUDE.md, no skills, no permissions: only block 1.
	blocks := BuildSystemPrompt("/nonexistent/xyz/path", nil, "")
	if len(blocks) != 1 {
		t.Errorf("expected 1 block (core only), got %d", len(blocks))
	}
}

func TestBuildSystemPrompt_CustomSection(t *testing.T) {
	// Save and restore global state.
	resetSections()
	defer func() {
		resetSections()
		RegisterCoreSection("identity", identitySection)
		RegisterCoreSection("environment", environmentSection)
		RegisterProjectSection("claudemd", claudeMDSection)
		RegisterProjectSection("skills", skillsSection)
		RegisterProjectSection("permissions", permissionsSection)
	}()

	RegisterCoreSection("custom", func(_ *PromptContext) string {
		return "custom core content"
	})
	RegisterProjectSection("custom-proj", func(ctx *PromptContext) string {
		if ctx.IsAgent {
			return "agent mode"
		}
		return "cli mode"
	})

	// Test CLI mode.
	blocks := BuildSystemPrompt("/tmp", nil, "")
	if len(blocks) < 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if !strings.Contains(blocks[0].Text, "custom core content") {
		t.Error("should contain custom core content")
	}
	if !strings.Contains(blocks[1].Text, "cli mode") {
		t.Error("should contain cli mode content")
	}
}

func TestPromptContext_IsAgent(t *testing.T) {
	section := func(ctx *PromptContext) string {
		if ctx.IsAgent {
			return "agent prompt"
		}
		return "cli prompt"
	}

	cli := section(&PromptContext{IsAgent: false})
	agent := section(&PromptContext{IsAgent: true})
	if cli != "cli prompt" {
		t.Errorf("expected cli prompt, got %q", cli)
	}
	if agent != "agent prompt" {
		t.Errorf("expected agent prompt, got %q", agent)
	}
}

func TestFormatPermissionRules(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := formatPermissionRules(nil); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})
	t.Run("single rule", func(t *testing.T) {
		rules := []config.PermissionRule{
			{Tool: "Read", Pattern: ".env", Action: "deny"},
		}
		got := formatPermissionRules(rules)
		if !strings.Contains(got, "deny") {
			t.Error("should contain action")
		}
		if !strings.Contains(got, "configured") {
			t.Error("should contain header text")
		}
	})
}

func TestSectionOrdering(t *testing.T) {
	resetSections()
	defer func() {
		resetSections()
		RegisterCoreSection("identity", identitySection)
		RegisterCoreSection("environment", environmentSection)
		RegisterProjectSection("claudemd", claudeMDSection)
		RegisterProjectSection("skills", skillsSection)
		RegisterProjectSection("permissions", permissionsSection)
	}()

	RegisterCoreSection("first", func(_ *PromptContext) string { return "AAA" })
	RegisterCoreSection("second", func(_ *PromptContext) string { return "BBB" })
	RegisterCoreSection("third", func(_ *PromptContext) string { return "CCC" })

	ctx := &PromptContext{}
	parts := renderSections(coreSections, ctx)
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(parts))
	}
	joined := strings.Join(parts, "|")
	if joined != "AAA|BBB|CCC" {
		t.Errorf("sections should maintain registration order, got %q", joined)
	}
}
