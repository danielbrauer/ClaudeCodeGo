package tui

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// diffData holds the parsed result of git diff HEAD.
type diffData struct {
	stats    diffStats
	files    []diffFile
	hunks    map[string][]diffHunk // keyed by file path
	loading  bool
	errorMsg string
}

// diffStats holds aggregate change statistics.
type diffStats struct {
	filesCount   int
	linesAdded   int
	linesRemoved int
}

// diffFile represents a single changed file.
type diffFile struct {
	path         string
	linesAdded   int
	linesRemoved int
	isBinary     bool
	isLargeFile  bool
	isTruncated  bool
	isUntracked  bool
	isNewFile    bool
}

// diffHunk represents a single diff hunk.
type diffHunk struct {
	oldStart int
	oldLines int
	newStart int
	newLines int
	lines    []string
}

const (
	diffTimeout        = 5 * time.Second
	diffMaxFiles       = 50
	diffMaxHunkLines   = 400
	diffMaxDiffSize    = 1_000_000
	diffMaxDisplayFiles = 5 // visible files in list before scrolling
)

// gitPath returns the git executable path.
func gitPath() string {
	p, err := exec.LookPath("git")
	if err != nil {
		return "git"
	}
	return p
}

// loadDiffData runs git commands to gather uncommitted changes.
func loadDiffData() diffData {
	if !isGitRepo() {
		return diffData{errorMsg: "Not a git repository"}
	}

	if isGitMergeState() {
		return diffData{errorMsg: "Git is in a merge/rebase state"}
	}

	// First get --shortstat to check if there are too many files.
	shortstat, err := runGit("--no-optional-locks", "diff", "HEAD", "--shortstat")
	if err == nil && shortstat != "" {
		stats := parseShortstat(shortstat)
		if stats != nil && stats.filesCount > 500 {
			return diffData{
				stats: *stats,
				files: nil,
				hunks: make(map[string][]diffHunk),
			}
		}
	}

	// Get --numstat for per-file stats.
	numstat, err := runGit("--no-optional-locks", "diff", "HEAD", "--numstat")
	if err != nil {
		return diffData{errorMsg: "Failed to run git diff: " + err.Error()}
	}

	stats, perFileStats := parseNumstat(numstat)

	// Collect untracked files to fill up to diffMaxFiles.
	remaining := diffMaxFiles - len(perFileStats)
	if remaining > 0 {
		untracked := getUntrackedFiles(remaining)
		for _, f := range untracked {
			perFileStats = append(perFileStats, f)
			stats.filesCount++
		}
	}

	// Get full diff for hunks.
	fullDiff, err := runGit("--no-optional-locks", "diff", "HEAD")
	hunks := make(map[string][]diffHunk)
	if err == nil {
		hunks = parseDiffOutput(fullDiff)
	}

	return diffData{
		stats: stats,
		files: perFileStats,
		hunks: hunks,
	}
}

// DiffLoadedMsg carries the result of loading diff data.
type DiffLoadedMsg struct {
	Data diffData
}

// isGitRepo checks if the current directory is inside a git repo.
func isGitRepo() bool {
	_, err := runGit("rev-parse", "--is-inside-work-tree")
	return err == nil
}

// isGitMergeState checks for merge/rebase/cherry-pick/revert in progress.
func isGitMergeState() bool {
	gitDir, err := runGit("rev-parse", "--git-dir")
	if err != nil {
		return false
	}
	gitDir = strings.TrimSpace(gitDir)
	for _, head := range []string{"MERGE_HEAD", "REBASE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD"} {
		// Use git rev-parse to check for special refs.
		_, err := runGit("rev-parse", "--verify", gitDir+"/"+head)
		if err == nil {
			return true
		}
	}
	return false
}

// runGit executes a git command and returns stdout.
func runGit(args ...string) (string, error) {
	cmd := exec.Command(gitPath(), args...)
	out, err := cmd.Output()
	return string(out), err
}

var shortstatRegex = regexp.MustCompile(`(\d+)\s+files?\s+changed(?:,\s+(\d+)\s+insertions?\(\+\))?(?:,\s+(\d+)\s+deletions?\(-\))?`)

// parseShortstat parses git diff --shortstat output.
func parseShortstat(s string) *diffStats {
	m := shortstatRegex.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	files, _ := strconv.Atoi(m[1])
	added, _ := strconv.Atoi(m[2])
	removed, _ := strconv.Atoi(m[3])
	return &diffStats{filesCount: files, linesAdded: added, linesRemoved: removed}
}

// parseNumstat parses git diff --numstat output.
func parseNumstat(output string) (diffStats, []diffFile) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var stats diffStats
	var files []diffFile

	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		stats.filesCount++
		isBinary := parts[0] == "-" || parts[1] == "-"
		added, _ := strconv.Atoi(parts[0])
		removed, _ := strconv.Atoi(parts[1])
		stats.linesAdded += added
		stats.linesRemoved += removed

		if len(files) < diffMaxFiles {
			files = append(files, diffFile{
				path:         parts[2],
				linesAdded:   added,
				linesRemoved: removed,
				isBinary:     isBinary,
			})
		}
	}

	return stats, files
}

// getUntrackedFiles returns untracked files (up to limit).
func getUntrackedFiles(limit int) []diffFile {
	output, err := runGit("--no-optional-locks", "ls-files", "--others", "--exclude-standard")
	if err != nil || strings.TrimSpace(output) == "" {
		return nil
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	var files []diffFile
	for _, line := range lines {
		if line == "" {
			continue
		}
		if len(files) >= limit {
			break
		}
		files = append(files, diffFile{
			path:        line,
			isUntracked: true,
		})
	}
	return files
}

var hunkHeaderRegex = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// parseDiffOutput parses full git diff output into per-file hunks.
func parseDiffOutput(output string) map[string][]diffHunk {
	result := make(map[string][]diffHunk)
	if strings.TrimSpace(output) == "" {
		return result
	}

	// Split on "diff --git " boundaries.
	sections := strings.Split(output, "diff --git ")
	for _, section := range sections {
		if len(result) >= diffMaxFiles {
			break
		}
		if len(section) > diffMaxDiffSize {
			continue
		}
		if section == "" {
			continue
		}

		lines := strings.Split(section, "\n")

		// First line: "a/file b/file"
		headerMatch := regexp.MustCompile(`^a/(.+?) b/(.+)$`).FindStringSubmatch(lines[0])
		if headerMatch == nil {
			continue
		}
		filePath := headerMatch[2]
		if filePath == "" {
			filePath = headerMatch[1]
		}

		var hunks []diffHunk
		var currentHunk *diffHunk
		lineCount := 0

		for i := 1; i < len(lines); i++ {
			line := lines[i]

			// Check for hunk header.
			hm := hunkHeaderRegex.FindStringSubmatch(line)
			if hm != nil {
				if currentHunk != nil {
					hunks = append(hunks, *currentHunk)
				}
				oldStart, _ := strconv.Atoi(hm[1])
				oldLines := 1
				if hm[2] != "" {
					oldLines, _ = strconv.Atoi(hm[2])
				}
				newStart, _ := strconv.Atoi(hm[3])
				newLines := 1
				if hm[4] != "" {
					newLines, _ = strconv.Atoi(hm[4])
				}
				currentHunk = &diffHunk{
					oldStart: oldStart,
					oldLines: oldLines,
					newStart: newStart,
					newLines: newLines,
				}
				continue
			}

			// Skip metadata lines.
			if strings.HasPrefix(line, "index ") ||
				strings.HasPrefix(line, "---") ||
				strings.HasPrefix(line, "+++") ||
				strings.HasPrefix(line, "new file") ||
				strings.HasPrefix(line, "deleted file") ||
				strings.HasPrefix(line, "old mode") ||
				strings.HasPrefix(line, "new mode") ||
				strings.HasPrefix(line, "Binary files") {
				continue
			}

			// Diff content lines.
			if currentHunk != nil &&
				(strings.HasPrefix(line, "+") ||
					strings.HasPrefix(line, "-") ||
					strings.HasPrefix(line, " ") ||
					line == "") {
				if lineCount >= diffMaxHunkLines {
					continue
				}
				currentHunk.lines = append(currentHunk.lines, line)
				lineCount++
			}
		}

		if currentHunk != nil {
			hunks = append(hunks, *currentHunk)
		}
		if len(hunks) > 0 {
			result[filePath] = hunks
		}
	}

	return result
}

// renderDiffView renders the full diff dialog for the TUI.
func renderDiffView(d *diffData, selectedFile int, viewMode string, width int) string {
	var b strings.Builder

	// Title bar.
	title := diffTitleStyle.Render("  Uncommitted changes")
	if d.errorMsg != "" {
		title += " " + diffDimStyle.Render(d.errorMsg)
	}
	b.WriteString(title + "\n")

	// Summary stats.
	if d.stats.filesCount > 0 {
		statsStr := fmt.Sprintf("  %d file%s changed", d.stats.filesCount, plural(d.stats.filesCount))
		if d.stats.linesAdded > 0 {
			statsStr += " " + diffAddStyle.Render(fmt.Sprintf("+%d", d.stats.linesAdded))
		}
		if d.stats.linesRemoved > 0 {
			statsStr += " " + diffRemoveStyle.Render(fmt.Sprintf("-%d", d.stats.linesRemoved))
		}
		b.WriteString(diffDimStyle.Render(statsStr) + "\n")
	}

	b.WriteString(diffDimStyle.Render("  " + strings.Repeat("─", clamp(width-4, 1, 200))) + "\n")

	if len(d.files) == 0 {
		if d.stats.filesCount > 0 {
			b.WriteString(diffDimStyle.Render("  Too many files to display details") + "\n")
		} else {
			b.WriteString(diffDimStyle.Render("  Working tree is clean") + "\n")
		}
		b.WriteString("\n")
		b.WriteString(diffDimStyle.Render("  Press Esc to close"))
		return b.String()
	}

	if viewMode == "list" {
		b.WriteString(renderDiffFileList(d, selectedFile, width))
		b.WriteString("\n")
		b.WriteString(diffDimStyle.Render("  ↑/↓ select  Enter view  Esc close"))
	} else {
		// Detail view for the selected file.
		if selectedFile >= 0 && selectedFile < len(d.files) {
			b.WriteString(renderDiffFileDetail(d, selectedFile, width))
		}
		b.WriteString("\n")
		b.WriteString(diffDimStyle.Render("  ← back  Esc close"))
	}

	return b.String()
}

// renderDiffFileList renders the file list with selection highlight.
func renderDiffFileList(d *diffData, selected int, width int) string {
	var b strings.Builder
	files := d.files

	// Compute visible window with scrolling.
	startIdx := 0
	endIdx := len(files)
	if len(files) > diffMaxDisplayFiles {
		startIdx = selected - diffMaxDisplayFiles/2
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx = startIdx + diffMaxDisplayFiles
		if endIdx > len(files) {
			endIdx = len(files)
			startIdx = endIdx - diffMaxDisplayFiles
			if startIdx < 0 {
				startIdx = 0
			}
		}
	}

	// Show "more above" indicator.
	if startIdx > 0 {
		b.WriteString(diffDimStyle.Render(fmt.Sprintf("  ↑ %d more file%s", startIdx, plural(startIdx))) + "\n")
	}

	maxPathWidth := clamp(width-20, 20, 200)

	for i := startIdx; i < endIdx; i++ {
		f := files[i]
		isSelected := i == selected

		// Pointer.
		pointer := "  "
		if isSelected {
			pointer = "› "
		}

		// Truncate path if needed.
		path := f.path
		if len(path) > maxPathWidth {
			path = "…" + path[len(path)-maxPathWidth+1:]
		}

		// File name + stats.
		var suffix string
		if f.isUntracked {
			suffix = diffDimStyle.Render(" untracked")
		} else if f.isBinary {
			suffix = diffDimStyle.Render(" Binary file")
		} else if f.isLargeFile {
			suffix = diffDimStyle.Render(" Large file modified")
		} else {
			var parts []string
			if f.linesAdded > 0 {
				parts = append(parts, diffAddStyle.Render(fmt.Sprintf("+%d", f.linesAdded)))
			}
			if f.linesRemoved > 0 {
				parts = append(parts, diffRemoveStyle.Render(fmt.Sprintf("-%d", f.linesRemoved)))
			}
			if len(parts) > 0 {
				suffix = " " + strings.Join(parts, " ")
			}
		}

		if isSelected {
			line := diffSelectedStyle.Render(pointer+path) + suffix
			b.WriteString(line + "\n")
		} else {
			line := "  " + path + suffix
			b.WriteString(line + "\n")
		}
	}

	// Show "more below" indicator.
	if endIdx < len(files) {
		remaining := len(files) - endIdx
		b.WriteString(diffDimStyle.Render(fmt.Sprintf("  ↓ %d more file%s", remaining, plural(remaining))) + "\n")
	}

	return b.String()
}

// renderDiffFileDetail renders the detailed diff for a single file.
func renderDiffFileDetail(d *diffData, fileIdx int, width int) string {
	var b strings.Builder
	f := d.files[fileIdx]

	// File header.
	b.WriteString("  " + diffFileHeaderStyle.Render(f.path))
	if f.isTruncated {
		b.WriteString(diffDimStyle.Render(" (truncated)"))
	}
	b.WriteString("\n")
	b.WriteString(diffDimStyle.Render("  " + strings.Repeat("─", clamp(width-4, 1, 200))) + "\n")

	if f.isUntracked {
		b.WriteString(diffDimStyle.Render("  New file not yet staged.") + "\n")
		b.WriteString(diffDimStyle.Render(fmt.Sprintf("  Run `git add %s` to see line counts.", f.path)) + "\n")
		return b.String()
	}

	if f.isBinary {
		b.WriteString(diffDimStyle.Render("  Binary file - cannot display diff") + "\n")
		return b.String()
	}

	if f.isLargeFile {
		b.WriteString(diffDimStyle.Render("  Large file - diff exceeds 1 MB limit") + "\n")
		return b.String()
	}

	hunks, ok := d.hunks[f.path]
	if !ok || len(hunks) == 0 {
		b.WriteString(diffDimStyle.Render("  No diff content") + "\n")
		return b.String()
	}

	// Render each hunk.
	for _, hunk := range hunks {
		// Hunk header.
		header := fmt.Sprintf("  @@ -%d,%d +%d,%d @@", hunk.oldStart, hunk.oldLines, hunk.newStart, hunk.newLines)
		b.WriteString(diffHunkHeaderStyle.Render(header) + "\n")

		for _, line := range hunk.lines {
			if strings.HasPrefix(line, "+") {
				b.WriteString(diffAddStyle.Render("  "+line) + "\n")
			} else if strings.HasPrefix(line, "-") {
				b.WriteString(diffRemoveStyle.Render("  "+line) + "\n")
			} else {
				b.WriteString(diffDimStyle.Render("  "+line) + "\n")
			}
		}
	}

	return b.String()
}

// plural returns "s" if n != 1.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// clamp restricts v to [lo, hi].
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
