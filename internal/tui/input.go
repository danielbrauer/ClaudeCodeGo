package tui

import (
	"math/rand"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// newTextInput creates and configures the multi-line text input editor.
func newTextInput(width int) textarea.Model {
	ti := textarea.New()
	ti.Placeholder = ""
	ti.CharLimit = 0 // no limit
	ti.ShowLineNumbers = false
	ti.FocusedStyle.CursorLine = lipgloss.NewStyle() // no cursor line highlight
	ti.FocusedStyle.Prompt = promptStyle              // purple bold styling applied by textarea
	ti.BlurredStyle.Prompt = promptStyle

	// Use SetPromptFunc so only the first display line gets the chevron;
	// continuation lines (hard newlines and soft wraps) get blank space.
	// promptWidth=2 matches the visual width of "❯ ".
	// SetWidth must be called AFTER SetPromptFunc (textarea docs requirement).
	ti.SetPromptFunc(2, func(lineIdx int) string {
		if lineIdx == 0 {
			return "❯ "
		}
		return "  "
	})
	ti.SetWidth(width)
	ti.SetHeight(1)
	ti.Focus()
	return ti
}

// promptSuggestionTemplates is the set of suggestion strings shown in the
// input placeholder. Some contain %s which gets replaced with a filename
// discovered from git history (or "<filepath>" as fallback).
var promptSuggestionTemplates = []string{
	"fix lint errors",
	"fix typecheck errors",
	"how does %s work?",
	"refactor %s",
	"how do I log an error?",
	"edit %s to...",
	"write a test for %s",
	"create a util logging.py that...",
}

// generatePromptSuggestion returns a suggestion string like `Try "edit app.go to..."`.
// It picks a random template, substituting a filename from recent git history
// (or "<filepath>" if git is unavailable).
func generatePromptSuggestion() string {
	file := discoverExampleFile()
	tmpl := promptSuggestionTemplates[rand.Intn(len(promptSuggestionTemplates))]
	if strings.Contains(tmpl, "%s") {
		tmpl = strings.Replace(tmpl, "%s", file, 1)
	}
	return `Try "` + tmpl + `"`
}

// discoverExampleFile returns a representative filename from git history.
// It looks at the user's recent modifications and picks a random non-config
// filename. Returns "<filepath>" if nothing suitable is found.
func discoverExampleFile() string {
	// Try to get the current user's email for author filtering.
	emailCmd := exec.Command("git", "config", "user.email")
	emailOut, err := emailCmd.Output()
	if err != nil || strings.TrimSpace(string(emailOut)) == "" {
		return pickGitFile("")
	}
	return pickGitFile(strings.TrimSpace(string(emailOut)))
}

// pickGitFile runs git log to find recently modified files, optionally filtered
// by author email. Returns a random basename or "<filepath>".
func pickGitFile(authorEmail string) string {
	args := []string{"log", "-n", "500", "--pretty=format:", "--name-only", "--diff-filter=M"}
	if authorEmail != "" {
		args = append(args, "--author="+authorEmail)
	}
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return "<filepath>"
	}

	lines := strings.Split(string(out), "\n")
	// Count file occurrences and pick from the most frequent.
	counts := make(map[string]int)
	for _, line := range lines {
		f := strings.TrimSpace(line)
		if f == "" {
			continue
		}
		counts[f]++
	}

	if len(counts) == 0 {
		return "<filepath>"
	}

	// Collect basenames of the top files (dedup).
	type fc struct {
		name  string
		count int
	}
	var sorted []fc
	for name, count := range counts {
		sorted = append(sorted, fc{name, count})
	}
	// Sort by count descending (simple selection for top N).
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].count > sorted[i].count {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Take top 20 unique basenames.
	seen := make(map[string]bool)
	var basenames []string
	for _, f := range sorted {
		parts := strings.Split(f.name, "/")
		base := parts[len(parts)-1]
		// Skip files that look like config/generated.
		if base == "" || base == "go.sum" || base == "package-lock.json" || base == "yarn.lock" {
			continue
		}
		if !seen[base] {
			seen[base] = true
			basenames = append(basenames, base)
		}
		if len(basenames) >= 20 {
			break
		}
	}

	if len(basenames) == 0 {
		return "<filepath>"
	}
	return basenames[rand.Intn(len(basenames))]
}

// maxInputLines is the upper bound for auto-expanding the text input height.
const maxInputLines = 10

// updateTextInputHeight adjusts the textarea height to match the visual line
// count (accounting for both hard newlines and word wrapping), clamped to
// [1, maxInputLines].
func updateTextInputHeight(m *model) {
	val := m.textInput.Value()
	if val == "" {
		m.textInput.SetHeight(1)
		return
	}

	// Width() already subtracts prompt width internally, so use it directly.
	textWidth := m.textInput.Width()
	if textWidth < 1 {
		textWidth = 1
	}

	// Use the same wrapping the textarea uses: word wrap then hard wrap.
	wrapped := ansi.Hardwrap(ansi.Wordwrap(val, textWidth, ""), textWidth, true)
	visual := strings.Count(wrapped, "\n") + 1

	if visual > maxInputLines {
		visual = maxInputLines
	}
	m.textInput.SetHeight(visual)
}

// renderInputBorder renders a full-width horizontal line in the prompt border color.
func renderInputBorder(width int) string {
	return inputBorderStyle.Render(strings.Repeat("─", width))
}
