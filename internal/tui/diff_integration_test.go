package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requireGit skips the test if git is not available.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

// initGitRepo creates a temporary git repo with an initial commit and returns
// the directory path. It also changes the working directory to the repo and
// registers a cleanup to restore the original directory.
func initGitRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// Initialize repo with a file and initial commit.
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "config", "commit.gpgsign", "false"},
		{"git", "commit", "--allow-empty", "--no-gpg-sign", "-m", "initial"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git setup %v failed: %v\n%s", args, err, out)
		}
	}

	return dir
}

// gitExec runs a git command in the given directory.
func gitExec(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestIsGitRepo_True(t *testing.T) {
	requireGit(t)
	initGitRepo(t)

	if !isGitRepo() {
		t.Error("expected isGitRepo() == true inside a git repo")
	}
}

func TestIsGitRepo_False(t *testing.T) {
	requireGit(t)

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	os.Chdir(dir)

	if isGitRepo() {
		t.Error("expected isGitRepo() == false outside a git repo")
	}
}

func TestIsGitMergeState_Clean(t *testing.T) {
	requireGit(t)
	initGitRepo(t)

	if isGitMergeState() {
		t.Error("expected isGitMergeState() == false on a clean repo")
	}
}

func TestGetUntrackedFiles(t *testing.T) {
	requireGit(t)
	dir := initGitRepo(t)

	// Create untracked files.
	os.WriteFile(filepath.Join(dir, "untracked1.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "untracked2.txt"), []byte("b"), 0644)

	files := getUntrackedFiles(10)
	if len(files) != 2 {
		t.Fatalf("expected 2 untracked files, got %d", len(files))
	}

	paths := map[string]bool{}
	for _, f := range files {
		paths[f.path] = true
		if !f.isUntracked {
			t.Errorf("file %q should be marked as untracked", f.path)
		}
	}
	if !paths["untracked1.txt"] || !paths["untracked2.txt"] {
		t.Errorf("expected untracked1.txt and untracked2.txt, got %v", paths)
	}
}

func TestGetUntrackedFiles_Limit(t *testing.T) {
	requireGit(t)
	dir := initGitRepo(t)

	for i := 0; i < 5; i++ {
		os.WriteFile(filepath.Join(dir, "file"+string(rune('a'+i))+".txt"), []byte("x"), 0644)
	}

	files := getUntrackedFiles(3)
	if len(files) != 3 {
		t.Errorf("expected 3 untracked files (limited), got %d", len(files))
	}
}

func TestGetUntrackedFiles_NoUntracked(t *testing.T) {
	requireGit(t)
	initGitRepo(t)

	files := getUntrackedFiles(10)
	if len(files) != 0 {
		t.Errorf("expected 0 untracked files, got %d", len(files))
	}
}

func TestLoadDiffData_CleanRepo(t *testing.T) {
	requireGit(t)
	initGitRepo(t)

	d := loadDiffData()
	if d.errorMsg != "" {
		t.Fatalf("unexpected error: %s", d.errorMsg)
	}
	if d.stats.filesCount != 0 {
		t.Errorf("expected 0 changed files, got %d", d.stats.filesCount)
	}
	if len(d.files) != 0 {
		t.Errorf("expected empty files list, got %d files", len(d.files))
	}
}

func TestLoadDiffData_WithModifiedFile(t *testing.T) {
	requireGit(t)
	dir := initGitRepo(t)

	// Create and commit a file.
	filePath := filepath.Join(dir, "hello.txt")
	os.WriteFile(filePath, []byte("hello\nworld\n"), 0644)
	gitExec(t, dir, "add", "hello.txt")
	gitExec(t, dir, "commit", "--no-gpg-sign", "-m", "add hello")

	// Modify it.
	os.WriteFile(filePath, []byte("hello\ngo world\nextra line\n"), 0644)

	d := loadDiffData()
	if d.errorMsg != "" {
		t.Fatalf("unexpected error: %s", d.errorMsg)
	}
	if d.stats.filesCount != 1 {
		t.Errorf("expected 1 changed file, got %d", d.stats.filesCount)
	}
	if d.stats.linesAdded != 2 {
		t.Errorf("expected 2 lines added, got %d", d.stats.linesAdded)
	}
	if d.stats.linesRemoved != 1 {
		t.Errorf("expected 1 line removed, got %d", d.stats.linesRemoved)
	}
	if len(d.files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(d.files))
	}
	if d.files[0].path != "hello.txt" {
		t.Errorf("expected file path 'hello.txt', got %q", d.files[0].path)
	}

	// Verify hunks were parsed.
	hunks, ok := d.hunks["hello.txt"]
	if !ok || len(hunks) == 0 {
		t.Fatal("expected hunks for hello.txt")
	}
	// Verify hunk content contains the expected changes.
	hunkContent := strings.Join(hunks[0].lines, "\n")
	if !strings.Contains(hunkContent, "+go world") {
		t.Error("expected hunk to contain '+go world'")
	}
	if !strings.Contains(hunkContent, "-world") {
		t.Error("expected hunk to contain '-world'")
	}
}

func TestLoadDiffData_WithUntrackedFiles(t *testing.T) {
	requireGit(t)
	dir := initGitRepo(t)

	// Create an untracked file (not staged/committed).
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new content\n"), 0644)

	d := loadDiffData()
	if d.errorMsg != "" {
		t.Fatalf("unexpected error: %s", d.errorMsg)
	}

	// Should include the untracked file in the files list.
	if d.stats.filesCount != 1 {
		t.Errorf("expected 1 file (untracked), got %d", d.stats.filesCount)
	}
	if len(d.files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(d.files))
	}
	if d.files[0].path != "new.txt" {
		t.Errorf("expected file path 'new.txt', got %q", d.files[0].path)
	}
	if !d.files[0].isUntracked {
		t.Error("expected file to be marked as untracked")
	}
}

func TestLoadDiffData_WithStagedChanges(t *testing.T) {
	requireGit(t)
	dir := initGitRepo(t)

	// Create, commit, modify, and stage a file.
	filePath := filepath.Join(dir, "staged.txt")
	os.WriteFile(filePath, []byte("original\n"), 0644)
	gitExec(t, dir, "add", "staged.txt")
	gitExec(t, dir, "commit", "--no-gpg-sign", "-m", "add staged")

	os.WriteFile(filePath, []byte("modified\n"), 0644)
	gitExec(t, dir, "add", "staged.txt")

	d := loadDiffData()
	if d.errorMsg != "" {
		t.Fatalf("unexpected error: %s", d.errorMsg)
	}
	if d.stats.filesCount != 1 {
		t.Errorf("expected 1 changed file, got %d", d.stats.filesCount)
	}
	if d.stats.linesAdded != 1 {
		t.Errorf("expected 1 line added, got %d", d.stats.linesAdded)
	}
	if d.stats.linesRemoved != 1 {
		t.Errorf("expected 1 line removed, got %d", d.stats.linesRemoved)
	}
}

func TestLoadDiffData_MultipleFiles(t *testing.T) {
	requireGit(t)
	dir := initGitRepo(t)

	// Create and commit two files.
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bbb\n"), 0644)
	gitExec(t, dir, "add", ".")
	gitExec(t, dir, "commit", "--no-gpg-sign", "-m", "add files")

	// Modify both.
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa modified\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bbb modified\nextra\n"), 0644)

	d := loadDiffData()
	if d.errorMsg != "" {
		t.Fatalf("unexpected error: %s", d.errorMsg)
	}
	if d.stats.filesCount != 2 {
		t.Errorf("expected 2 changed files, got %d", d.stats.filesCount)
	}
	if len(d.files) != 2 {
		t.Errorf("expected 2 files in list, got %d", len(d.files))
	}
	if len(d.hunks) != 2 {
		t.Errorf("expected hunks for 2 files, got %d", len(d.hunks))
	}
}

func TestLoadDiffData_NotGitRepo(t *testing.T) {
	requireGit(t)

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) })
	os.Chdir(dir)

	d := loadDiffData()
	if d.errorMsg == "" {
		t.Error("expected error message for non-git directory")
	}
	if !strings.Contains(d.errorMsg, "Not a git repository") {
		t.Errorf("expected 'Not a git repository' error, got %q", d.errorMsg)
	}
}

func TestLoadDiffData_DeletedFile(t *testing.T) {
	requireGit(t)
	dir := initGitRepo(t)

	// Create, commit, then delete a file.
	filePath := filepath.Join(dir, "deleteme.txt")
	os.WriteFile(filePath, []byte("line1\nline2\nline3\n"), 0644)
	gitExec(t, dir, "add", "deleteme.txt")
	gitExec(t, dir, "commit", "--no-gpg-sign", "-m", "add deleteme")

	os.Remove(filePath)

	d := loadDiffData()
	if d.errorMsg != "" {
		t.Fatalf("unexpected error: %s", d.errorMsg)
	}
	if d.stats.filesCount != 1 {
		t.Errorf("expected 1 changed file, got %d", d.stats.filesCount)
	}
	if d.stats.linesRemoved != 3 {
		t.Errorf("expected 3 lines removed, got %d", d.stats.linesRemoved)
	}
	if d.stats.linesAdded != 0 {
		t.Errorf("expected 0 lines added, got %d", d.stats.linesAdded)
	}
}

func TestRunGit(t *testing.T) {
	requireGit(t)
	initGitRepo(t)

	out, err := runGit("rev-parse", "--is-inside-work-tree")
	if err != nil {
		t.Fatalf("runGit failed: %v", err)
	}
	if strings.TrimSpace(out) != "true" {
		t.Errorf("expected 'true', got %q", strings.TrimSpace(out))
	}
}

func TestRunGit_InvalidCommand(t *testing.T) {
	requireGit(t)
	initGitRepo(t)

	_, err := runGit("not-a-real-command")
	if err == nil {
		t.Error("expected error for invalid git command")
	}
}

func TestGitPath(t *testing.T) {
	requireGit(t)
	p := gitPath()
	if p == "" {
		t.Error("gitPath() returned empty string")
	}
}
