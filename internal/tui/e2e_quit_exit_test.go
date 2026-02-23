package tui

import (
	"testing"
)

func TestE2E_QuitCommand(t *testing.T) {
	m, _ := testModel(t)

	result, _ := submitCommand(m, "/quit")

	if !result.quitting {
		t.Error("quitting should be true after /quit")
	}
}

func TestE2E_ExitCommand(t *testing.T) {
	m, _ := testModel(t)

	result, _ := submitCommand(m, "/exit")

	if !result.quitting {
		t.Error("quitting should be true after /exit")
	}
}

func TestE2E_BareExitCommands(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"exit"},
		{"quit"},
		{":q"},
		{":q!"},
		{":wq"},
		{":wq!"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			m, _ := testModel(t)
			result, _ := submitCommand(m, tt.input)

			if !result.quitting {
				t.Errorf("quitting should be true for bare exit command %q", tt.input)
			}
		})
	}
}

func TestE2E_NonExitCommand_DoesNotQuit(t *testing.T) {
	m, _ := testModel(t)

	result, _ := submitCommand(m, "hello world")

	// Regular messages should switch to streaming mode, not quit.
	if result.quitting {
		t.Error("regular messages should not trigger quitting")
	}
	if result.mode != modeStreaming {
		t.Errorf("mode = %d, want modeStreaming (%d)", result.mode, modeStreaming)
	}
}

func TestE2E_IsExitCommand(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"exit", true},
		{"quit", true},
		{":q", true},
		{":q!", true},
		{":wq", true},
		{":wq!", true},
		{"hello", false},
		{"/exit", false}, // slash version is handled differently
		{"Exit", false},  // case sensitive
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isExitCommand(tt.input)
			if got != tt.want {
				t.Errorf("isExitCommand(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
