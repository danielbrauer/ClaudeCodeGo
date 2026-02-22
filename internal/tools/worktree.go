package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// WorktreeInput is the input schema for the EnterWorktree tool.
type WorktreeInput struct {
	Name *string `json:"name,omitempty"`
}

// WorktreeTool creates an isolated git worktree.
type WorktreeTool struct {
	workDir string
}

// NewWorktreeTool creates a new EnterWorktree tool.
func NewWorktreeTool(workDir string) *WorktreeTool {
	return &WorktreeTool{workDir: workDir}
}

func (t *WorktreeTool) Name() string { return "EnterWorktree" }

func (t *WorktreeTool) Description() string {
	return `Create an isolated git worktree so an agent can work on a separate copy of the repo without affecting the main working directory. Optionally provide a name for the worktree.`
}

func (t *WorktreeTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "description": "Optional worktree name"
    }
  },
  "additionalProperties": false
}`)
}

func (t *WorktreeTool) RequiresPermission(_ json.RawMessage) bool {
	return true // creates files and git branches
}

func (t *WorktreeTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var in WorktreeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("parsing EnterWorktree input: %w", err)
	}

	// Verify we're in a git repo.
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-dir")
	cmd.Dir = t.workDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Sprintf("Error: not a git repository: %s", strings.TrimSpace(string(output))), nil
	}

	// Generate worktree name if not provided.
	name := "claude-worktree"
	if in.Name != nil && *in.Name != "" {
		name = *in.Name
	} else {
		name = fmt.Sprintf("claude-worktree-%d", time.Now().Unix())
	}

	// Create worktree path (sibling to the current repo).
	parentDir := filepath.Dir(t.workDir)
	worktreePath := filepath.Join(parentDir, name)
	branchName := name

	// Create the worktree.
	cmd = exec.CommandContext(ctx, "git", "worktree", "add", worktreePath, "-b", branchName)
	cmd.Dir = t.workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("Error creating worktree: %s", strings.TrimSpace(string(output))), nil
	}

	result := map[string]interface{}{
		"worktreePath":   worktreePath,
		"worktreeBranch": branchName,
		"message":        fmt.Sprintf("Created worktree at %s on branch %s", worktreePath, branchName),
	}
	out, _ := json.Marshal(result)
	return string(out), nil
}
