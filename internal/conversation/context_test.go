package conversation

import (
	"strings"
	"testing"
)

func TestBuildContextMessage_Empty(t *testing.T) {
	ctx := UserContext{}
	got := BuildContextMessage(ctx)
	if got != "" {
		t.Errorf("empty context should return empty string, got: %q", got)
	}
}

func TestBuildContextMessage_WithClaudeMD(t *testing.T) {
	ctx := UserContext{
		ClaudeMD: "Some project instructions",
	}
	got := BuildContextMessage(ctx)
	if !strings.Contains(got, "<system-reminder>") {
		t.Error("should contain system-reminder tag")
	}
	if !strings.Contains(got, "# claudeMd") {
		t.Error("should contain claudeMd section header")
	}
	if !strings.Contains(got, "Some project instructions") {
		t.Error("should contain CLAUDE.md content")
	}
	if !strings.Contains(got, "IMPORTANT: this context may or may not be relevant") {
		t.Error("should contain importance note")
	}
}

func TestBuildContextMessage_WithCurrentDate(t *testing.T) {
	ctx := UserContext{
		CurrentDate: "Today's date is 2026-02-26.",
	}
	got := BuildContextMessage(ctx)
	if !strings.Contains(got, "# currentDate") {
		t.Error("should contain currentDate section header")
	}
	if !strings.Contains(got, "2026-02-26") {
		t.Error("should contain date")
	}
}

func TestBuildContextMessage_WithGitStatus(t *testing.T) {
	ctx := UserContext{
		GitStatus: "Current branch: main\n\nStatus:\n(clean)",
	}
	got := BuildContextMessage(ctx)
	if !strings.Contains(got, "# gitStatus") {
		t.Error("should contain gitStatus section header")
	}
	if !strings.Contains(got, "Current branch: main") {
		t.Error("should contain git status content")
	}
}

func TestBuildContextMessage_AllFields(t *testing.T) {
	ctx := UserContext{
		ClaudeMD:    "# Project\nSome instructions",
		CurrentDate: "Today's date is 2026-02-26.",
		GitStatus:   "Current branch: feat-x\n\nStatus:\n(clean)",
	}
	got := BuildContextMessage(ctx)

	// Check structure: system-reminder wrapper.
	if !strings.HasPrefix(got, "<system-reminder>") {
		t.Error("should start with <system-reminder>")
	}
	if !strings.Contains(got, "</system-reminder>") {
		t.Error("should contain closing </system-reminder>")
	}

	// Check section ordering: claudeMd, currentDate, gitStatus.
	claudeIdx := strings.Index(got, "# claudeMd")
	dateIdx := strings.Index(got, "# currentDate")
	gitIdx := strings.Index(got, "# gitStatus")
	if claudeIdx == -1 || dateIdx == -1 || gitIdx == -1 {
		t.Fatal("all sections should be present")
	}
	if claudeIdx >= dateIdx || dateIdx >= gitIdx {
		t.Error("sections should appear in order: claudeMd, currentDate, gitStatus")
	}
}

func TestFormatCurrentDate(t *testing.T) {
	date := FormatCurrentDate()
	if !strings.HasPrefix(date, "Today's date is ") {
		t.Errorf("should start with 'Today's date is', got: %q", date)
	}
	if !strings.HasSuffix(date, ".") {
		t.Errorf("should end with period, got: %q", date)
	}
}
