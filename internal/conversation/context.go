package conversation

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// UserContext holds context data that gets injected into user messages
// as <system-reminder> blocks, matching the JS CLI's TN1() pattern.
// Note: gitStatus goes into the system prompt (via owq()), not here.
type UserContext struct {
	ClaudeMD    string // formatted CLAUDE.md content with path annotations
	CurrentDate string // "Today's date is YYYY-MM-DD."
}

// CollectGitStatus gathers git status information for the working directory.
// Returns empty string if not a git repo or on error.
// Matches the JS CLI's TM8() function format.
func CollectGitStatus(cwd string) string {
	// Check if this is a git repo.
	if !isGitRepo(cwd) {
		return ""
	}

	// Collect branch, main branch, status, and recent commits in parallel.
	branchCh := make(chan string, 1)
	mainCh := make(chan string, 1)
	statusCh := make(chan string, 1)
	logCh := make(chan string, 1)

	go func() { branchCh <- gitCurrentBranch(cwd) }()
	go func() { mainCh <- gitMainBranch(cwd) }()
	go func() { statusCh <- gitStatusShort(cwd) }()
	go func() { logCh <- gitRecentCommits(cwd) }()

	branch := <-branchCh
	mainBranch := <-mainCh
	status := <-statusCh
	recentCommits := <-logCh

	if status == "" {
		status = "(clean)"
	}

	// Truncate status if too long (matching JS's 40k char limit).
	const maxStatusLen = 40000
	if len(status) > maxStatusLen {
		status = status[:maxStatusLen] + "\n... (truncated because it exceeds 40k characters. If you need more information, run \"git status\" using BashTool)"
	}

	return fmt.Sprintf(`This is the git status at the start of the conversation. Note that this status is a snapshot in time, and will not update during the conversation.
Current branch: %s

Main branch (you will usually use this for PRs): %s

Status:
%s

Recent commits:
%s`, branch, mainBranch, status, recentCommits)
}

// FormatCurrentDate returns the date string matching JS CLI format.
func FormatCurrentDate() string {
	return fmt.Sprintf("Today's date is %s.", time.Now().Format("2006-01-02"))
}

// BuildContextMessage creates the <system-reminder> context message that gets
// prepended to conversation messages. This matches the JS CLI's TN1() function.
// Returns empty string if there's no context to inject.
func BuildContextMessage(ctx UserContext) string {
	entries := make(map[string]string)

	if ctx.ClaudeMD != "" {
		entries["claudeMd"] = ctx.ClaudeMD
	}
	if ctx.CurrentDate != "" {
		entries["currentDate"] = ctx.CurrentDate
	}

	if len(entries) == 0 {
		return ""
	}

	var sections []string
	// Emit in a stable order matching JS CLI.
	for _, key := range []string{"claudeMd", "currentDate"} {
		if val, ok := entries[key]; ok {
			sections = append(sections, fmt.Sprintf("# %s\n%s", key, val))
		}
	}

	return fmt.Sprintf(`<system-reminder>
As you answer the user's questions, you can use the following context:
%s

      IMPORTANT: this context may or may not be relevant to your tasks. You should not respond to this context unless it is highly relevant to your task.
</system-reminder>
`, strings.Join(sections, "\n"))
}

// isGitRepo checks if the directory is inside a git repository.
func isGitRepo(cwd string) bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// gitCurrentBranch returns the current branch name.
func gitCurrentBranch(cwd string) string {
	cmd := exec.Command("git", "--no-optional-locks", "branch", "--show-current")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	result := strings.TrimSpace(string(out))
	if result == "" {
		// Detached HEAD — fall back to short SHA.
		cmd2 := exec.Command("git", "--no-optional-locks", "rev-parse", "--short", "HEAD")
		cmd2.Dir = cwd
		out2, err2 := cmd2.Output()
		if err2 != nil {
			return "unknown"
		}
		return strings.TrimSpace(string(out2))
	}
	return result
}

// gitMainBranch detects the main branch name (main or master).
func gitMainBranch(cwd string) string {
	// Try to detect from remote HEAD.
	cmd := exec.Command("git", "--no-optional-locks", "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err == nil {
		ref := strings.TrimSpace(string(out))
		parts := strings.Split(ref, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	// Fall back: check if main or master exists.
	for _, branch := range []string{"main", "master"} {
		cmd := exec.Command("git", "--no-optional-locks", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
		cmd.Dir = cwd
		if err := cmd.Run(); err == nil {
			return branch
		}
	}

	return "main"
}

// gitStatusShort returns the short status output.
func gitStatusShort(cwd string) string {
	cmd := exec.Command("git", "--no-optional-locks", "status", "--short")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitRecentCommits returns the last 5 commits in oneline format.
func gitRecentCommits(cwd string) string {
	cmd := exec.Command("git", "--no-optional-locks", "log", "--oneline", "-n", "5")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
