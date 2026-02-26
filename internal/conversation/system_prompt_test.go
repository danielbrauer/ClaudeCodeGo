package conversation

import (
	"runtime"
	"strings"
	"testing"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/config"
)

func testCtx() *PromptContext {
	return &PromptContext{
		CWD:   "/test",
		Model: api.ModelClaude46Opus,
	}
}

func TestSectionIdentity(t *testing.T) {
	ctx := testCtx()
	text := sectionIdentity(ctx)
	if !strings.Contains(text, "Claude Code") {
		t.Error("identity section should mention Claude Code")
	}
	if !strings.Contains(text, "interactive agent") {
		t.Error("identity section should mention interactive agent")
	}
	if !strings.Contains(text, "security testing") {
		t.Error("identity section should contain security guardrails")
	}
}

func TestSectionSystem(t *testing.T) {
	ctx := testCtx()
	text := sectionSystem(ctx)
	if !strings.HasPrefix(text, "# System") {
		t.Error("system section should start with # System header")
	}
	for _, want := range []string{
		"permission mode",
		"system-reminder",
		"prompt injection",
		"hooks",
		"context limits",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("system section should contain %q", want)
		}
	}
}

func TestSectionDoingTasks(t *testing.T) {
	ctx := testCtx()
	text := sectionDoingTasks(ctx)
	if !strings.HasPrefix(text, "# Doing tasks") {
		t.Error("doing tasks should start with header")
	}
	for _, want := range []string{
		"over-engineering",
		"security vulnerabilities",
		"OWASP top 10",
		"backwards-compatibility hacks",
		"premature abstraction",
		"time estimates",
		"brute force",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("doing tasks should contain %q", want)
		}
	}
}

func TestSectionActionCare(t *testing.T) {
	ctx := testCtx()
	text := sectionActionCare(ctx)
	if !strings.HasPrefix(text, "# Executing actions with care") {
		t.Error("action care should start with header")
	}
	for _, want := range []string{
		"reversibility and blast radius",
		"Destructive operations",
		"force-pushing",
		"measure twice, cut once",
		"CLAUDE.md files",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("action care should contain %q", want)
		}
	}
}

func TestSectionUsingTools(t *testing.T) {
	ctx := testCtx()
	text := sectionUsingTools(ctx)
	if !strings.HasPrefix(text, "# Using your tools") {
		t.Error("using tools should start with header")
	}
	for _, want := range []string{
		"Read instead of cat",
		"Edit instead of sed",
		"Write instead of cat with heredoc",
		"Glob instead of find",
		"Grep instead of grep",
		"parallel tool calls",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("using tools should contain %q", want)
		}
	}
}

func TestSectionToneStyle(t *testing.T) {
	ctx := testCtx()
	text := sectionToneStyle(ctx)
	if !strings.HasPrefix(text, "# Tone and style") {
		t.Error("tone style should start with header")
	}
	for _, want := range []string{
		"emojis",
		"concise",
		"file_path:line_number",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("tone style should contain %q", want)
		}
	}
}

func TestSectionEnvironment(t *testing.T) {
	ctx := &PromptContext{CWD: "/my/project", Model: api.ModelClaude46Opus}
	text := sectionEnvironment(ctx)
	if !strings.Contains(text, "/my/project") {
		t.Errorf("should include CWD, got: %s", text)
	}
	if !strings.Contains(text, runtime.GOOS) {
		t.Errorf("should include platform, got: %s", text)
	}
	if !strings.Contains(text, "Opus 4.6") {
		t.Errorf("should include model display name, got: %s", text)
	}
	if !strings.Contains(text, "knowledge cutoff") {
		t.Errorf("should include knowledge cutoff, got: %s", text)
	}
	if !strings.Contains(text, "fast_mode_info") {
		t.Errorf("should include fast mode info, got: %s", text)
	}
}

func TestSectionSkills_Empty(t *testing.T) {
	ctx := testCtx()
	if text := sectionSkills(ctx); text != "" {
		t.Errorf("empty skill content should produce empty string, got: %q", text)
	}
}

func TestSectionSkills_NonEmpty(t *testing.T) {
	ctx := &PromptContext{CWD: "/test", Model: api.ModelClaude46Opus, SkillContent: "some skill instructions"}
	text := sectionSkills(ctx)
	if !strings.HasPrefix(text, "# Active Skills") {
		t.Error("skills section should start with header")
	}
	if !strings.Contains(text, "some skill instructions") {
		t.Error("skills section should include content")
	}
}

func TestSectionPermissions_NilSettings(t *testing.T) {
	ctx := testCtx()
	if text := sectionPermissions(ctx); text != "" {
		t.Errorf("nil settings should produce empty string, got: %q", text)
	}
}

func TestSectionPermissions_EmptyRules(t *testing.T) {
	ctx := &PromptContext{CWD: "/test", Model: api.ModelClaude46Opus, Settings: &config.Settings{}}
	if text := sectionPermissions(ctx); text != "" {
		t.Errorf("empty permissions should produce empty string, got: %q", text)
	}
}

func TestSectionPermissions_WithRules(t *testing.T) {
	ctx := &PromptContext{
		CWD:   "/test",
		Model: api.ModelClaude46Opus,
		Settings: &config.Settings{
			Permissions: []config.PermissionRule{
				{Tool: "Bash", Pattern: "npm:*", Action: "allow"},
			},
		},
	}
	text := sectionPermissions(ctx)
	if !strings.HasPrefix(text, "# Permission Rules") {
		t.Error("permissions section should start with header")
	}
	if !strings.Contains(text, "Bash") {
		t.Error("permissions section should include tool name")
	}
}

func TestRenderSections(t *testing.T) {
	ctx := testCtx()
	sections := []PromptSection{
		func(_ *PromptContext) string { return "alpha" },
		func(_ *PromptContext) string { return "" }, // skipped
		func(_ *PromptContext) string { return "beta" },
	}
	got := renderSections(sections, ctx)
	want := "alpha\n\nbeta"
	if got != want {
		t.Errorf("renderSections = %q, want %q", got, want)
	}
}

func TestRenderSections_AllEmpty(t *testing.T) {
	ctx := testCtx()
	sections := []PromptSection{
		func(_ *PromptContext) string { return "" },
	}
	if got := renderSections(sections, ctx); got != "" {
		t.Errorf("all-empty sections should return empty string, got: %q", got)
	}
}

func TestRenderSections_Nil(t *testing.T) {
	if got := renderSections(nil, testCtx()); got != "" {
		t.Errorf("nil sections should return empty string, got: %q", got)
	}
}

func TestBuildSystemPrompt_Block1Structure(t *testing.T) {
	blocks := BuildSystemPrompt(testCtx())
	if len(blocks) < 1 {
		t.Fatal("expected at least 1 block")
	}
	if blocks[0].Type != "text" {
		t.Errorf("block 0 type = %q, want text", blocks[0].Type)
	}
	if !strings.Contains(blocks[0].Text, "Claude Code") {
		t.Error("Block 1 should contain identity")
	}
	if !strings.Contains(blocks[0].Text, "/test") {
		t.Error("Block 1 should contain CWD")
	}
}

func TestBuildSystemPrompt_NoProjectBlock(t *testing.T) {
	ctx := &PromptContext{CWD: "/nonexistent", Model: api.ModelClaude46Opus}
	blocks := BuildSystemPrompt(ctx)
	if len(blocks) == 0 {
		t.Fatal("expected at least 1 block")
	}
}

func TestBuildSystemPrompt_WithSkills(t *testing.T) {
	ctx := &PromptContext{CWD: "/nonexistent", Model: api.ModelClaude46Opus, SkillContent: "test skill"}
	blocks := BuildSystemPrompt(ctx)
	if len(blocks) < 2 {
		t.Fatal("expected 2 blocks with skill content")
	}
	if !strings.Contains(blocks[1].Text, "Active Skills") {
		t.Error("Block 2 should contain skills")
	}
}

func TestBuildSystemPrompt_WithPermissions(t *testing.T) {
	settings := &config.Settings{
		Permissions: []config.PermissionRule{
			{Tool: "Bash", Action: "allow"},
		},
	}
	ctx := &PromptContext{CWD: "/nonexistent", Model: api.ModelClaude46Opus, Settings: settings}
	blocks := BuildSystemPrompt(ctx)
	if len(blocks) < 2 {
		t.Fatal("expected 2 blocks with permissions")
	}
	if !strings.Contains(blocks[1].Text, "Permission Rules") {
		t.Error("Block 2 should contain permissions")
	}
}

func TestBuildSystemPrompt_MultipleProjectSections(t *testing.T) {
	settings := &config.Settings{
		Permissions: []config.PermissionRule{
			{Tool: "Read", Action: "allow"},
		},
	}
	ctx := &PromptContext{CWD: "/nonexistent", Model: api.ModelClaude46Opus, Settings: settings, SkillContent: "my skills"}
	blocks := BuildSystemPrompt(ctx)
	if len(blocks) < 2 {
		t.Fatal("expected 2 blocks")
	}
	if !strings.Contains(blocks[1].Text, "Active Skills") {
		t.Error("Block 2 should contain skills")
	}
	if !strings.Contains(blocks[1].Text, "Permission Rules") {
		t.Error("Block 2 should contain permissions")
	}
}

func TestRegisterCoreSection(t *testing.T) {
	orig := make([]PromptSection, len(coreSections))
	copy(orig, coreSections)
	defer func() { coreSections = orig }()

	RegisterCoreSection(func(_ *PromptContext) string {
		return "custom core"
	})

	blocks := BuildSystemPrompt(testCtx())
	if !strings.Contains(blocks[0].Text, "custom core") {
		t.Error("registered core section should appear in Block 1")
	}
}

func TestRegisterProjectSection(t *testing.T) {
	orig := make([]PromptSection, len(projectSections))
	copy(orig, projectSections)
	defer func() { projectSections = orig }()

	RegisterProjectSection(func(_ *PromptContext) string {
		return "custom project"
	})

	ctx := &PromptContext{CWD: "/nonexistent", Model: api.ModelClaude46Opus}
	blocks := BuildSystemPrompt(ctx)
	found := false
	for _, b := range blocks {
		if strings.Contains(b.Text, "custom project") {
			found = true
			break
		}
	}
	if !found {
		t.Error("registered project section should appear in blocks")
	}
}

func TestFormatPermissionRules(t *testing.T) {
	rules := []config.PermissionRule{
		{Tool: "Bash", Pattern: "npm:*", Action: "allow"},
		{Tool: "Read", Action: "deny"},
	}
	got := formatPermissionRules(rules)
	if !strings.Contains(got, "The following permission rules are configured:") {
		t.Error("should have header line")
	}
	if !strings.Contains(got, "allow") {
		t.Error("should contain action")
	}
}

func TestFormatPermissionRules_Empty(t *testing.T) {
	if got := formatPermissionRules(nil); got != "" {
		t.Errorf("nil rules should return empty, got: %q", got)
	}
}

func TestBuildSystemPrompt_WithGitStatus(t *testing.T) {
	ctx := &PromptContext{
		CWD:       "/test",
		Model:     api.ModelClaude46Opus,
		GitStatus: "Current branch: main\n\nStatus:\n(clean)",
	}
	blocks := BuildSystemPrompt(ctx)
	// Last block should be the gitStatus block (matching JS owq() pattern).
	lastBlock := blocks[len(blocks)-1]
	if !strings.HasPrefix(lastBlock.Text, "gitStatus: ") {
		t.Errorf("last block should start with 'gitStatus: ', got: %q", lastBlock.Text[:50])
	}
	if !strings.Contains(lastBlock.Text, "Current branch: main") {
		t.Error("gitStatus block should contain git status content")
	}
}

func TestBuildSystemPrompt_NoGitStatus(t *testing.T) {
	ctx := testCtx() // no GitStatus set
	blocks := BuildSystemPrompt(ctx)
	for _, b := range blocks {
		if strings.HasPrefix(b.Text, "gitStatus: ") {
			t.Error("should not have gitStatus block when GitStatus is empty")
		}
	}
}

func TestModelKnowledgeCutoff(t *testing.T) {
	tests := []struct {
		model string
		want  string
	}{
		{api.ModelClaude46Opus, "May 2025"},
		{api.ModelClaude46Sonnet, "August 2025"},
		{api.ModelClaude45Haiku, "February 2025"},
		{"unknown-model", ""},
	}
	for _, tt := range tests {
		got := modelKnowledgeCutoff(tt.model)
		if got != tt.want {
			t.Errorf("modelKnowledgeCutoff(%q) = %q, want %q", tt.model, got, tt.want)
		}
	}
}
