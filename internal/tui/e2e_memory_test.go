package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestE2E_MemoryCommand_ProjectArg(t *testing.T) {
	t.Setenv("EDITOR", "true") // "true" is always available on Unix

	cwd, _ := os.Getwd()
	path := memoryFilePath("project", cwd)
	want := filepath.Join(cwd, "CLAUDE.md")

	if path != want {
		t.Errorf("memoryFilePath('project', cwd) = %q, want %q", path, want)
	}
}

func TestE2E_MemoryCommand_UserDefault(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}

	cwd := "/tmp"
	path := memoryFilePath("", cwd)
	want := filepath.Join(home, ".claude", "CLAUDE.md")

	if path != want {
		t.Errorf("memoryFilePath('', cwd) = %q, want %q", path, want)
	}
}

func TestE2E_MemoryCommand_HandleSubmit_NoEditor(t *testing.T) {
	// Clear all editor env vars and PATH to force "no editor" error.
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	t.Setenv("PATH", "")

	m, _ := testModel(t)

	// /memory should try to open editor and return an error when none found.
	// The error is displayed but doesn't crash the model.
	result, _ := submitCommand(m, "/memory")

	// The model should remain functional (not crash).
	// With no editor, the command returns immediately with error output.
	_ = result
}

func TestE2E_MemoryCommand_WithArg(t *testing.T) {
	cwd := "/fake/project"
	path := memoryFilePath("project", cwd)
	if path != filepath.Join(cwd, "CLAUDE.md") {
		t.Errorf("memoryFilePath('project', %q) = %q", cwd, path)
	}

	path = memoryFilePath("user", cwd)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}
	if path != filepath.Join(home, ".claude", "CLAUDE.md") {
		t.Errorf("memoryFilePath('user', %q) = %q", cwd, path)
	}
}
