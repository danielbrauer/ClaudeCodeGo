package conversation

import (
	"strings"
	"testing"

	"github.com/anthropics/claude-code-go/internal/api"
	"github.com/anthropics/claude-code-go/internal/config"
)

func TestWithPlanModePrompt_PlanMode(t *testing.T) {
	base := []api.SystemBlock{
		{Type: "text", Text: "You are Claude Code."},
	}

	result := WithPlanModePrompt(base, config.ModePlan)

	if len(result) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(result))
	}

	if !strings.Contains(result[1].Text, "Plan mode is active") {
		t.Error("expected plan mode system prompt in second block")
	}
}

func TestWithPlanModePrompt_DefaultMode(t *testing.T) {
	base := []api.SystemBlock{
		{Type: "text", Text: "You are Claude Code."},
	}

	result := WithPlanModePrompt(base, config.ModeDefault)

	if len(result) != 1 {
		t.Fatalf("expected 1 block (unmodified), got %d", len(result))
	}
}

func TestWithPlanModePrompt_EmptyMode(t *testing.T) {
	base := []api.SystemBlock{
		{Type: "text", Text: "identity"},
		{Type: "text", Text: "project"},
	}

	result := WithPlanModePrompt(base, "")

	if len(result) != 2 {
		t.Fatalf("empty mode should not add blocks: got %d", len(result))
	}
}

func TestWithPlanModePrompt_AcceptEditsMode(t *testing.T) {
	base := []api.SystemBlock{
		{Type: "text", Text: "identity"},
	}

	result := WithPlanModePrompt(base, config.ModeAcceptEdits)

	if len(result) != 1 {
		t.Fatalf("acceptEdits should not add blocks: got %d", len(result))
	}
}

func TestWithPlanModePrompt_DoesNotMutateOriginal(t *testing.T) {
	base := []api.SystemBlock{
		{Type: "text", Text: "identity"},
	}
	original := make([]api.SystemBlock, len(base))
	copy(original, base)

	_ = WithPlanModePrompt(base, config.ModePlan)

	if len(base) != len(original) {
		t.Error("WithPlanModePrompt should not mutate the original slice")
	}
}

func TestPlanModeSystemPromptContent(t *testing.T) {
	// Verify key phrases are present.
	checks := []string{
		"Plan mode is active",
		"MUST NOT make any edits",
		"ExitPlanMode",
		"Read, Glob, Grep",
	}
	for _, check := range checks {
		if !strings.Contains(PlanModeSystemPrompt, check) {
			t.Errorf("PlanModeSystemPrompt missing %q", check)
		}
	}
}
