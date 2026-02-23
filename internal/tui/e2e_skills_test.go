package tui

import (
	"strings"
	"testing"

	"github.com/anthropics/claude-code-go/internal/skills"
)

func TestE2E_SkillCommand_RegistersAndExecutes(t *testing.T) {
	loadedSkills := []skills.Skill{
		{
			Name:        "commit",
			Description: "Create a commit",
			Trigger:     "/commit",
			Content:     "Please create a well-structured commit message.",
		},
	}
	m, _ := testModel(t, withSkills(loadedSkills))

	// The skill should be registered.
	cmd, ok := m.slashReg.lookup("commit")
	if !ok {
		t.Fatal("/commit not registered")
	}
	if cmd.Description != "Create a commit" {
		t.Errorf("description = %q, want 'Create a commit'", cmd.Description)
	}

	// Execute should return the skill content with sentinel prefix.
	output := cmd.Execute(&m)
	if !strings.HasPrefix(output, skillCommandPrefix) {
		t.Errorf("output should have skill prefix, got %q", output)
	}
	content := strings.TrimPrefix(output, skillCommandPrefix)
	if content != "Please create a well-structured commit message." {
		t.Errorf("skill content = %q", content)
	}
}

func TestE2E_SkillCommand_ViaHandleSubmit(t *testing.T) {
	loadedSkills := []skills.Skill{
		{
			Name:        "deploy",
			Description: "Deploy the app",
			Trigger:     "/deploy",
			Content:     "Run the deployment pipeline for this project.",
		},
	}
	m, _ := testModel(t, withSkills(loadedSkills))

	result, _ := submitCommand(m, "/deploy")

	// Skill commands should switch to streaming mode (content sent to loop).
	if result.mode != modeStreaming {
		t.Errorf("mode = %d, want modeStreaming (%d)", result.mode, modeStreaming)
	}
}

func TestE2E_SkillCommand_NoTrigger_NotRegistered(t *testing.T) {
	loadedSkills := []skills.Skill{
		{
			Name:        "no-trigger",
			Description: "Skill without trigger",
			Trigger:     "",
			Content:     "ignored",
		},
	}
	m, _ := testModel(t, withSkills(loadedSkills))

	if _, ok := m.slashReg.lookup("no-trigger"); ok {
		t.Error("skill without trigger should not be registered")
	}
}

func TestE2E_SkillCommand_MultipleSkills(t *testing.T) {
	loadedSkills := []skills.Skill{
		{
			Name:    "commit",
			Trigger: "/commit",
			Content: "commit content",
		},
		{
			Name:    "review",
			Trigger: "/pr-review",
			Content: "review content",
		},
		{
			Name:    "no-trigger",
			Trigger: "",
			Content: "ignored",
		},
	}
	m, _ := testModel(t, withSkills(loadedSkills))

	// commit should be registered.
	if _, ok := m.slashReg.lookup("commit"); !ok {
		t.Error("/commit should be registered")
	}

	// pr-review should be registered.
	if _, ok := m.slashReg.lookup("pr-review"); !ok {
		t.Error("/pr-review should be registered")
	}

	// no-trigger should NOT be registered.
	if _, ok := m.slashReg.lookup("no-trigger"); ok {
		t.Error("no-trigger should not be registered")
	}
}

func TestE2E_SkillCommand_AppersInCompletion(t *testing.T) {
	loadedSkills := []skills.Skill{
		{
			Name:    "commit",
			Trigger: "/commit",
			Content: "content",
		},
	}
	m, _ := testModel(t, withSkills(loadedSkills))

	matches := m.slashReg.complete("comm")
	found := false
	for _, name := range matches {
		if name == "commit" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("complete('comm') should include 'commit', got %v", matches)
	}
}
