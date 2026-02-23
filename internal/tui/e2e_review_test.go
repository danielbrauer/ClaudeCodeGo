package tui

import (
	"strings"
	"testing"
)

func TestE2E_ReviewCommand_SwitchesToStreaming(t *testing.T) {
	m, _ := testModel(t)

	result, _ := submitCommand(m, "/review")

	if result.mode != modeStreaming {
		t.Errorf("mode = %d, want modeStreaming (%d)", result.mode, modeStreaming)
	}
}

func TestE2E_ReviewCommand_WithPRNumber(t *testing.T) {
	m, _ := testModel(t)

	result, _ := submitCommand(m, "/review 42")

	if result.mode != modeStreaming {
		t.Errorf("mode = %d, want modeStreaming (%d)", result.mode, modeStreaming)
	}
}

func TestE2E_ReviewCommand_BlursInput(t *testing.T) {
	m, _ := testModel(t)

	result, _ := submitCommand(m, "/review")

	if result.textInput.Focused() {
		t.Error("text input should be blurred during /review streaming")
	}
}

func TestE2E_BuildReviewPrompt_NoPR(t *testing.T) {
	prompt := buildReviewPrompt("")

	if !strings.Contains(prompt, "gh pr list") {
		t.Error("review prompt with no PR should mention gh pr list")
	}
	if !strings.Contains(prompt, "code review") {
		t.Error("review prompt should mention code review")
	}
}

func TestE2E_BuildReviewPrompt_WithPR(t *testing.T) {
	prompt := buildReviewPrompt("123")

	if !strings.Contains(prompt, "gh pr view") {
		t.Error("review prompt with PR should mention gh pr view")
	}
	if !strings.Contains(prompt, "gh pr diff") {
		t.Error("review prompt should mention gh pr diff")
	}
	if !strings.Contains(prompt, "123") {
		t.Error("review prompt should include the PR number")
	}
}
