package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// findEditor returns the editor command to use, checking $VISUAL, $EDITOR,
// then falling back to common editors.
func findEditor() (envName string, cmdStr string) {
	if v := strings.TrimSpace(os.Getenv("VISUAL")); v != "" {
		return "$VISUAL", v
	}
	if e := strings.TrimSpace(os.Getenv("EDITOR")); e != "" {
		return "$EDITOR", e
	}
	if runtime.GOOS == "windows" {
		return "notepad", "notepad"
	}
	// Try common editors.
	for _, candidate := range []string{"code", "vi", "nano"} {
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate, candidate
		}
	}
	return "", ""
}

// editorCommand returns an *exec.Cmd for opening the given file in the user's
// preferred editor. It creates the file and parent directories if needed.
// Returns nil and an error message if no editor is found.
func editorCommand(path string) (*exec.Cmd, error) {
	// Ensure parent directory exists.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating directory %s: %w", dir, err)
	}

	// Create the file if it doesn't exist.
	if !fileExists(path) {
		if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
			return nil, fmt.Errorf("creating file %s: %w", path, err)
		}
	}

	_, cmdStr := findEditor()
	if cmdStr == "" {
		return nil, fmt.Errorf("no editor found; set the $EDITOR or $VISUAL environment variable")
	}

	// Split on spaces to handle EDITOR="code --wait".
	parts := strings.Fields(cmdStr)
	parts = append(parts, path)
	return exec.Command(parts[0], parts[1:]...), nil
}

// editorHintMessage returns a user-facing message after opening the editor.
func editorHintMessage(path string) string {
	displayPath := shortenPath(path)
	envName, cmdStr := findEditor()
	var hint string
	if envName != "" && envName[0] == '$' {
		hint = fmt.Sprintf("> Using %s=%q. To change editor, set $EDITOR or $VISUAL environment variable.", envName, cmdStr)
	} else {
		hint = "> To use a different editor, set the $EDITOR or $VISUAL environment variable."
	}
	return fmt.Sprintf("Opened memory file at %s\n\n%s", displayPath, hint)
}

// memoryFilePath returns the path to the memory file based on the argument.
// "project" returns ./CLAUDE.md, anything else returns ~/.claude/CLAUDE.md.
func memoryFilePath(arg string, cwd string) string {
	if arg == "project" {
		return filepath.Join(cwd, "CLAUDE.md")
	}
	// Default: user memory.
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".claude", "CLAUDE.md")
	}
	// Fallback if homedir unavailable.
	return filepath.Join(cwd, "CLAUDE.md")
}

// shortenPath returns a display-friendly version of an absolute path,
// using ~ for home dir or ./ for project-relative paths.
func shortenPath(path string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if strings.HasPrefix(path, home) {
			return "~" + path[len(home):]
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, path); err == nil && !strings.HasPrefix(rel, "..") {
			return "./" + rel
		}
	}
	return path
}

// fileExists returns true if the given path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
