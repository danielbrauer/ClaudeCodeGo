package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileExists(t *testing.T) {
	// Existing file.
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.md")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !fileExists(path) {
		t.Error("fileExists returned false for an existing file")
	}

	// Non-existent file.
	if fileExists(filepath.Join(tmp, "nope.md")) {
		t.Error("fileExists returned true for a non-existent file")
	}

	// Directory should not count as a file.
	if fileExists(tmp) {
		t.Error("fileExists returned true for a directory")
	}
}

func TestMemoryFilePath_Project(t *testing.T) {
	cwd := "/fake/project"
	got := memoryFilePath("project", cwd)
	want := filepath.Join(cwd, "CLAUDE.md")
	if got != want {
		t.Errorf("memoryFilePath(\"project\", %q) = %q, want %q", cwd, got, want)
	}
}

func TestMemoryFilePath_User(t *testing.T) {
	cwd := "/fake/project"
	got := memoryFilePath("", cwd)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}
	want := filepath.Join(home, ".claude", "CLAUDE.md")
	if got != want {
		t.Errorf("memoryFilePath(\"\", %q) = %q, want %q", cwd, got, want)
	}
}

func TestMemoryFilePath_UnknownArgDefaultsToUser(t *testing.T) {
	cwd := "/fake/project"
	got := memoryFilePath("something-else", cwd)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}
	want := filepath.Join(home, ".claude", "CLAUDE.md")
	if got != want {
		t.Errorf("memoryFilePath(\"something-else\", %q) = %q, want %q", cwd, got, want)
	}
}

func TestShortenPath_Home(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}
	path := filepath.Join(home, ".claude", "CLAUDE.md")
	got := shortenPath(path)
	if !strings.HasPrefix(got, "~") {
		t.Errorf("shortenPath(%q) = %q, want ~ prefix", path, got)
	}
	if got != "~/.claude/CLAUDE.md" {
		t.Errorf("shortenPath(%q) = %q, want %q", path, got, "~/.claude/CLAUDE.md")
	}
}

func TestShortenPath_Absolute(t *testing.T) {
	// A path outside both home and cwd should be returned as-is.
	got := shortenPath("/some/random/absolute/path.md")
	if got != "/some/random/absolute/path.md" {
		t.Errorf("shortenPath returned %q, want unchanged absolute path", got)
	}
}

func TestFindEditor_VISUAL(t *testing.T) {
	t.Setenv("VISUAL", "emacs")
	t.Setenv("EDITOR", "vim")

	name, cmd := findEditor()
	if name != "$VISUAL" {
		t.Errorf("findEditor() name = %q, want $VISUAL", name)
	}
	if cmd != "emacs" {
		t.Errorf("findEditor() cmd = %q, want emacs", cmd)
	}
}

func TestFindEditor_EDITOR(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "nano")

	name, cmd := findEditor()
	if name != "$EDITOR" {
		t.Errorf("findEditor() name = %q, want $EDITOR", name)
	}
	if cmd != "nano" {
		t.Errorf("findEditor() cmd = %q, want nano", cmd)
	}
}

func TestFindEditor_WhitespaceOnly(t *testing.T) {
	t.Setenv("VISUAL", "   ")
	t.Setenv("EDITOR", "  ")

	// Should skip whitespace-only values and fall through.
	name, _ := findEditor()
	if name == "$VISUAL" || name == "$EDITOR" {
		t.Errorf("findEditor() should skip whitespace-only env vars, got name=%q", name)
	}
}

func TestEditorCommand_CreatesFileAndDirs(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "true") // "true" is always available on Unix

	tmp := t.TempDir()
	path := filepath.Join(tmp, "sub", "dir", "CLAUDE.md")

	cmd, err := editorCommand(path)
	if err != nil {
		t.Fatalf("editorCommand() error: %v", err)
	}
	if cmd == nil {
		t.Fatal("editorCommand() returned nil cmd")
	}

	// The file should have been created.
	if !fileExists(path) {
		t.Error("editorCommand() did not create the file")
	}

	// The parent directory should exist.
	info, err := os.Stat(filepath.Join(tmp, "sub", "dir"))
	if err != nil {
		t.Fatalf("parent directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("parent path is not a directory")
	}
}

func TestEditorCommand_NoEditor(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	t.Setenv("PATH", "") // ensure no fallback editors are found

	tmp := t.TempDir()
	path := filepath.Join(tmp, "CLAUDE.md")

	_, err := editorCommand(path)
	if err == nil {
		t.Fatal("editorCommand() should return error when no editor is found")
	}
	if !strings.Contains(err.Error(), "no editor found") {
		t.Errorf("error should mention no editor found, got: %v", err)
	}
}

func TestEditorCommand_ExistingFile(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "true")

	tmp := t.TempDir()
	path := filepath.Join(tmp, "CLAUDE.md")
	if err := os.WriteFile(path, []byte("existing content"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd, err := editorCommand(path)
	if err != nil {
		t.Fatalf("editorCommand() error: %v", err)
	}
	if cmd == nil {
		t.Fatal("editorCommand() returned nil cmd")
	}

	// Content should be preserved (not overwritten).
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "existing content" {
		t.Errorf("editorCommand() overwrote existing file, got %q", string(data))
	}
}

func TestEditorCommand_SplitsArgs(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "code --wait")

	tmp := t.TempDir()
	path := filepath.Join(tmp, "CLAUDE.md")

	cmd, err := editorCommand(path)
	if err != nil {
		t.Fatalf("editorCommand() error: %v", err)
	}

	// cmd.Path should be "code" and args should include "--wait" and the path.
	if !strings.HasSuffix(cmd.Path, "code") && cmd.Args[0] != "code" {
		t.Errorf("expected command to be 'code', got Path=%q Args[0]=%q", cmd.Path, cmd.Args[0])
	}
	if len(cmd.Args) < 3 || cmd.Args[1] != "--wait" {
		t.Errorf("expected --wait in args, got %v", cmd.Args)
	}
	if cmd.Args[len(cmd.Args)-1] != path {
		t.Errorf("expected file path as last arg, got %v", cmd.Args)
	}
}

func TestEditorHintMessage_EnvVar(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "vim")

	msg := editorHintMessage("/home/user/.claude/CLAUDE.md")
	if !strings.Contains(msg, "Opened memory file at") {
		t.Errorf("message should contain 'Opened memory file at', got: %s", msg)
	}
	if !strings.Contains(msg, "$EDITOR") {
		t.Errorf("message should mention $EDITOR, got: %s", msg)
	}
	if !strings.Contains(msg, "vim") {
		t.Errorf("message should mention the editor command, got: %s", msg)
	}
}

func TestEditorHintMessage_Fallback(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	msg := editorHintMessage("/some/path/CLAUDE.md")
	// Even with no editor env set, the hint message function should work.
	if !strings.Contains(msg, "Opened memory file at") {
		t.Errorf("message should contain 'Opened memory file at', got: %s", msg)
	}
}
