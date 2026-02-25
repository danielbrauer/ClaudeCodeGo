package conversation

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/claude-code-go/internal/config"
)

func TestSectionIdentity(t *testing.T) {
	ctx := &PromptContext{CWD: "/test"}
	text := sectionIdentity(ctx)
	if !strings.Contains(text, "Claude Code") {
		t.Error("identity section should mention Claude Code")
	}
	if !strings.Contains(text, "tools") {
		t.Error("identity section should mention tools")
	}
}

func TestSectionSecurityGuardrails(t *testing.T) {
	ctx := &PromptContext{}
	text := sectionSecurityGuardrails(ctx)
	for _, want := range []string{
		"authorized security testing",
		"CTF challenges",
		"DoS attacks",
		"supply chain compromise",
		"Dual-use security tools",
		"pentesting engagements",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("security guardrails should contain %q", want)
		}
	}
}

func TestSectionTaskPhilosophy(t *testing.T) {
	ctx := &PromptContext{}
	text := sectionTaskPhilosophy(ctx)
	if !strings.HasPrefix(text, "# Doing tasks") {
		t.Error("task philosophy should start with Doing tasks header")
	}
	for _, want := range []string{
		"over-engineering",
		"security vulnerabilities",
		"OWASP top 10",
		"backwards-compatibility hacks",
		"premature abstraction",
		"time estimates",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("task philosophy should contain %q", want)
		}
	}
}

func TestSectionActionCare(t *testing.T) {
	ctx := &PromptContext{}
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

func TestSectionEnvironment(t *testing.T) {
	ctx := &PromptContext{CWD: "/my/project"}
	text := sectionEnvironment(ctx)
	if !strings.Contains(text, "/my/project") {
		t.Errorf("should include CWD, got: %s", text)
	}
	if !strings.Contains(text, runtime.GOOS) {
		t.Errorf("should include OS, got: %s", text)
	}
	if !strings.Contains(text, time.Now().Format("2006-01-02")) {
		t.Errorf("should include today's date, got: %s", text)
	}
}

func TestSectionSkills_Empty(t *testing.T) {
	ctx := &PromptContext{}
	if text := sectionSkills(ctx); text != "" {
		t.Errorf("empty skill content should produce empty string, got: %q", text)
	}
}

func TestSectionSkills_NonEmpty(t *testing.T) {
	ctx := &PromptContext{SkillContent: "some skill instructions"}
	text := sectionSkills(ctx)
	if !strings.HasPrefix(text, "# Active Skills") {
		t.Error("skills section should start with header")
	}
	if !strings.Contains(text, "some skill instructions") {
		t.Error("skills section should include content")
	}
}

func TestSectionPermissions_NilSettings(t *testing.T) {
	ctx := &PromptContext{}
	if text := sectionPermissions(ctx); text != "" {
		t.Errorf("nil settings should produce empty string, got: %q", text)
	}
}

func TestSectionPermissions_EmptyRules(t *testing.T) {
	ctx := &PromptContext{Settings: &config.Settings{}}
	if text := sectionPermissions(ctx); text != "" {
		t.Errorf("empty permissions should produce empty string, got: %q", text)
	}
}

func TestSectionPermissions_WithRules(t *testing.T) {
	ctx := &PromptContext{
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
	ctx := &PromptContext{}
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
	ctx := &PromptContext{}
	sections := []PromptSection{
		func(_ *PromptContext) string { return "" },
	}
	if got := renderSections(sections, ctx); got != "" {
		t.Errorf("all-empty sections should return empty string, got: %q", got)
	}
}

func TestRenderSections_Nil(t *testing.T) {
	if got := renderSections(nil, &PromptContext{}); got != "" {
		t.Errorf("nil sections should return empty string, got: %q", got)
	}
}

func TestBuildSystemPrompt_Block1Structure(t *testing.T) {
	blocks := BuildSystemPrompt("/test", nil, "")
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
	// With no CLAUDE.md, no skills, nil settings → only Block 1
	blocks := BuildSystemPrompt("/nonexistent", nil, "")
	// Block 1 is always present; Block 2 may or may not be depending on
	// whether LoadClaudeMD finds anything at the path. We just verify
	// Block 1 exists and is well-formed.
	if len(blocks) == 0 {
		t.Fatal("expected at least 1 block")
	}
}

func TestBuildSystemPrompt_WithSkills(t *testing.T) {
	blocks := BuildSystemPrompt("/nonexistent", nil, "test skill")
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
	blocks := BuildSystemPrompt("/nonexistent", settings, "")
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
	blocks := BuildSystemPrompt("/nonexistent", settings, "my skills")
	if len(blocks) < 2 {
		t.Fatal("expected 2 blocks")
	}
	// Block 2 should have both skills and permissions, separated by \n\n
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

	blocks := BuildSystemPrompt("/test", nil, "")
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

	blocks := BuildSystemPrompt("/nonexistent", nil, "")
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
